package credentials_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/credentials"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/model"
)

func TestIntegration_SeedGlobalLLMCredentials_CreateIdempotentAndRotate(t *testing.T) {
	db := connectTestDB(t)
	kms := testKMS(t)
	envName := "TEST_GLOBAL_LLM_KEY_" + t.Name()
	t.Setenv(envName, "sk-first")
	manifest := writeGlobalLLMManifest(t, map[string]any{
		"version": 1,
		"credentials": []map[string]any{
			{
				"id":          "test-openai-primary-" + t.Name(),
				"label":       "Test OpenAI primary",
				"provider_id": "openai",
				"base_url":    "https://api.openai.com/v1",
				"api_key_env": envName,
				"required":    true,
				"meta": map[string]any{
					"tier": "test",
				},
			},
		},
	})

	first, err := credentials.SeedGlobalLLMCredentials(context.Background(), db, kms, nil, nil, manifest)
	if err != nil {
		t.Fatalf("first seed: %v", err)
	}
	if first.Created != 1 || first.Updated != 0 || first.Unchanged != 0 {
		t.Fatalf("first result = %+v, want created=1", first)
	}
	cred := loadManagedSeedCredential(t, db, "test-openai-primary-"+t.Name())
	if got := decryptCredentialForTest(t, kms, cred); got != "sk-first" {
		t.Fatalf("decrypted key = %q, want sk-first", got)
	}
	if cred.OrgID != nil {
		t.Fatalf("credential is not a system credential: org=%v", cred.OrgID)
	}
	if cred.ProviderID != "openai" || cred.AuthScheme != "bearer" || cred.BaseURL != "https://api.openai.com/v1" {
		t.Fatalf("credential metadata not defaulted correctly: provider=%q scheme=%q base=%q", cred.ProviderID, cred.AuthScheme, cred.BaseURL)
	}

	second, err := credentials.SeedGlobalLLMCredentials(context.Background(), db, kms, nil, nil, manifest)
	if err != nil {
		t.Fatalf("second seed: %v", err)
	}
	if second.Unchanged != 1 || second.Created != 0 || second.Updated != 0 {
		t.Fatalf("second result = %+v, want unchanged=1", second)
	}

	t.Setenv(envName, "sk-second")
	third, err := credentials.SeedGlobalLLMCredentials(context.Background(), db, kms, nil, nil, manifest)
	if err != nil {
		t.Fatalf("third seed: %v", err)
	}
	if third.Updated != 1 {
		t.Fatalf("third result = %+v, want updated=1", third)
	}
	cred = loadManagedSeedCredential(t, db, "test-openai-primary-"+t.Name())
	if got := decryptCredentialForTest(t, kms, cred); got != "sk-second" {
		t.Fatalf("rotated decrypted key = %q, want sk-second", got)
	}
}

func TestIntegration_SeedGlobalLLMCredentials_RequiredMissingEnvFails(t *testing.T) {
	db := connectTestDB(t)
	kms := testKMS(t)
	manifest := writeGlobalLLMManifest(t, map[string]any{
		"version": 1,
		"credentials": []map[string]any{
			{
				"id":          "test-required-missing-" + t.Name(),
				"provider_id": "openai",
				"base_url":    "https://api.openai.com/v1",
				"api_key_env": "TEST_GLOBAL_LLM_KEY_MISSING_" + t.Name(),
				"required":    true,
			},
		},
	})

	if _, err := credentials.SeedGlobalLLMCredentials(context.Background(), db, kms, nil, nil, manifest); err == nil {
		t.Fatal("expected missing required env to fail")
	}
}

func TestIntegration_SeedGlobalLLMCredentials_OptionalMissingEnvSkips(t *testing.T) {
	db := connectTestDB(t)
	kms := testKMS(t)
	seedID := "test-optional-missing-" + t.Name()
	manifest := writeGlobalLLMManifest(t, map[string]any{
		"version": 1,
		"credentials": []map[string]any{
			{
				"id":          seedID,
				"provider_id": "openai",
				"base_url":    "https://api.openai.com/v1",
				"api_key_env": "TEST_GLOBAL_LLM_KEY_OPTIONAL_MISSING_" + t.Name(),
				"required":    false,
			},
		},
	})

	result, err := credentials.SeedGlobalLLMCredentials(context.Background(), db, kms, nil, nil, manifest)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if result.Skipped != 1 || result.Created != 0 || result.Updated != 0 {
		t.Fatalf("result = %+v, want skipped=1", result)
	}
	var count int64
	if err := db.Model(&model.Credential{}).
		Where("org_id IS NULL AND meta @> ?::jsonb", `{"managed_by":"global_llm_seed","global_seed_id":"`+seedID+`"}`).
		Count(&count).Error; err != nil {
		t.Fatalf("count skipped credential: %v", err)
	}
	if count != 0 {
		t.Fatalf("created %d credential(s), want 0", count)
	}
}

