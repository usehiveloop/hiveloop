package cache_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/cache"
	"github.com/usehivy/hivy/internal/model"
)

func TestIntegration_CacheManager_TokenRevocation_ThreeTier(t *testing.T) {
	db := connectTestDB(t)
	rc := connectTestRedis(t)
	kms := createTestKMS(t)
	mgr := buildManager(t, rc, kms, db)

	org := createTestOrg(t, db)
	jti := "jti-" + uuid.New().String()
	credID := uuid.New()

	tokenRecord := model.Token{
		ID:           uuid.New(),
		OrgID:        org.ID,
		CredentialID: credID,
		JTI:          jti,
		ExpiresAt:    time.Now().Add(time.Hour),
	}

	dummyCred := model.Credential{
		ID: credID, OrgID: org.ID, Label: "dummy",
		BaseURL: "https://example.com", AuthScheme: "bearer",
		EncryptedKey: []byte("ek"), WrappedDEK: []byte("wd"),
	}
	db.Create(&dummyCred)
	db.Create(&tokenRecord)
	t.Cleanup(func() {
		db.Where("id = ?", tokenRecord.ID).Delete(&model.Token{})
		db.Where("id = ?", dummyCred.ID).Delete(&model.Credential{})
	})

	revoked, err := mgr.IsTokenRevoked(context.Background(), jti)
	if err != nil {
		t.Fatalf("is revoked: %v", err)
	}
	if revoked {
		t.Fatal("should not be revoked yet")
	}

	if err := mgr.InvalidateToken(context.Background(), jti, time.Hour); err != nil {
		t.Fatalf("invalidate token: %v", err)
	}

	revoked, err = mgr.IsTokenRevoked(context.Background(), jti)
	if err != nil {
		t.Fatalf("is revoked after invalidate: %v", err)
	}
	if !revoked {
		t.Fatal("should be revoked after InvalidateToken")
	}

	rtc := cache.NewRevokedTokenCache(rc)
	inRedis, _ := rtc.IsRevoked(context.Background(), jti)
	if !inRedis {
		t.Fatal("revoked JTI should be in Redis")
	}
}

func TestIntegration_CacheManager_IsTokenRevoked_PromotesFromDB(t *testing.T) {
	db := connectTestDB(t)
	rc := connectTestRedis(t)
	kms := createTestKMS(t)
	mgr := buildManager(t, rc, kms, db)

	org := createTestOrg(t, db)
	jti := "jti-db-" + uuid.New().String()
	credID := uuid.New()

	revokedAt := time.Now()
	dummyCred := model.Credential{
		ID: credID, OrgID: org.ID, Label: "dummy",
		BaseURL: "https://example.com", AuthScheme: "bearer",
		EncryptedKey: []byte("ek"), WrappedDEK: []byte("wd"),
	}
	db.Create(&dummyCred)
	tokenRecord := model.Token{
		ID:           uuid.New(),
		OrgID:        org.ID,
		CredentialID: credID,
		JTI:          jti,
		ExpiresAt:    time.Now().Add(time.Hour),
		RevokedAt:    &revokedAt,
	}
	db.Create(&tokenRecord)
	t.Cleanup(func() {
		db.Where("id = ?", tokenRecord.ID).Delete(&model.Token{})
		db.Where("id = ?", dummyCred.ID).Delete(&model.Credential{})
	})

	revoked, err := mgr.IsTokenRevoked(context.Background(), jti)
	if err != nil {
		t.Fatalf("is revoked: %v", err)
	}
	if !revoked {
		t.Fatal("should detect revocation from DB")
	}

	revoked, err = mgr.IsTokenRevoked(context.Background(), jti)
	if err != nil {
		t.Fatalf("is revoked second: %v", err)
	}
	if !revoked {
		t.Fatal("should still be revoked from L1")
	}
}

func TestIntegration_CacheManager_OrgIsolation(t *testing.T) {
	db := connectTestDB(t)
	rc := connectTestRedis(t)
	kms := createTestKMS(t)
	mgr := buildManager(t, rc, kms, db)

	org1 := createTestOrg(t, db)
	org2 := createTestOrg(t, db)
	cred := createTestCredential(t, db, kms, org1.ID, "sk-org1-secret")

	result, err := mgr.GetDecryptedCredential(context.Background(), cred.ID.String(), org1.ID)
	if err != nil {
		t.Fatalf("org1 get: %v", err)
	}
	if string(result.APIKey) != "sk-org1-secret" {
		t.Fatalf("expected 'sk-org1-secret', got %q", result.APIKey)
	}

	_, err = mgr.GetDecryptedCredential(context.Background(), cred.ID.String(), org2.ID)
	if err == nil {
		t.Fatal("org2 should NOT be able to access org1's credential")
	}
}

func TestIntegration_CacheManager_ConcurrentAccess(t *testing.T) {
	db := connectTestDB(t)
	rc := connectTestRedis(t)
	kms := createTestKMS(t)
	mgr := buildManager(t, rc, kms, db)

	org := createTestOrg(t, db)
	cred := createTestCredential(t, db, kms, org.ID, "sk-concurrent-test")

	const workers = 20
	errs := make(chan error, workers)
	for range workers {
		go func() {
			result, err := mgr.GetDecryptedCredential(context.Background(), cred.ID.String(), org.ID)
			if err != nil {
				errs <- err
				return
			}
			if string(result.APIKey) != "sk-concurrent-test" {
				errs <- fmt.Errorf("wrong key: %q", result.APIKey)
				return
			}
			errs <- nil
		}()
	}

	for range workers {
		if err := <-errs; err != nil {
			t.Fatalf("concurrent access error: %v", err)
		}
	}
}
