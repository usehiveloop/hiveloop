package cache_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/awnumar/memguard"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/cache"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/model"
)

const (
	testDBURL     = "postgres://hivy:localdev@localhost:5433/hivy_test?sslmode=disable" // #nosec G101 -- local test DB fixture
	testRedisAddr = "localhost:6379"
)

func connectTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = testDBURL
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("cannot connect to Postgres: %v", err)
	}
	sqlDB, _ := db.DB()
	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("Postgres not reachable: %v", err)
	}
	if err := model.AutoMigrate(db); err != nil {
		t.Fatalf("migration failed: %v", err)
	}
	return db
}

func connectTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = testRedisAddr
	}
	client := redis.NewClient(&redis.Options{Addr: addr})
	if err := client.Ping(context.Background()).Err(); err != nil {
		t.Fatalf("Redis not reachable: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	return client
}

func createTestKMS(t *testing.T) *crypto.KeyWrapper {
	t.Helper()
	kms, err := crypto.NewAEADWrapper(t.Context(), "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=", "test-key")
	if err != nil {
		t.Fatalf("cannot create AEAD wrapper: %v", err)
	}
	return kms
}

// createTestCredential creates a real encrypted credential in Postgres via KMS.
func createTestCredential(t *testing.T, db *gorm.DB, kms *crypto.KeyWrapper, orgID uuid.UUID, apiKey string) model.Credential {
	t.Helper()

	dek, err := crypto.GenerateDEK()
	if err != nil {
		t.Fatalf("generate DEK: %v", err)
	}
	encryptedKey, err := crypto.EncryptCredential([]byte(apiKey), dek)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	wrappedDEK, err := kms.Wrap(context.Background(), dek)
	if err != nil {
		t.Fatalf("kms wrap: %v", err)
	}

	for i := range dek {
		dek[i] = 0
	}

	cred := model.Credential{
		ID:           uuid.New(),
		OrgID:        orgID,
		Label:        "test-cred",
		BaseURL:      "https://api.openai.com",
		AuthScheme:   "bearer",
		EncryptedKey: encryptedKey,
		WrappedDEK:   wrappedDEK,
	}
	if err := db.Create(&cred).Error; err != nil {
		t.Fatalf("create credential: %v", err)
	}
	return cred
}

func createTestOrg(t *testing.T, db *gorm.DB) model.Org {
	t.Helper()
	org := model.Org{
		ID:        uuid.New(),
		Name:      fmt.Sprintf("cache-test-%s", uuid.New().String()[:8]),
		RateLimit: 1000,
		Active:    true,
	}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	t.Cleanup(func() {
		db.Where("org_id = ?", org.ID).Delete(&model.Token{})
		db.Where("org_id = ?", org.ID).Delete(&model.Credential{})
		db.Where("id = ?", org.ID).Delete(&model.Org{})
	})
	return org
}

func buildManager(t *testing.T, redisClient *redis.Client, kms *crypto.KeyWrapper, db *gorm.DB) *cache.Manager {
	t.Helper()
	cfg := cache.Config{
		MemMaxSize: 100,
		MemTTL:     5 * time.Minute,
		RedisTTL:   10 * time.Minute,
		DEKMaxSize: 100,
		DEKTTL:     10 * time.Minute,
		HardExpiry: 15 * time.Minute,
	}
	return cache.Build(cfg, redisClient, kms, db, nil)
}

func TestMemoryCache_SetGetRoundtrip(t *testing.T) {
	mc := cache.NewMemoryCache(100, 5*time.Minute)

	apiKey := []byte("sk-test-key-12345")
	enclave := memguard.NewEnclave(apiKey)
	mc.Set("cred-1", &cache.CachedCredential{
		Enclave:    enclave,
		BaseURL:    "https://api.openai.com",
		AuthScheme: "bearer",
		OrgID:      uuid.New(),
		CachedAt:   time.Now(),
		HardExpiry: time.Now().Add(time.Hour),
	})

	got, ok := mc.Get("cred-1")
	if !ok {
		t.Fatal("expected L1 cache hit")
	}
	buf, err := got.Enclave.Open()
	if err != nil {
		t.Fatalf("open enclave: %v", err)
	}
	if string(buf.Bytes()) != "sk-test-key-12345" {
		t.Fatalf("expected 'sk-test-key-12345', got %q", buf.Bytes())
	}
	buf.Destroy()
}

func TestMemoryCache_Miss(t *testing.T) {
	mc := cache.NewMemoryCache(100, 5*time.Minute)
	_, ok := mc.Get("nonexistent")
	if ok {
		t.Fatal("expected cache miss")
	}
}

func TestMemoryCache_Invalidate(t *testing.T) {
	mc := cache.NewMemoryCache(100, 5*time.Minute)
	mc.Set("cred-1", &cache.CachedCredential{
		Enclave:    memguard.NewEnclave([]byte("key")),
		HardExpiry: time.Now().Add(time.Hour),
	})
	mc.Invalidate("cred-1")
	_, ok := mc.Get("cred-1")
	if ok {
		t.Fatal("expected miss after invalidation")
	}
}

func TestMemoryCache_HardExpiry(t *testing.T) {
	mc := cache.NewMemoryCache(100, time.Hour)
	mc.Set("cred-1", &cache.CachedCredential{
		Enclave:    memguard.NewEnclave([]byte("key")),
		HardExpiry: time.Now().Add(-time.Second),
	})
	_, ok := mc.Get("cred-1")
	if ok {
		t.Fatal("expected miss for expired hard-expiry")
	}
}

func TestMemoryCache_Purge(t *testing.T) {
	mc := cache.NewMemoryCache(100, 5*time.Minute)
	for i := range 10 {
		mc.Set(fmt.Sprintf("cred-%d", i), &cache.CachedCredential{
			Enclave:    memguard.NewEnclave([]byte("key")),
			HardExpiry: time.Now().Add(time.Hour),
		})
	}
	if mc.Len() != 10 {
		t.Fatalf("expected 10 entries, got %d", mc.Len())
	}
	mc.Purge()
	if mc.Len() != 0 {
		t.Fatalf("expected 0 entries after purge, got %d", mc.Len())
	}
}

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
