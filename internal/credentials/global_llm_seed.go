package credentials

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/cache"
	"github.com/usehivy/hivy/internal/counter"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/model"
)

const (
	globalLLMSeedManager = "global_llm_seed"
)

type globalLLMSeedProvider struct {
	AuthScheme string
}

var globalLLMSeedProviders = map[string]globalLLMSeedProvider{
	"anthropic":  {AuthScheme: "x-api-key"},
	"cohere":     {AuthScheme: "bearer"},
	"deepseek":   {AuthScheme: "bearer"},
	"google":     {AuthScheme: "query_param"},
	"groq":       {AuthScheme: "bearer"},
	"mistral":    {AuthScheme: "bearer"},
	"moonshotai": {AuthScheme: "bearer"},
	"openai":     {AuthScheme: "bearer"},
	"openrouter": {AuthScheme: "bearer"},
	"perplexity": {AuthScheme: "bearer"},
	"xai":        {AuthScheme: "bearer"},
}

type GlobalLLMCredentialSeedResult struct {
	Created   int
	Updated   int
	Unchanged int
	Revoked   int
	Skipped   int
}

type globalLLMSeedManifest struct {
	Version     int                       `json:"version"`
	Prune       bool                      `json:"prune,omitempty"`
	Credentials []globalLLMCredentialSpec `json:"credentials"`
}

type globalLLMCredentialSpec struct {
	ID             string     `json:"id"`
	Label          string     `json:"label"`
	ProviderID     string     `json:"provider_id"`
	BaseURL        string     `json:"base_url,omitempty"`
	AuthScheme     string     `json:"auth_scheme,omitempty"`
	APIKeyEnv      string     `json:"api_key_env"`
	Required       bool       `json:"required,omitempty"`
	Enabled        *bool      `json:"enabled,omitempty"`
	Remaining      *int64     `json:"remaining,omitempty"`
	RefillAmount   *int64     `json:"refill_amount,omitempty"`
	RefillInterval *string    `json:"refill_interval,omitempty"`
	Meta           model.JSON `json:"meta,omitempty"`
}

func SeedGlobalLLMCredentials(ctx context.Context, db *gorm.DB, kms *crypto.KeyWrapper, cm *cache.Manager, ctr *counter.Counter, manifestPath string) (*GlobalLLMCredentialSeedResult, error) {
	if strings.TrimSpace(manifestPath) == "" {
		manifestPath = "global/credentials/llm.json"
	}
	body, err := os.ReadFile(manifestPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &GlobalLLMCredentialSeedResult{}, nil
		}
		return nil, fmt.Errorf("read global LLM credentials manifest: %w", err)
	}
	var manifest globalLLMSeedManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return nil, fmt.Errorf("parse global LLM credentials manifest: %w", err)
	}
	if manifest.Version != 1 {
		return nil, fmt.Errorf("global LLM credentials manifest version %d is unsupported", manifest.Version)
	}
	if db == nil {
		return nil, fmt.Errorf("db is required")
	}
	if kms == nil {
		return nil, fmt.Errorf("kms is required")
	}
	if err := SeedPlatformOrg(db); err != nil {
		return nil, err
	}

	result := &GlobalLLMCredentialSeedResult{}
	seen := map[string]bool{}
	for _, spec := range manifest.Credentials {
		if seen[spec.ID] {
			return nil, fmt.Errorf("duplicate global LLM credential id %q", spec.ID)
		}
		seen[spec.ID] = true
		state, err := seedGlobalLLMCredential(ctx, db, kms, cm, ctr, spec)
		if err != nil {
			return nil, err
		}
		switch state {
		case "created":
			result.Created++
		case "updated":
			result.Updated++
		case "unchanged":
			result.Unchanged++
		case "revoked":
			result.Revoked++
		case "skipped":
			result.Skipped++
		}
	}
	if manifest.Prune {
		revoked, err := revokeManagedCredentialsNotInManifest(ctx, db, cm, seen)
		if err != nil {
			return nil, err
		}
		result.Revoked += revoked
	}
	return result, nil
}

