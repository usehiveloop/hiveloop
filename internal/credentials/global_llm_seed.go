package credentials

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	globalLLMSeedManager       = "global_llm_seed"
	globalLLMSeedLockKey int64 = 2026052402
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

	result := &GlobalLLMCredentialSeedResult{}
	seen := map[string]bool{}
	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", globalLLMSeedLockKey).Error; err != nil {
			return fmt.Errorf("lock global LLM credentials seed: %w", err)
		}
		for _, spec := range manifest.Credentials {
			if seen[spec.ID] {
				return fmt.Errorf("duplicate global LLM credential id %q", spec.ID)
			}
			seen[spec.ID] = true
			state, err := seedGlobalLLMCredential(ctx, tx, kms, cm, ctr, spec)
			if err != nil {
				return err
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
			revoked, err := revokeManagedCredentialsNotInManifest(ctx, tx, cm, seen)
			if err != nil {
				return err
			}
			result.Revoked += revoked
		}
		return nil
	}); err != nil {
		return nil, err
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
		"org_id":          nil,
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
