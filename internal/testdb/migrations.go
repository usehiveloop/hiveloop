package testdb

import (
	"testing"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/migrations"
)

func ApplyMigrations(t testing.TB, db *gorm.DB) {
	t.Helper()

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql db: %v", err)
	}
	if _, err := migrations.Up(t.Context(), sqlDB); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
}
