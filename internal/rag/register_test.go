package rag_test

import (
	"testing"

	"github.com/usehiveloop/hiveloop/internal/rag/testhelpers"
)

// TestAutoMigrate_WiredIntoHarness proves that:
//   - testhelpers.ConnectTestDB reaches real Postgres,
//   - model.AutoMigrate runs,
//   - and rag.AutoMigrate (called from within model.AutoMigrate) also
//     runs — a no-op in Phase 0 but the wiring must already be live
//     before any Phase 1 tranche can append real migrations.
//
// If either migration fails, ConnectTestDB calls t.Fatalf and this
// test fails loudly, per TESTING.md rule 7.
func TestAutoMigrate_WiredIntoHarness(t *testing.T) {
	db := testhelpers.ConnectTestDB(t)
	if db == nil {
		t.Fatal("ConnectTestDB returned nil *gorm.DB with no error")
	}
}
