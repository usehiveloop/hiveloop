package handler_test

import (
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/testdb"
)

func connectFreshMigratedTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	baseDSN := testdb.DatabaseURL("DATABASE_URL", "HIVY_DATABASE_URL", "TEST_DATABASE_URL")
	dbName := "hivy_test_codex_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	maintenanceDSN, testDSN := testDatabaseDSNs(t, baseDSN, dbName)

	maintenanceDB, err := gorm.Open(postgres.Open(maintenanceDSN), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect maintenance database: %v", err)
	}
	maintenanceSQL, err := maintenanceDB.DB()
	if err != nil {
		t.Fatalf("maintenance sql db: %v", err)
	}

	if err := maintenanceDB.Exec(`CREATE DATABASE ` + dbName).Error; err != nil {
		_ = maintenanceSQL.Close()
		t.Fatalf("create isolated test database: %v", err)
	}

	db, err := gorm.Open(postgres.Open(testDSN), &gorm.Config{})
	if err != nil {
		_ = maintenanceDB.Exec(`DROP DATABASE IF EXISTS ` + dbName).Error
		_ = maintenanceSQL.Close()
		t.Fatalf("connect isolated test database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		_ = maintenanceDB.Exec(`DROP DATABASE IF EXISTS ` + dbName).Error
		_ = maintenanceSQL.Close()
		t.Fatalf("isolated sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(3)
	sqlDB.SetMaxIdleConns(1)

	t.Cleanup(func() {
		_ = sqlDB.Close()
		_ = maintenanceDB.Exec(`DROP DATABASE IF EXISTS ` + dbName).Error
		_ = maintenanceSQL.Close()
	})
	testdb.ApplyMigrations(t, db)
	return db
}

func testDatabaseDSNs(t *testing.T, dsn, dbName string) (string, string) {
	t.Helper()
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse database url: %v", err)
	}
	maintenance := *u
	maintenance.Path = "/postgres"
	testDB := *u
	testDB.Path = "/" + dbName
	return maintenance.String(), testDB.String()
}
