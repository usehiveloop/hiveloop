package testhelpers_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/rag/testhelpers"
)

// TestHarnessEndToEnd exercises the Phase 0 harness itself:
//   - ConnectTestDB opens real Postgres, runs both AutoMigrate passes, and
//     returns a working *gorm.DB.
//   - The fixture constructors write real rows.
//   - t.Cleanup ran and the org (and its membership) are gone after the
//     subtest that created them exits.
//
// We do NOT assert "the fields we wrote came back" — that only proves
// gorm works, which is banned by TESTING.md rule 3. Instead we verify
// the cross-harness contract: the DB connection is real, the IDs
// returned are real UUIDs (non-zero), and cleanup actually deletes
// what it created.
func TestHarnessEndToEnd(t *testing.T) {
	db := testhelpers.ConnectTestDB(t)

	var createdOrgID uuid.UUID

	t.Run("fixtures insert real rows", func(t *testing.T) {
		org := testhelpers.NewTestOrg(t, db)
		if org.ID == uuid.Nil {
			t.Fatal("NewTestOrg returned Org with zero UUID — Postgres did not generate one")
		}
		createdOrgID = org.ID

		user := testhelpers.NewTestUser(t, db, org.ID)
		if user.ID == uuid.Nil {
			t.Fatal("NewTestUser returned User with zero UUID")
		}

		integ := testhelpers.NewTestInIntegration(t, db, "github")
		if integ.ID == uuid.Nil {
			t.Fatal("NewTestInIntegration returned InIntegration with zero UUID")
		}

		conn := testhelpers.NewTestInConnection(t, db, org.ID, user.ID, integ.ID)
		if conn.ID == uuid.Nil {
			t.Fatal("NewTestInConnection returned InConnection with zero UUID")
		}

		// Sanity: the InConnection is actually visible via a fresh read —
		// i.e. the connection pool saw a real INSERT, not a transaction
		// we're about to roll back.
		var count int64
		if err := db.Model(&model.InConnection{}).Where("org_id = ?", org.ID).Count(&count).Error; err != nil {
			t.Fatalf("count InConnection for org: %v", err)
		}
		if count != 1 {
			t.Fatalf("expected 1 InConnection for created org, got %d", count)
		}
	})

	// The subtest exited; its t.Cleanup handlers must have deleted the
	// org. Verify.
	var remaining int64
	if err := db.Model(&model.Org{}).Where("id = ?", createdOrgID).Count(&remaining).Error; err != nil {
		t.Fatalf("post-cleanup count: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("t.Cleanup did not delete the created org (id=%s, still %d rows)", createdOrgID, remaining)
	}
}