func seedGlobalLLMCredential(ctx context.Context, db *gorm.DB, kms *crypto.KeyWrapper, cm *cache.Manager, ctr *counter.Counter, spec globalLLMCredentialSpec) (string, error) {
	if err := validateGlobalLLMSpec(&spec); err != nil {
		return "", err
	}
	enabled := spec.Enabled == nil || *spec.Enabled
	existing, err := loadManagedCredential(ctx, db, spec.ID)
	if err != nil {
		return "", err
	}
	if !enabled {
		if existing == nil || existing.RevokedAt != nil {
			return "skipped", nil
		}
		now := time.Now()
		if err := db.WithContext(ctx).Model(existing).Update("revoked_at", &now).Error; err != nil {
			return "", fmt.Errorf("revoke global LLM credential %q: %w", spec.ID, err)
		}
		invalidateCredential(ctx, cm, existing.ID.String())
		return "revoked", nil
	}

	apiKey := os.Getenv(spec.APIKeyEnv)
	if apiKey == "" {
		if spec.Required {
			return "", fmt.Errorf("global LLM credential %q requires env var %s", spec.ID, spec.APIKeyEnv)
		}
		return "skipped", nil
	}

	encryptedKey, wrappedDEK, err := encryptCredentialValue(ctx, kms, []byte(apiKey))
	if err != nil {
		return "", fmt.Errorf("encrypt global LLM credential %q: %w", spec.ID, err)
	}
	meta := managedCredentialMeta(spec)

	if existing == nil {
		cred := model.Credential{
			OrgID:          PlatformOrgID,
			Label:          spec.Label,
			BaseURL:        spec.BaseURL,
			AuthScheme:     spec.AuthScheme,
			EncryptedKey:   encryptedKey,
			WrappedDEK:     wrappedDEK,
			Remaining:      spec.Remaining,
			RefillAmount:   spec.RefillAmount,
			RefillInterval: spec.RefillInterval,
			ProviderID:     spec.ProviderID,
			Meta:           meta,
			IsSystem:       true,
		}
		if err := db.WithContext(ctx).Create(&cred).Error; err != nil {
			return "", fmt.Errorf("create global LLM credential %q: %w", spec.ID, err)
		}
		seedCounter(ctx, ctr, cred.ID.String(), cred.Remaining)
		return "created", nil
	}

	keyChanged := true
	if current, err := decryptExistingCredential(ctx, kms, existing); err == nil {
		keyChanged = !bytes.Equal(current, []byte(apiKey))
		for i := range current {
			current[i] = 0
		}
	}

	updates := map[string]any{
		"label":           spec.Label,
		"base_url":        spec.BaseURL,
		"auth_scheme":     spec.AuthScheme,
		"provider_id":     spec.ProviderID,
		"remaining":       spec.Remaining,
		"refill_amount":   spec.RefillAmount,
		"refill_interval": spec.RefillInterval,
		"meta":            meta,
		"is_system":       true,
		"org_id":          PlatformOrgID,
		"revoked_at":      nil,
	}
	if keyChanged {
		updates["encrypted_key"] = encryptedKey
		updates["wrapped_dek"] = wrappedDEK
	}

	if credentialMatchesSpec(existing, spec, meta) && !keyChanged && existing.RevokedAt == nil {
		return "unchanged", nil
	}
	if err := db.WithContext(ctx).Model(existing).Updates(updates).Error; err != nil {
		return "", fmt.Errorf("update global LLM credential %q: %w", spec.ID, err)
	}
	invalidateCredential(ctx, cm, existing.ID.String())
	seedCounter(ctx, ctr, existing.ID.String(), spec.Remaining)
	return "updated", nil
}

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
		Where("org_id = ? AND is_system = ? AND meta @> ?::jsonb", PlatformOrgID, true, managedCredentialLookupJSON(seedID)).
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
		Where("org_id = ? AND is_system = ? AND revoked_at IS NULL AND meta @> ?::jsonb", PlatformOrgID, true, `{"managed_by":"global_llm_seed"}`).
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
