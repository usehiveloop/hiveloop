// Package testhelpers is the shared integration-test scaffolding for the
// RAG subsystem. Every subsequent tranche consumes `ConnectTestDB` plus the
// fixtures in fixtures.go.
//
// The pattern is a public-API duplicate of the private helper at
// internal/middleware/integration_test.go:31-60 (see TESTING.md for why we
// do not share). When Phase 1 lands we may refactor both call sites onto a
// single helper, but Phase 0 leaves the middleware one untouched.
package testhelpers

import (
	"os"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/rag"
)

// testDBURL mirrors internal/middleware/integration_test.go:26 — the
// hiveloop_test database on the dev Postgres instance at localhost:5433.
const testDBURL = "postgres://hiveloop:localdev@localhost:5433/hiveloop_test?sslmode=disable"

// ConnectTestDB opens a real Postgres connection, runs
// `model.AutoMigrate` (which in turn calls `rag.AutoMigrate`), and
// registers `t.Cleanup` to close the underlying *sql.DB.
//
// Unlike the private helper in the middleware package, this one imports
// the rag package so the RAG schema is migrated alongside the core
// Hiveloop schema. Phase 0's AutoMigrate is a no-op; Phase 1 tranches
// append real migrations.
//
// A test that needs a test DB but the service isn't running should see a
// hard, loud failure — see rule 7 of TESTING.md.
func ConnectTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = testDBURL
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("cannot connect to Postgres at %s (run `make test-services-up` first): %v", dsn, err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("failed to get underlying sql.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(3)
	sqlDB.SetMaxIdleConns(1)
	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("Postgres not reachable at %s (run `make test-services-up` first): %v", dsn, err)
	}

	if err := model.AutoMigrate(db); err != nil {
		t.Fatalf("model.AutoMigrate failed: %v", err)
	}
	if err := rag.AutoMigrate(db); err != nil {
		t.Fatalf("rag.AutoMigrate failed: %v", err)
	}

	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}
