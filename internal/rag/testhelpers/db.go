package testhelpers

import (
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/testdb"
)

// ConnectTestDB opens a real Postgres connection, runs the core and
// RAG goose migrations steps, and registers `t.Cleanup` to close the
// underlying *sql.DB.
//
// A test that needs a test DB but the service isn't running should see
// a hard, loud failure — see rule 7 of TESTING.md.
func ConnectTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := testdb.DatabaseURL("HIVY_DATABASE_URL", "DATABASE_URL", "TEST_DATABASE_URL")

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

	testdb.ApplyMigrations(t, db)

	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}
