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
func TestRAGUserExternalUserGroup_StaleSweepPattern(t *testing.T) {
	db, _, userID, connID := setupExternalGroupSchema(t)

	// 1. Seed the prior-sync snapshot.
	seed := []model.RAGUserExternalUserGroup{
		{UserID: userID, ExternalUserGroupID: "github_old_a", RAGSourceID: connID, Stale: false},
		{UserID: userID, ExternalUserGroupID: "github_old_b", RAGSourceID: connID, Stale: false},
		{UserID: userID, ExternalUserGroupID: "github_keep", RAGSourceID: connID, Stale: false},
	}
	for i := range seed {
		if err := db.Create(&seed[i]).Error; err != nil {
			t.Fatalf("seed[%d]: %v", i, err)
		}
	}

	// 2. Sync start: stale everything for this connection.
	if err := db.Exec(`
		UPDATE rag_user_external_user_groups
		SET stale = true
		WHERE rag_source_id = ?
	`, connID).Error; err != nil {
		t.Fatalf("bulk stale update: %v", err)
	}

	// All 3 rows should now be stale.
	var staleBefore int64
	db.Model(&model.RAGUserExternalUserGroup{}).
		Where("rag_source_id = ? AND stale = true", connID).
		Count(&staleBefore)
	if staleBefore != 3 {
		t.Fatalf("post-stale-update: expected 3 stale rows, got %d", staleBefore)
	}

	// 3. Body: upsert fresh rows with stale=false. One overlaps
	//    (github_keep), one is new (github_new). The overlapping row
	//    must flip back from stale=true to stale=false; the new row
	//    must be inserted with stale=false.
	fresh := []model.RAGUserExternalUserGroup{
		{UserID: userID, ExternalUserGroupID: "github_keep", RAGSourceID: connID, Stale: false},
		{UserID: userID, ExternalUserGroupID: "github_new", RAGSourceID: connID, Stale: false},
	}
	for i := range fresh {
		if err := db.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "user_id"},
				{Name: "external_user_group_id"},
				{Name: "rag_source_id"},
			},
			DoUpdates: clause.AssignmentColumns([]string{"stale"}),
		}).Create(&fresh[i]).Error; err != nil {
			t.Fatalf("upsert fresh[%d]: %v", i, err)
		}
	}

	// 4. Sync end: sweep remaining stale rows.
	if err := db.Exec(`
		DELETE FROM rag_user_external_user_groups
		WHERE rag_source_id = ? AND stale = true
	`, connID).Error; err != nil {
		t.Fatalf("sweep: %v", err)
	}

	// Verify the final state.
	var survivors []model.RAGUserExternalUserGroup
	if err := db.Where("rag_source_id = ?", connID).
		Order("external_user_group_id").
		Find(&survivors).Error; err != nil {
		t.Fatalf("read survivors: %v", err)
	}
	if len(survivors) != 2 {
		t.Fatalf("expected exactly 2 survivors, got %d: %+v", len(survivors), survivors)
	}
	gotIDs := []string{survivors[0].ExternalUserGroupID, survivors[1].ExternalUserGroupID}
	sort.Strings(gotIDs)
	if gotIDs[0] != "github_keep" || gotIDs[1] != "github_new" {
		t.Fatalf("expected [github_keep, github_new], got %v", gotIDs)
	}
	for _, s := range survivors {
		if s.Stale {
			t.Fatalf("survivor %q still stale=true; sweep invariant broken", s.ExternalUserGroupID)
		}
	}

	// Explicit: the old non-overlapping rows are gone.
	var oldRemaining int64
	db.Model(&model.RAGUserExternalUserGroup{}).
		Where("rag_source_id = ? AND external_user_group_id IN ?", connID, []string{"github_old_a", "github_old_b"}).
		Count(&oldRemaining)
	if oldRemaining != 0 {
		t.Fatalf("expected 0 of the old-stale rows to remain, got %d", oldRemaining)
	}
}

