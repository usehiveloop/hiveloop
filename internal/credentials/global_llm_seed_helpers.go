package credentials

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/cache"
	"github.com/usehivy/hivy/internal/counter"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/model"
)

func validateGlobalLLMSpec(spec *globalLLMCredentialSpec) error {
	spec.ID = strings.TrimSpace(spec.ID)
	spec.Label = strings.TrimSpace(spec.Label)
	spec.ProviderID = strings.TrimSpace(spec.ProviderID)
	spec.BaseURL = strings.TrimSpace(spec.BaseURL)
	spec.AuthScheme = strings.TrimSpace(spec.AuthScheme)
	spec.APIKeyEnv = strings.TrimSpace(spec.APIKeyEnv)
	if spec.ID == "" {
		return fmt.Errorf("global LLM credential id is required")
	}
	if spec.ProviderID == "" {
		return fmt.Errorf("global LLM credential %q provider_id is required", spec.ID)
	}
	if spec.Label == "" {
		spec.Label = spec.ID
	}
	seedProvider, ok := globalLLMSeedProviders[spec.ProviderID]
	if !ok {
		return fmt.Errorf("global LLM credential %q has unsupported seed provider_id %q", spec.ID, spec.ProviderID)
	}
	if spec.BaseURL == "" {
		return fmt.Errorf("global LLM credential %q base_url is required", spec.ID)
	}
	if spec.AuthScheme == "" {
		spec.AuthScheme = seedProvider.AuthScheme
	}
	validSchemes := map[string]bool{"bearer": true, "x-api-key": true, "api-key": true, "query_param": true}
	if !validSchemes[spec.AuthScheme] {
		return fmt.Errorf("global LLM credential %q has invalid auth_scheme %q", spec.ID, spec.AuthScheme)
	}
	if spec.APIKeyEnv == "" && (spec.Enabled == nil || *spec.Enabled) {
		return fmt.Errorf("global LLM credential %q api_key_env is required", spec.ID)
	}
	if err := validateCredentialBaseURL(spec.BaseURL); err != nil {
		return fmt.Errorf("global LLM credential %q invalid base_url: %w", spec.ID, err)
	}
	if spec.RefillInterval != nil {
		if _, err := time.ParseDuration(*spec.RefillInterval); err != nil {
			return fmt.Errorf("global LLM credential %q invalid refill_interval: %w", spec.ID, err)
		}
	}
	return nil
}

func loadManagedCredential(ctx context.Context, db *gorm.DB, seedID string) (*model.Credential, error) {
	var cred model.Credential
	err := db.WithContext(ctx).
		Where("org_id IS NULL AND meta @> ?::jsonb", managedCredentialLookupJSON(seedID)).
		Order("created_at DESC").
		First(&cred).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load global LLM credential %q: %w", seedID, err)
	}
	return &cred, nil
}

func managedCredentialMeta(spec globalLLMCredentialSpec) model.JSON {
	meta := model.JSON{}
	for k, v := range spec.Meta {
		meta[k] = v
	}
	meta["managed_by"] = globalLLMSeedManager
	meta["global_seed_id"] = spec.ID
	meta["api_key_env"] = spec.APIKeyEnv
	return meta
}

func managedCredentialLookupJSON(seedID string) string {
	body, _ := json.Marshal(map[string]any{
		"managed_by":     globalLLMSeedManager,
		"global_seed_id": seedID,
	})
	return string(body)
}

func encryptCredentialValue(ctx context.Context, kms *crypto.KeyWrapper, plaintext []byte) ([]byte, []byte, error) {
	dek, err := crypto.GenerateDEK()
	if err != nil {
		return nil, nil, err
	}
	encryptedKey, err := crypto.EncryptCredential(plaintext, dek)
	if err != nil {
		zeroBytes(dek)
		return nil, nil, err
	}
	wrappedDEK, err := kms.Wrap(ctx, dek)
	zeroBytes(dek)
	if err != nil {
		return nil, nil, err
	}
	return encryptedKey, wrappedDEK, nil
}

func decryptExistingCredential(ctx context.Context, kms *crypto.KeyWrapper, cred *model.Credential) ([]byte, error) {
	dek, err := kms.Unwrap(ctx, cred.WrappedDEK)
	if err != nil {
		return nil, err
	}
	plaintext, err := crypto.DecryptCredential(cred.EncryptedKey, dek)
	zeroBytes(dek)
	return plaintext, err
}

func credentialMatchesSpec(cred *model.Credential, spec globalLLMCredentialSpec, meta model.JSON) bool {
	return cred.Label == spec.Label &&
		cred.BaseURL == spec.BaseURL &&
		cred.AuthScheme == spec.AuthScheme &&
		cred.ProviderID == spec.ProviderID &&
		int64PtrEqual(cred.Remaining, spec.Remaining) &&
		int64PtrEqual(cred.RefillAmount, spec.RefillAmount) &&
		stringPtrEqual(cred.RefillInterval, spec.RefillInterval) &&
		metaEqual(cred.Meta, meta)
}

func metaEqual(left, right model.JSON) bool {
	lb, _ := json.Marshal(left)
	rb, _ := json.Marshal(right)
	return bytes.Equal(lb, rb)
}

func int64PtrEqual(left, right *int64) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}

func stringPtrEqual(left, right *string) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}

func seedCounter(ctx context.Context, ctr *counter.Counter, credentialID string, remaining *int64) {
	if ctr != nil && remaining != nil {
		_ = ctr.SeedCredential(ctx, credentialID, *remaining)
	}
}

func invalidateCredential(ctx context.Context, cm *cache.Manager, credentialID string) {
	if cm != nil {
		_ = cm.InvalidateCredential(ctx, credentialID)
	}
}

func revokeManagedCredentialsNotInManifest(ctx context.Context, db *gorm.DB, cm *cache.Manager, seen map[string]bool) (int, error) {
	var creds []model.Credential
	if err := db.WithContext(ctx).
		Where("org_id IS NULL AND revoked_at IS NULL AND meta @> ?::jsonb", `{"managed_by":"global_llm_seed"}`).
		Find(&creds).Error; err != nil {
		return 0, fmt.Errorf("list managed global LLM credentials for prune: %w", err)
	}
	revoked := 0
	now := time.Now()
	for _, cred := range creds {
		seedID, _ := cred.Meta["global_seed_id"].(string)
		if seen[seedID] {
			continue
		}
		if err := db.WithContext(ctx).Model(&cred).Update("revoked_at", &now).Error; err != nil {
			return revoked, fmt.Errorf("prune global LLM credential %q: %w", seedID, err)
		}
		invalidateCredential(ctx, cm, cred.ID.String())
		revoked++
	}
	return revoked, nil
}

func zeroBytes(data []byte) {
	for i := range data {
		data[i] = 0
	}
}

func validateCredentialBaseURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("parse failed")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("scheme must be http or https")
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("missing host")
	}
	return nil
}
