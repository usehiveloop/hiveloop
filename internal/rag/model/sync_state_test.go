package model_test

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
	"github.com/usehiveloop/hiveloop/internal/rag/testhelpers"
)

// migrate1C runs AutoMigrate1C exactly once per test binary. Tranche 1C
// is not yet wired into rag.AutoMigrate (that happens in 1F), so tests
// must migrate their own tables. Idempotence is already exercised by
// AutoMigrate1C's internal guards; re-running is safe.
func migrate1C(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := ragmodel.AutoMigrate1C(db); err != nil {
		t.Fatalf("AutoMigrate1C: %v", err)
	}
}

// isUniqueViolation pins on the actual Postgres error code 23505 so we
// don't false-positive on other integrity failures (e.g. FK, check).
func isUniqueViolation(err error) bool {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		return pg.Code == "23505"
	}
	return false
}

func TestRAGSyncState_UniquePerInConnection(t *testing.T) {
	db := testhelpers.ConnectTestDB(t)
	migrate1C(t, db)

	org := testhelpers.NewTestOrg(t, db)
	user := testhelpers.NewTestUser(t, db, org.ID)
	integ := testhelpers.NewTestInIntegration(t, db, "github")
	conn := testhelpers.NewTestInConnection(t, db, org.ID, user.ID, integ.ID)

	t.Cleanup(func() {
		db.Where("in_connection_id = ?", conn.ID).Delete(&ragmodel.RAGSyncState{})
	})

	first := &ragmodel.RAGSyncState{
		OrgID:          org.ID,
		InConnectionID: conn.ID,
		Status:         ragmodel.RAGConnectionStatusActive,
		AccessType:     ragmodel.AccessTypePrivate,
		ProcessingMode: ragmodel.ProcessingModeRegular,
	}
	if err := db.Create(first).Error; err != nil {
		t.Fatalf("first insert should succeed: %v", err)
	}

	second := &ragmodel.RAGSyncState{
		OrgID:          org.ID,
		InConnectionID: conn.ID,
		Status:         ragmodel.RAGConnectionStatusPaused,
		AccessType:     ragmodel.AccessTypePrivate,
		ProcessingMode: ragmodel.ProcessingModeRegular,
	}
	err := db.Create(second).Error
	if err == nil {
		t.Fatal("second insert for same in_connection_id must violate unique constraint")
	}
	if !isUniqueViolation(err) {
		t.Fatalf("expected 23505 unique_violation, got: %v", err)
	}
}

func TestRAGSyncState_InConnectionCascade(t *testing.T) {
	db := testhelpers.ConnectTestDB(t)
	migrate1C(t, db)

	org := testhelpers.NewTestOrg(t, db)
	user := testhelpers.NewTestUser(t, db, org.ID)
	integ := testhelpers.NewTestInIntegration(t, db, "notion")
	conn := testhelpers.NewTestInConnection(t, db, org.ID, user.ID, integ.ID)

	state := &ragmodel.RAGSyncState{
		OrgID:          org.ID,
		InConnectionID: conn.ID,
		Status:         ragmodel.RAGConnectionStatusScheduled,
		AccessType:     ragmodel.AccessTypePublic,
		ProcessingMode: ragmodel.ProcessingModeRegular,
	}
	if err := db.Create(state).Error; err != nil {
		t.Fatalf("create sync state: %v", err)
	}

	// Delete the InConnection directly; the FK cascade we installed in
	// AutoMigrate1C should take out the sync state row with it.
	if err := db.Exec(`DELETE FROM in_connections WHERE id = ?`, conn.ID).Error; err != nil {
		t.Fatalf("delete in_connection: %v", err)
	}

	var count int64
	if err := db.Model(&ragmodel.RAGSyncState{}).
		Where("in_connection_id = ?", conn.ID).
		Count(&count).Error; err != nil {
		t.Fatalf("count sync_states: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected sync state row to be cascade-deleted; %d survived", count)
	}
}

// TestRAGConnectionStatus_IsActive — table-driven coverage of the
// scheduler's "should I run this loop iteration?" gate.
//
// Onyx reference: backend/onyx/db/enums.py:203-204.
func TestRAGConnectionStatus_IsActive(t *testing.T) {
	cases := []struct {
		status ragmodel.RAGConnectionStatus
		want   bool
	}{
		{ragmodel.RAGConnectionStatusScheduled, true},
		{ragmodel.RAGConnectionStatusInitialIndexing, true},
		{ragmodel.RAGConnectionStatusActive, true},
		{ragmodel.RAGConnectionStatusPaused, false},
		{ragmodel.RAGConnectionStatusDeleting, false},
		{ragmodel.RAGConnectionStatusInvalid, false},
	}
	for _, tc := range cases {
		t.Run(string(tc.status), func(t *testing.T) {
			if got := tc.status.IsActive(); got != tc.want {
				t.Fatalf("%s.IsActive() = %v, want %v", tc.status, got, tc.want)
			}
		})
	}
}

// TestRAGConnectionStatus_IndexableStatuses pins the Onyx invariant
// (enums.py:196-201) that IndexableStatuses = ActiveStatuses ∪ {PAUSED}.
//
// Why it matters: the model-swap code path in Onyx uses IndexableStatuses
// to decide which connectors must be re-indexed; paused connectors still
// have to migrate their vectors even though they're not actively
// scheduling new work.
func TestRAGConnectionStatus_IndexableStatuses(t *testing.T) {
	indexable := ragmodel.IndexableStatuses()
	active := ragmodel.ActiveStatuses()

	// Must contain every active status.
	for _, a := range active {
		if !containsStatus(indexable, a) {
			t.Fatalf("IndexableStatuses missing active status %s", a)
		}
	}
	// Must contain PAUSED.
	if !containsStatus(indexable, ragmodel.RAGConnectionStatusPaused) {
		t.Fatalf("IndexableStatuses missing PAUSED")
	}
	// Must NOT contain DELETING or INVALID (they are not indexable).
	for _, bad := range []ragmodel.RAGConnectionStatus{
		ragmodel.RAGConnectionStatusDeleting,
		ragmodel.RAGConnectionStatusInvalid,
	} {
		if containsStatus(indexable, bad) {
			t.Fatalf("IndexableStatuses must not contain %s", bad)
		}
	}
	// Cardinality pin: exactly ActiveStatuses + 1 (PAUSED).
	if len(indexable) != len(active)+1 {
		t.Fatalf("IndexableStatuses len = %d, want %d", len(indexable), len(active)+1)
	}

	// ActiveStatuses must return a fresh slice — mutating it must not
	// affect subsequent calls. This is the "fresh copy" invariant of
	// the port.
	before := ragmodel.ActiveStatuses()
	before[0] = ragmodel.RAGConnectionStatusInvalid
	after := ragmodel.ActiveStatuses()
	if after[0] != ragmodel.RAGConnectionStatusActive {
		t.Fatalf("ActiveStatuses returned a shared slice — caller mutation leaked into subsequent call")
	}
}

func containsStatus(xs []ragmodel.RAGConnectionStatus, want ragmodel.RAGConnectionStatus) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