// ---------------------------------------------------------------------
// RAGPublicExternalUserGroup
// ---------------------------------------------------------------------

// TestRAGPublicExternalUserGroup_StaleSweepPattern — same pattern as
// the user-level sweep, on the public-group table. Public groups
// enforce access for anyone-with-the-link style shares; stale rows
// here leak public-document membership metadata across sync cycles.
func TestRAGPublicExternalUserGroup_StaleSweepPattern(t *testing.T) {
	db, _, _, connID := setupExternalGroupSchema(t)

	seed := []model.RAGPublicExternalUserGroup{
		{ExternalUserGroupID: "gdrive_public_a", RAGSourceID: connID, Stale: false},
		{ExternalUserGroupID: "gdrive_public_b", RAGSourceID: connID, Stale: false},
		{ExternalUserGroupID: "gdrive_public_keep", RAGSourceID: connID, Stale: false},
	}
	for i := range seed {
		if err := db.Create(&seed[i]).Error; err != nil {
			t.Fatalf("seed[%d]: %v", i, err)
		}
	}

	if err := db.Exec(`
		UPDATE rag_public_external_user_groups
		SET stale = true
		WHERE rag_source_id = ?
	`, connID).Error; err != nil {
		t.Fatalf("bulk stale: %v", err)
	}

	fresh := []model.RAGPublicExternalUserGroup{
		{ExternalUserGroupID: "gdrive_public_keep", RAGSourceID: connID, Stale: false},
		{ExternalUserGroupID: "gdrive_public_new", RAGSourceID: connID, Stale: false},
	}
	for i := range fresh {
		if err := db.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "external_user_group_id"},
				{Name: "rag_source_id"},
			},
			DoUpdates: clause.AssignmentColumns([]string{"stale"}),
		}).Create(&fresh[i]).Error; err != nil {
			t.Fatalf("upsert fresh[%d]: %v", i, err)
		}
	}

	if err := db.Exec(`
		DELETE FROM rag_public_external_user_groups
		WHERE rag_source_id = ? AND stale = true
	`, connID).Error; err != nil {
		t.Fatalf("sweep: %v", err)
	}

	var survivors []model.RAGPublicExternalUserGroup
	if err := db.Where("rag_source_id = ?", connID).
		Order("external_user_group_id").
		Find(&survivors).Error; err != nil {
		t.Fatalf("read survivors: %v", err)
	}
	if len(survivors) != 2 {
		t.Fatalf("expected exactly 2 survivors, got %d: %+v", len(survivors), survivors)
	}
	ids := []string{survivors[0].ExternalUserGroupID, survivors[1].ExternalUserGroupID}
	sort.Strings(ids)
	if ids[0] != "gdrive_public_keep" || ids[1] != "gdrive_public_new" {
		t.Fatalf("expected [gdrive_public_keep, gdrive_public_new], got %v", ids)
	}
	for _, s := range survivors {
		if s.Stale {
			t.Fatalf("survivor %q still stale=true", s.ExternalUserGroupID)
		}
	}
}

func TestRAGPublicExternalUserGroup_RAGSourceCascade(t *testing.T) {
	db, _, _, connID := setupExternalGroupSchema(t)

	row := model.RAGPublicExternalUserGroup{
		ExternalUserGroupID: "gdrive_public_share_x",
		RAGSourceID:         connID,
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := db.Exec(`DELETE FROM rag_sources WHERE id = ?`, connID).Error; err != nil {
		t.Fatalf("delete RAGSource: %v", err)
	}

	var remaining int64
	if err := db.Model(&model.RAGPublicExternalUserGroup{}).
		Where("external_user_group_id = ? AND rag_source_id = ?", "gdrive_public_share_x", connID).
		Count(&remaining).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("expected cascade delete to leave 0 rows, got %d", remaining)
	}
}
