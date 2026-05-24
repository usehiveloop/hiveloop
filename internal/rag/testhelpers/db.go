package testhelpers

import (
	"os"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/testdb"
)

// testDBURL mirrors internal/middleware/integration_test.go:26 — the
// hivy_test database on the dev Postgres instance at localhost:15432.
const testDBURL = "postgres://hivy:localdev@localhost:15432/hivy_test?sslmode=disable" // #nosec G101 -- local test DB fixture

// ConnectTestDB opens a real Postgres connection, runs the core and
// RAG goose migrations steps, and registers `t.Cleanup` to close the
// underlying *sql.DB.
//
// A test that needs a test DB but the service isn't running should see
// a hard, loud failure — see rule 7 of TESTING.md.
func ConnectTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := os.Getenv("HIVY_DATABASE_URL")
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

	testdb.ApplyMigrations(t, db)

	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}
