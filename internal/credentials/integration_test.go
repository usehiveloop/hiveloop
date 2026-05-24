package credentials_test

import (
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/testdb"
)

// Integration tests connect to a real Postgres instance to catch bugs the
// mocked unit tests can't — wrong GORM tags, bad SQL, idempotency failures.
// Run `make test-setup` first to bring up the local Postgres container.
//
// Helpers live here; per-area tests live in <area>_integration_test.go.

// testDBURL mirrors the DSN used throughout the other integration-test packages
// (internal/middleware/integration_test.go, internal/rag/testhelpers/db.go).
// Password is a documented local-dev value; same one CI uses via env.
//
//nolint:gosec // G101: hardcoded local-dev DSN, mirrors sibling integration tests
const testDBURL = "postgres://hivy:localdev@localhost:15432/hivy_test?sslmode=disable"

// connectTestDB opens a real Postgres connection and runs goose migrations.
// It follows the same shape as the sibling helpers but only migrates the core
// schema — the rag schema isn't needed here.
func connectTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = testDBURL
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("cannot connect to Postgres at %s (run `make test-setup` first): %v", dsn, err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("underlying sql.DB: %v", err)
	}
	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("Postgres not reachable at %s: %v", dsn, err)
	}
	testdb.ApplyMigrations(t, db)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

// seedSystemCred inserts an org-less system credential and registers a cleanup.
// revoked=true sets RevokedAt so the picker should filter it out.
func seedSystemCred(t *testing.T, db *gorm.DB, providerID string, revoked bool) model.Credential {
	t.Helper()
	cred := model.Credential{
		Label:        "system-" + providerID,
		BaseURL:      "https://api.example.com",
		AuthScheme:   "bearer",
		EncryptedKey: []byte("enc"),
		WrappedDEK:   []byte("dek"),
		ProviderID:   providerID,
	}
	if revoked {
		now := time.Now()
		cred.RevokedAt = &now
	}
	if err := db.Create(&cred).Error; err != nil {
		t.Fatalf("seed system credential: %v", err)
	}
	t.Cleanup(func() { db.Unscoped().Delete(&cred) })
	return cred
}

// seedBYOKOrg creates a throwaway org.
func seedBYOKOrg(t *testing.T, db *gorm.DB) uuid.UUID {
	t.Helper()
	org := model.Org{
		ID:     uuid.New(),
		Name:   "byok-org-" + uuid.NewString(),
		Active: true,
	}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("seed byok org: %v", err)
	}
	t.Cleanup(func() { db.Unscoped().Delete(&org) })
	return org.ID
}

// seedBYOKCred inserts an org-owned credential.
func seedBYOKCred(t *testing.T, db *gorm.DB, orgID uuid.UUID, providerID string) model.Credential {
	t.Helper()
	cred := model.Credential{
		OrgID:        &orgID,
		Label:        "byok-" + providerID,
		BaseURL:      "https://api.example.com",
		AuthScheme:   "bearer",
		EncryptedKey: []byte("enc"),
		WrappedDEK:   []byte("dek"),
		ProviderID:   providerID,
	}
	if err := db.Create(&cred).Error; err != nil {
		t.Fatalf("seed byok credential: %v", err)
	}
	t.Cleanup(func() { db.Unscoped().Delete(&cred) })
	return cred
}
