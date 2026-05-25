package handler_test

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/testdb"
)

func connectTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := testdb.DatabaseURL("DATABASE_URL", "HIVY_DATABASE_URL", "TEST_DATABASE_URL")
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("cannot connect to Postgres: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(3)
	sqlDB.SetMaxIdleConns(1)
	testdb.ApplyMigrations(t, db)
	t.Cleanup(func() { sqlDB.Close() })
	return db
}

func connectTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	addr := testdb.RedisAddr("HIVY_REDIS_ADDR", "TEST_REDIS_ADDR")
	client := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { client.Close() })
	return client
}

func cleanupOrg(t *testing.T, db *gorm.DB, orgID uuid.UUID) {
	t.Helper()
	db.Where("org_id = ?", orgID).Delete(&model.Generation{})
	db.Where("org_id = ?", orgID).Delete(&model.APIKey{})
	db.Where("org_id = ?", orgID).Delete(&model.AuditEntry{})
	db.Where("org_id = ?", orgID).Delete(&model.Token{})
	db.Where("org_id = ?", orgID).Delete(&model.Credential{})
	db.Where("id = ?", orgID).Delete(&model.Org{})
}

func createTestOrg(t *testing.T, db *gorm.DB) model.Org {
	t.Helper()
	org := model.Org{
		ID:        uuid.New(),
		Name:      fmt.Sprintf("apikey-handler-%s", uuid.New().String()[:8]),
		RateLimit: 1000,
		Active:    true,
	}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	t.Cleanup(func() { cleanupOrg(t, db, org.ID) })
	return org
}
