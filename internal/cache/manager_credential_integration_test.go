package cache_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/cache"
	"github.com/usehivy/hivy/internal/model"
)

func TestIntegration_CacheManager_L3ColdPath(t *testing.T) {
	db := connectTestDB(t)
	rc := connectTestRedis(t)
	kms := createTestKMS(t)
	mgr := buildManager(t, rc, kms, db)

	org := createTestOrg(t, db)
	cred := createTestCredential(t, db, kms, org.ID, "sk-test-cold-path-key")

	result, err := mgr.GetDecryptedCredential(context.Background(), cred.ID.String(), org.ID)
	if err != nil {
		t.Fatalf("get credential: %v", err)
	}
	if string(result.APIKey) != "sk-test-cold-path-key" {
		t.Fatalf("expected 'sk-test-cold-path-key', got %q", result.APIKey)
	}
	if result.BaseURL != "https://api.openai.com" {
		t.Fatalf("expected base URL, got %q", result.BaseURL)
	}
	if result.AuthScheme != "bearer" {
		t.Fatalf("expected auth scheme, got %q", result.AuthScheme)
	}
}

func TestIntegration_CacheManager_GetByIDResolvesSystemCredential(t *testing.T) {
	db := connectTestDB(t)
	rc := connectTestRedis(t)
	kms := createTestKMS(t)
	mgr := buildManager(t, rc, kms, db)

	cred := createTestSystemCredential(t, db, kms, "sk-system-key")
	t.Cleanup(func() { db.Where("id = ?", cred.ID).Delete(&model.Credential{}) })

	result, err := mgr.GetDecryptedCredentialByID(context.Background(), cred.ID.String())
	if err != nil {
		t.Fatalf("get system credential by id: %v", err)
	}
	if string(result.APIKey) != "sk-system-key" {
		t.Fatalf("expected system key, got %q", result.APIKey)
	}
}

func TestIntegration_CacheManager_L1HitAfterColdPath(t *testing.T) {
	db := connectTestDB(t)
	rc := connectTestRedis(t)
	kms := createTestKMS(t)
	mgr := buildManager(t, rc, kms, db)

	org := createTestOrg(t, db)
	cred := createTestCredential(t, db, kms, org.ID, "sk-test-l1-promotion")

	_, err := mgr.GetDecryptedCredential(context.Background(), cred.ID.String(), org.ID)
	if err != nil {
		t.Fatalf("first get: %v", err)
	}

	cached, ok := mgr.Memory().Get(cred.ID.String())
	if !ok {
		t.Fatal("expected L1 to be populated after cold path")
	}
	buf, err := cached.Enclave.Open()
	if err != nil {
		t.Fatalf("open enclave: %v", err)
	}
	if string(buf.Bytes()) != "sk-test-l1-promotion" {
		t.Fatalf("L1 has wrong value: %q", buf.Bytes())
	}
	buf.Destroy()

	result, err := mgr.GetDecryptedCredential(context.Background(), cred.ID.String(), org.ID)
	if err != nil {
		t.Fatalf("second get: %v", err)
	}
	if string(result.APIKey) != "sk-test-l1-promotion" {
		t.Fatalf("expected 'sk-test-l1-promotion', got %q", result.APIKey)
	}
}

func TestIntegration_CacheManager_L2HitAfterL1Eviction(t *testing.T) {
	db := connectTestDB(t)
	rc := connectTestRedis(t)
	kms := createTestKMS(t)
	mgr := buildManager(t, rc, kms, db)

	org := createTestOrg(t, db)
	cred := createTestCredential(t, db, kms, org.ID, "sk-test-l2-hit")

	_, err := mgr.GetDecryptedCredential(context.Background(), cred.ID.String(), org.ID)
	if err != nil {
		t.Fatalf("first get: %v", err)
	}

	mgr.Memory().Invalidate(cred.ID.String())
	if _, ok := mgr.Memory().Get(cred.ID.String()); ok {
		t.Fatal("L1 should be empty after invalidation")
	}

	result, err := mgr.GetDecryptedCredential(context.Background(), cred.ID.String(), org.ID)
	if err != nil {
		t.Fatalf("get after L1 eviction: %v", err)
	}
	if string(result.APIKey) != "sk-test-l2-hit" {
		t.Fatalf("expected 'sk-test-l2-hit', got %q", result.APIKey)
	}

	if _, ok := mgr.Memory().Get(cred.ID.String()); !ok {
		t.Fatal("L1 should be repopulated from L2")
	}
}

func TestIntegration_CacheManager_AllMiss_CredentialNotFound(t *testing.T) {
	db := connectTestDB(t)
	rc := connectTestRedis(t)
	kms := createTestKMS(t)
	mgr := buildManager(t, rc, kms, db)

	org := createTestOrg(t, db)

	_, err := mgr.GetDecryptedCredential(context.Background(), uuid.New().String(), org.ID)
	if err == nil {
		t.Fatal("expected error for nonexistent credential")
	}
}

func TestIntegration_CacheManager_RevokedCredentialNotServed(t *testing.T) {
	db := connectTestDB(t)
	rc := connectTestRedis(t)
	kms := createTestKMS(t)
	mgr := buildManager(t, rc, kms, db)

	org := createTestOrg(t, db)
	cred := createTestCredential(t, db, kms, org.ID, "sk-revoked-key")

	now := time.Now()
	db.Model(&cred).Update("revoked_at", &now)

	_, err := mgr.GetDecryptedCredential(context.Background(), cred.ID.String(), org.ID)
	if err == nil {
		t.Fatal("expected error for revoked credential")
	}
}

func TestIntegration_CacheManager_InvalidateCredential(t *testing.T) {
	db := connectTestDB(t)
	rc := connectTestRedis(t)
	kms := createTestKMS(t)
	mgr := buildManager(t, rc, kms, db)

	org := createTestOrg(t, db)
	cred := createTestCredential(t, db, kms, org.ID, "sk-invalidate-test")

	_, err := mgr.GetDecryptedCredential(context.Background(), cred.ID.String(), org.ID)
	if err != nil {
		t.Fatalf("initial get: %v", err)
	}

	if err := mgr.InvalidateCredential(context.Background(), cred.ID.String()); err != nil {
		t.Fatalf("invalidate: %v", err)
	}

	if _, ok := mgr.Memory().Get(cred.ID.String()); ok {
		t.Fatal("L1 should be empty after InvalidateCredential")
	}

	redisCache := cache.NewRedisCache(rc, 10*time.Minute)
	got, _ := redisCache.Get(context.Background(), cred.ID.String())
	if got != nil {
		t.Fatal("L2 should be empty after InvalidateCredential")
	}

	result, err := mgr.GetDecryptedCredential(context.Background(), cred.ID.String(), org.ID)
	if err != nil {
		t.Fatalf("get after invalidate: %v", err)
	}
	if string(result.APIKey) != "sk-invalidate-test" {
		t.Fatalf("expected 'sk-invalidate-test', got %q", result.APIKey)
	}
}