func TestIntegration_SeedGlobalLLMCredentials_DisabledRevokesExisting(t *testing.T) {
	db := connectTestDB(t)
	kms := testKMS(t)
	envName := "TEST_GLOBAL_LLM_KEY_DISABLE_" + t.Name()
	seedID := "test-disable-" + t.Name()
	t.Setenv(envName, "sk-active")
	activeManifest := writeGlobalLLMManifest(t, map[string]any{
		"version": 1,
		"credentials": []map[string]any{
			{
				"id":          seedID,
				"provider_id": "openai",
				"base_url":    "https://api.openai.com/v1",
				"api_key_env": envName,
			},
		},
	})
	if _, err := credentials.SeedGlobalLLMCredentials(context.Background(), db, kms, nil, nil, activeManifest); err != nil {
		t.Fatalf("active seed: %v", err)
	}

	disabled := false
	disabledManifest := writeGlobalLLMManifest(t, map[string]any{
		"version": 1,
		"credentials": []map[string]any{
			{
				"id":          seedID,
				"provider_id": "openai",
				"base_url":    "https://api.openai.com/v1",
				"enabled":     disabled,
			},
		},
	})
	result, err := credentials.SeedGlobalLLMCredentials(context.Background(), db, kms, nil, nil, disabledManifest)
	if err != nil {
		t.Fatalf("disabled seed: %v", err)
	}
	if result.Revoked != 1 {
		t.Fatalf("disabled result = %+v, want revoked=1", result)
	}
	cred := loadManagedSeedCredential(t, db, seedID)
	if cred.RevokedAt == nil {
		t.Fatal("credential was not revoked")
	}
}

func TestIntegration_SeedGlobalLLMCredentials_MissingBaseURLFails(t *testing.T) {
	db := connectTestDB(t)
	kms := testKMS(t)
	envName := "TEST_GLOBAL_LLM_KEY_BASE_URL_" + t.Name()
	t.Setenv(envName, "sk-test")
	manifest := writeGlobalLLMManifest(t, map[string]any{
		"version": 1,
		"credentials": []map[string]any{
			{
				"id":          "test-missing-base-url-" + t.Name(),
				"provider_id": "openai",
				"api_key_env": envName,
			},
		},
	})

	if _, err := credentials.SeedGlobalLLMCredentials(context.Background(), db, kms, nil, nil, manifest); err == nil {
		t.Fatal("expected missing base_url to fail")
	}
}

func TestIntegration_SeedGlobalLLMCredentials_UnsupportedProviderFails(t *testing.T) {
	db := connectTestDB(t)
	kms := testKMS(t)
	envName := "TEST_GLOBAL_LLM_KEY_PROVIDER_" + t.Name()
	t.Setenv(envName, "sk-test")
	manifest := writeGlobalLLMManifest(t, map[string]any{
		"version": 1,
		"credentials": []map[string]any{
			{
				"id":          "test-unsupported-provider-" + t.Name(),
				"provider_id": "not-a-seed-provider",
				"base_url":    "https://api.example.com/v1",
				"api_key_env": envName,
			},
		},
	})

	if _, err := credentials.SeedGlobalLLMCredentials(context.Background(), db, kms, nil, nil, manifest); err == nil {
		t.Fatal("expected unsupported provider to fail")
	}
}

func writeGlobalLLMManifest(t *testing.T, body map[string]any) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "llm.json")
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return path
}

func testKMS(t *testing.T) *crypto.KeyWrapper {
	t.Helper()
	key := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))
	kms, err := crypto.NewAEADWrapper(context.Background(), key, "test")
	if err != nil {
		t.Fatalf("new test kms: %v", err)
	}
	return kms
}

func loadManagedSeedCredential(t *testing.T, db *gorm.DB, seedID string) model.Credential {
	t.Helper()
	var cred model.Credential
	if err := db.Where("org_id IS NULL AND meta @> ?::jsonb",
		`{"managed_by":"global_llm_seed","global_seed_id":"`+seedID+`"}`,
	).First(&cred).Error; err != nil {
		t.Fatalf("load managed credential %q: %v", seedID, err)
	}
	t.Cleanup(func() { db.Unscoped().Delete(&cred) })
	return cred
}

func decryptCredentialForTest(t *testing.T, kms *crypto.KeyWrapper, cred model.Credential) string {
	t.Helper()
	dek, err := kms.Unwrap(context.Background(), cred.WrappedDEK)
	if err != nil {
		t.Fatalf("unwrap dek: %v", err)
	}
	plaintext, err := crypto.DecryptCredential(cred.EncryptedKey, dek)
	for i := range dek {
		dek[i] = 0
	}
	if err != nil {
		t.Fatalf("decrypt credential: %v", err)
	}
	out := string(plaintext)
	for i := range plaintext {
		plaintext[i] = 0
	}
	return out
}
