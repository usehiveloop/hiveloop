package cache_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

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
