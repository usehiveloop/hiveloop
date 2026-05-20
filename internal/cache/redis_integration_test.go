package cache_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/awnumar/memguard"
	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/cache"
)

func TestIntegration_RedisCache_SetGetRoundtrip(t *testing.T) {
	rc := connectTestRedis(t)
	redisCache := cache.NewRedisCache(rc, 5*time.Minute)
	ctx := context.Background()
	credID := uuid.New().String()
	t.Cleanup(func() { rc.Del(ctx, "pbcred:"+credID) })

	cred := &cache.RedisCredential{
		EncryptedKey: []byte("encrypted-api-key"),
		WrappedDEK:   []byte("wrapped-dek-bytes"),
		BaseURL:      "https://api.anthropic.com",
		AuthScheme:   "x-api-key",
		OrgID:        uuid.New().String(),
	}
	if err := redisCache.Set(ctx, credID, cred); err != nil {
		t.Fatalf("redis set: %v", err)
	}

	got, err := redisCache.Get(ctx, credID)
	if err != nil {
		t.Fatalf("redis get: %v", err)
	}
	if got == nil {
		t.Fatal("expected cache hit")
	}
	if string(got.EncryptedKey) != "encrypted-api-key" {
		t.Fatalf("expected encrypted key, got %q", got.EncryptedKey)
	}
	if got.BaseURL != "https://api.anthropic.com" {
		t.Fatalf("expected base URL, got %q", got.BaseURL)
	}
	if got.AuthScheme != "x-api-key" {
		t.Fatalf("expected auth scheme, got %q", got.AuthScheme)
	}
}

func TestIntegration_RedisCache_Miss(t *testing.T) {
	rc := connectTestRedis(t)
	redisCache := cache.NewRedisCache(rc, 5*time.Minute)

	got, err := redisCache.Get(context.Background(), "nonexistent-"+uuid.New().String())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatal("expected nil on cache miss")
	}
}

func TestIntegration_RedisCache_Invalidate(t *testing.T) {
	rc := connectTestRedis(t)
	redisCache := cache.NewRedisCache(rc, 5*time.Minute)
	ctx := context.Background()
	credID := uuid.New().String()

	_ = redisCache.Set(ctx, credID, &cache.RedisCredential{
		EncryptedKey: []byte("ek"),
		WrappedDEK:   []byte("wd"),
		BaseURL:      "https://example.com",
		AuthScheme:   "bearer",
		OrgID:        uuid.New().String(),
	})

	if err := redisCache.Invalidate(ctx, credID); err != nil {
		t.Fatalf("invalidate: %v", err)
	}

	got, _ := redisCache.Get(ctx, credID)
	if got != nil {
		t.Fatal("expected nil after invalidation")
	}
}

func TestIntegration_CacheManager_DoesNotPersistPlaintextCredentialInRedis(t *testing.T) {
	db := connectTestDB(t)
	rc := connectTestRedis(t)
	kms := createTestKMS(t)
	mgr := buildManager(t, rc, kms, db)
	ctx := context.Background()

	plaintextKey := "sk-super-secret-key-never-in-redis"
	org := createTestOrg(t, db)
	cred := createTestCredential(t, db, kms, org.ID, plaintextKey)
	credID := cred.ID.String()
	t.Cleanup(func() { rc.Del(ctx, "pbcred:"+credID) })

	got, err := mgr.GetDecryptedCredential(ctx, credID, org.ID)
	if err != nil {
		t.Fatalf("populate cache from manager: %v", err)
	}
	if string(got.APIKey) != plaintextKey {
		t.Fatalf("manager returned API key %q, want %q", got.APIKey, plaintextKey)
	}

	raw, err := rc.Get(ctx, "pbcred:"+credID).Result()
	if err != nil {
		t.Fatalf("raw redis get: %v", err)
	}
	if strings.Contains(raw, plaintextKey) {
		t.Fatal("plaintext API key found in Redis! Security violation.")
	}

	if err := db.Unscoped().Delete(&cred).Error; err != nil {
		t.Fatalf("delete DB credential before Redis hydration: %v", err)
	}

	freshMgr := buildManager(t, rc, kms, db)
	fromRedis, err := freshMgr.GetDecryptedCredential(ctx, credID, org.ID)
	if err != nil {
		t.Fatalf("hydrate from Redis cache after DB delete: %v", err)
	}
	if string(fromRedis.APIKey) != plaintextKey {
		t.Fatalf("Redis-hydrated API key = %q, want %q", fromRedis.APIKey, plaintextKey)
	}
}

func TestIntegration_RevokedTokenCache_MarkAndCheck(t *testing.T) {
	rc := connectTestRedis(t)
	rtc := cache.NewRevokedTokenCache(rc)
	ctx := context.Background()
	jti := "jti-" + uuid.New().String()

	revoked, err := rtc.IsRevoked(ctx, jti)
	if err != nil {
		t.Fatalf("is revoked: %v", err)
	}
	if revoked {
		t.Fatal("should not be revoked initially")
	}

	if err := rtc.MarkRevoked(ctx, jti, time.Hour); err != nil {
		t.Fatalf("mark revoked: %v", err)
	}

	revoked, err = rtc.IsRevoked(ctx, jti)
	if err != nil {
		t.Fatalf("is revoked after mark: %v", err)
	}
	if !revoked {
		t.Fatal("should be revoked after MarkRevoked")
	}
}

func TestIntegration_Invalidation_CredentialPubSub(t *testing.T) {
	rc := connectTestRedis(t)
	memCache := cache.NewMemoryCache(100, 5*time.Minute)
	dekCache := cache.NewDEKCache(100, 5*time.Minute)
	inv := cache.NewInvalidator(rc, memCache, dekCache, nil)

	credID := uuid.New().String()
	memCache.Set(credID, &cache.CachedCredential{
		Enclave:    memguard.NewEnclave([]byte("secret")),
		HardExpiry: time.Now().Add(time.Hour),
	})
	dekCache.Set(credID, memguard.NewEnclave([]byte("dek-bytes")))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	subReady := make(chan struct{})
	go func() {

		close(subReady)
		_ = inv.Subscribe(ctx)
	}()
	<-subReady
	time.Sleep(100 * time.Millisecond)

	if err := inv.PublishCredentialInvalidation(context.Background(), credID); err != nil {
		t.Fatalf("publish: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	if _, ok := memCache.Get(credID); ok {
		t.Fatal("L1 cache entry should be evicted after invalidation message")
	}
	if _, ok := dekCache.Get(credID); ok {
		t.Fatal("DEK cache entry should be evicted after invalidation message")
	}
}

func TestIntegration_Invalidation_TokenPubSub(t *testing.T) {
	rc := connectTestRedis(t)
	memCache := cache.NewMemoryCache(100, 5*time.Minute)
	dekCache := cache.NewDEKCache(100, 5*time.Minute)
	inv := cache.NewInvalidator(rc, memCache, dekCache, nil)

	jti := "jti-" + uuid.New().String()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan struct{})
	go func() {
		close(ready)
		_ = inv.Subscribe(ctx)
	}()
	<-ready
	time.Sleep(100 * time.Millisecond)

	if inv.IsTokenLocallyRevoked(jti) {
		t.Fatal("should not be revoked before message")
	}

	if err := inv.PublishTokenRevocation(context.Background(), jti); err != nil {
		t.Fatalf("publish: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	if !inv.IsTokenLocallyRevoked(jti) {
		t.Fatal("should be locally revoked after pub/sub message")
	}
}
