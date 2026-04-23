package model_test

import (
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehiveloop/hiveloop/internal/rag/model"
	"github.com/usehiveloop/hiveloop/internal/rag/testhelpers"
)

// setupExternalGroupSchema opens the test DB (which migrates the full
// RAG schema) plus per-test org / user / connection / source fixtures
// that every test in this file needs. The returned source ID is a FK
// target for the three external-group tables.
func setupExternalGroupSchema(t *testing.T) (*gorm.DB, uuid.UUID, uuid.UUID, uuid.UUID) {
	t.Helper()
	db := testhelpers.ConnectTestDB(t)
	org := testhelpers.NewTestOrg(t, db)
	user := testhelpers.NewTestUser(t, db, org.ID)
	integ := testhelpers.NewTestInIntegration(t, db, "github")
	conn := testhelpers.NewTestInConnection(t, db, org.ID, user.ID, integ.ID)
	src := testhelpers.NewTestRAGSource(t, db, org.ID, conn.ID)
	// Belt-and-suspenders cleanup of 1D tables for this source.
	t.Cleanup(func() {
		db.Where("rag_source_id = ?", src.ID).Delete(&model.RAGExternalUserGroup{})
		db.Where("rag_source_id = ?", src.ID).Delete(&model.RAGUserExternalUserGroup{})
		db.Where("rag_source_id = ?", src.ID).Delete(&model.RAGPublicExternalUserGroup{})
	})
	return db, org.ID, user.ID, src.ID
}

// ---------------------------------------------------------------------
// RAGExternalUserGroup
// ---------------------------------------------------------------------

func TestRAGExternalUserGroup_UniquePerConnection(t *testing.T) {
	db, orgID, _, connID := setupExternalGroupSchema(t)

	base := model.RAGExternalUserGroup{
		OrgID:               orgID,
		RAGSourceID:         connID,
		ExternalUserGroupID: "github_backend",
		DisplayName:         "Backend",
		MemberEmails:        pq.StringArray{"a@x.com"},
		UpdatedAt:           time.Now(),
	}

	// First insert must succeed.
	if err := db.Create(&base).Error; err != nil {
		t.Fatalf("first insert: %v", err)
	}

	// Same (rag_source_id, external_user_group_id) must violate
	// uq_rag_external_user_group_conn_ext.
	dup := model.RAGExternalUserGroup{
		OrgID:               orgID,
		RAGSourceID:         connID,
		ExternalUserGroupID: "github_backend",
		DisplayName:         "Backend (duplicate)",
		UpdatedAt:           time.Now(),
	}
	err := db.Create(&dup).Error
	if err == nil {
		t.Fatal("expected unique-violation on duplicate (rag_source_id, external_user_group_id), got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "duplicate") && !strings.Contains(strings.ToLower(err.Error()), "unique") {
		t.Fatalf("expected a duplicate/unique error, got %v", err)
	}
}

func TestRAGExternalUserGroup_RAGSourceCascade(t *testing.T) {
	db, orgID, _, connID := setupExternalGroupSchema(t)

	row := model.RAGExternalUserGroup{
		OrgID:               orgID,
		RAGSourceID:         connID,
		ExternalUserGroupID: "github_frontend",
		DisplayName:         "Frontend",
		UpdatedAt:           time.Now(),
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	// Delete the RAGSource row directly. Cascade should drop the
	// external-user-group row.
	if err := db.Exec(`DELETE FROM rag_sources WHERE id = ?`, connID).Error; err != nil {
		t.Fatalf("delete RAGSource: %v", err)
	}

	var remaining int64
	if err := db.Model(&model.RAGExternalUserGroup{}).
		Where("id = ?", row.ID).
		Count(&remaining).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("expected cascade delete to leave 0 rows, got %d", remaining)
	}
}

// ---------------------------------------------------------------------
// RAGUserExternalUserGroup — composite PK + stale-sweep
// ---------------------------------------------------------------------

func TestRAGUserExternalUserGroup_CompositePK(t *testing.T) {
	db, _, userID, connID := setupExternalGroupSchema(t)

	row := model.RAGUserExternalUserGroup{
		UserID:              userID,
		ExternalUserGroupID: "github_backend",
		RAGSourceID:         connID,
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("first insert: %v", err)
	}
	// Same triple must collide on the composite PK.
	err := db.Create(&row).Error
	if err == nil {
		t.Fatal("expected PK violation on duplicate (user_id, external_user_group_id, rag_source_id), got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "duplicate") && !strings.Contains(strings.ToLower(err.Error()), "unique") {
		t.Fatalf("expected a duplicate/unique error, got %v", err)
	}
}

// TestRAGUserExternalUserGroup_StaleSweepPattern — SECURITY CRITICAL.
//
// Pins the three-step pattern used by the external-group sync loop.
// If this test ever starts passing while the real sweep is broken, or
// starts failing because the sweep semantics changed, STOP — users
// will see results from deleted groups.
//
// Scenario:
//  1. Seed 3 rows with stale=false (representing a prior sync snapshot).
//  2. Begin sync: bulk UPDATE sets stale=true for all rows in the
//     connection, marking them as "not yet refreshed".
//  3. Body: upsert 2 rows — one overlapping an existing row (resets
//     stale=false), one entirely new (stale=false).
//  4. End sync: DELETE rows where stale=true — sweeps the single
//     untouched row.
// Result: exactly the 2 upserted rows survive, all with stale=false.
