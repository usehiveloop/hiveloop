package integrations

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/testdb"
)

func connectIntegrationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := testdb.DatabaseURL("HIVY_DATABASE_URL", "DATABASE_URL", "TEST_DATABASE_URL")
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true})
	if err != nil {
		t.Fatalf("cannot connect to Postgres: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db handle: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	schema := "integrations_test_" + strings.ReplaceAll(uuid.NewString(), "-", "_")
	if err := db.Exec(`CREATE SCHEMA "` + schema + `"`).Error; err != nil {
		t.Fatalf("create schema: %v", err)
	}
	_ = sqlDB.Close()

	db, err = gorm.Open(postgres.Open(withSearchPath(t, dsn, schema)), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true})
	if err != nil {
		t.Fatalf("connect schema postgres: %v", err)
	}
	sqlDB, err = db.DB()
	if err != nil {
		t.Fatalf("schema db handle: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Exec(`DROP SCHEMA IF EXISTS "` + schema + `" CASCADE`).Error
		_ = sqlDB.Close()
	})
	testdb.ApplyMigrations(t, db)
	return db
}

func withSearchPath(t *testing.T, dsn string, schema string) string {
	t.Helper()
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse test dsn: %v", err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	return u.String()
}

func writeManifest(t *testing.T, dir string, body map[string]any) {
	t.Helper()
	payload, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, fmt.Sprint(body["id"])+".json"), payload, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}
