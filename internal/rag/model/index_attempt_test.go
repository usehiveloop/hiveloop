package model_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
	"github.com/usehiveloop/hiveloop/internal/rag/testhelpers"
)

// ---------------------------------------------------------------------------
// Pure-logic tests — no DB, no mocks (nothing to mock).
// ---------------------------------------------------------------------------

// TestIndexingStatus_IsTerminal pins the terminal-vs-running partition
// the scheduler uses to decide whether an attempt can be retried.
// If this partition drifts, the indexing queue either stalls (false
// positive — scheduler thinks in-flight work is done) or thrashes
// (false negative — scheduler relaunches a finished attempt). Both
// are user-visible outages.
func TestIndexingStatus_IsTerminal(t *testing.T) {
	cases := []struct {
		status ragmodel.IndexingStatus
		want   bool
	}{
		{ragmodel.IndexingStatusNotStarted, false},
		{ragmodel.IndexingStatusInProgress, false},
		{ragmodel.IndexingStatusSuccess, true},
		{ragmodel.IndexingStatusCompletedWithErrors, true},
		{ragmodel.IndexingStatusCanceled, true},
		{ragmodel.IndexingStatusFailed, true},
		// Unknown string must NOT be treated as terminal — this is the
		// safe default (we'd rather leave a mystery attempt alone than
		// retry it).
		{ragmodel.IndexingStatus("bogus"), false},
	}
	for _, tc := range cases {
		t.Run(string(tc.status), func(t *testing.T) {
			if got := tc.status.IsTerminal(); got != tc.want {
				t.Fatalf("IsTerminal(%q) = %v, want %v", tc.status, got, tc.want)
			}
		})
	}
}

// TestIndexingStatus_IsSuccessful pins the "produced usable index
// output" partition — completed_with_errors counts as successful.
// A wrong answer here means either (a) we incorrectly mark a partially
// failed run as failed and invalidate its indexed docs, or (b) we
// treat a truly failed run as a success and leave the data stale.
func TestIndexingStatus_IsSuccessful(t *testing.T) {
	cases := []struct {
		status ragmodel.IndexingStatus
		want   bool
	}{
		{ragmodel.IndexingStatusSuccess, true},
		{ragmodel.IndexingStatusCompletedWithErrors, true},
		{ragmodel.IndexingStatusNotStarted, false},
		{ragmodel.IndexingStatusInProgress, false},
		{ragmodel.IndexingStatusCanceled, false},
		{ragmodel.IndexingStatusFailed, false},
		{ragmodel.IndexingStatus(""), false},
	}
	for _, tc := range cases {
		t.Run(string(tc.status), func(t *testing.T) {
			if got := tc.status.IsSuccessful(); got != tc.want {
				t.Fatalf("IsSuccessful(%q) = %v, want %v", tc.status, got, tc.want)
			}
		})
	}
}

// TestRAGIndexAttempt_IsCoordinationComplete pins the batch-coordination
// gate that signals "docprocessing can finalise". Four distinct branches
// must be exercised: nil TotalBatches (docfetching not done),
// CompletedBatches below total, exactly at total, and above total
// (the race case — completer increments after the finaliser wrote the
// final count).
func TestRAGIndexAttempt_IsCoordinationComplete(t *testing.T) {
	fiveP := func(n int) *int { return &n }

	cases := []struct {
		name             string
		totalBatches     *int
		completedBatches int
		want             bool
	}{
		{"nil total — fetcher still enumerating", nil, 0, false},
		{"nil total even though completed nonzero", nil, 7, false},
		{"5 of expected 5", fiveP(5), 5, true},
		{"4 of expected 5", fiveP(5), 4, false},
		{"6 of expected 5 — race case counts as complete", fiveP(5), 6, true},
		{"0 of expected 0 — trivial empty connector", fiveP(0), 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := &ragmodel.RAGIndexAttempt{
				TotalBatches:     tc.totalBatches,
				CompletedBatches: tc.completedBatches,
			}
			if got := a.IsCoordinationComplete(); got != tc.want {
				t.Fatalf("IsCoordinationComplete() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestRAGIndexAttempt_IsFinished proxies through IndexingStatus.IsTerminal;
// we pin the proxy explicitly so a future refactor that broke the
// delegation would be caught without relying on IsTerminal's test.
func TestRAGIndexAttempt_IsFinished(t *testing.T) {
	cases := []struct {
		status ragmodel.IndexingStatus
		want   bool
	}{
		{ragmodel.IndexingStatusInProgress, false},
		{ragmodel.IndexingStatusSuccess, true},
		{ragmodel.IndexingStatusFailed, true},
	}
	for _, tc := range cases {
		t.Run(string(tc.status), func(t *testing.T) {
			a := &ragmodel.RAGIndexAttempt{Status: tc.status}
			if got := a.IsFinished(); got != tc.want {
				t.Fatalf("IsFinished() for %q = %v, want %v", tc.status, got, tc.want)
			}
		})
	}
}

// TestSyncType_Valid pins the accepted SyncType set. Unknown values
// must be rejected — the coordinator dispatches jobs by SyncType
// value, so any extra value would route to a nonexistent handler.
func TestSyncType_Valid(t *testing.T) {
	cases := []struct {
		val  ragmodel.SyncType
		want bool
	}{
		{ragmodel.SyncTypeConnectorDeletion, true},
		{ragmodel.SyncTypePruning, true},
		{ragmodel.SyncTypeExternalPermissions, true},
		{ragmodel.SyncTypeExternalGroup, true},
		// Intentionally excluded — Hiveloop does not have these.
		{ragmodel.SyncType("document_set"), false},
		{ragmodel.SyncType("user_group"), false},
		{ragmodel.SyncType(""), false},
		{ragmodel.SyncType("definitely_not_a_sync_type"), false},
	}
	for _, tc := range cases {
		t.Run(string(tc.val), func(t *testing.T) {
			if got := tc.val.IsValid(); got != tc.want {
				t.Fatalf("IsValid(%q) = %v, want %v", tc.val, got, tc.want)
			}
		})
	}
}

// TestSyncStatus_IsTerminal pins Onyx's notion of sync terminality:
// SUCCESS and FAILED only. Note `canceled` is NOT terminal — it's a
// transition state while the canceller tears down resources (see
// Onyx backend/onyx/db/enums.py:119-125). Getting this wrong would
// cause the scheduler to re-spawn a sync that's mid-cancellation.
func TestSyncStatus_IsTerminal(t *testing.T) {
	cases := []struct {
		val  ragmodel.SyncStatus
		want bool
	}{
		{ragmodel.SyncStatusSuccess, true},
		{ragmodel.SyncStatusFailed, true},
		{ragmodel.SyncStatusInProgress, false},
		{ragmodel.SyncStatusCanceled, false},
		{ragmodel.SyncStatus(""), false},
	}
	for _, tc := range cases {
		t.Run(string(tc.val), func(t *testing.T) {
			if got := tc.val.IsTerminal(); got != tc.want {
				t.Fatalf("IsTerminal(%q) = %v, want %v", tc.val, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Postgres integration tests.
// ---------------------------------------------------------------------------

// setupIndexAttemptDB opens the test DB with the full RAG schema
// migrated. ConnectTestDB calls rag.AutoMigrate internally.
func setupIndexAttemptDB(t *testing.T) *gorm.DB {
	t.Helper()
	return testhelpers.ConnectTestDB(t)
}

// TestRAGIndexAttempt_HeartbeatPartialIndex confirms the watchdog
// query planner picks the partial `idx_rag_index_attempt_heartbeat`
// index instead of a seq scan. A partial index is only "worth having"
// if the planner actually uses it — this test catches the class of
// mistake where the WHERE clause in the index doesn't match the query
// predicate (Postgres then silently ignores it).
func TestRAGIndexAttempt_HeartbeatPartialIndex(t *testing.T) {
	db := setupIndexAttemptDB(t)
	ctx := context.Background()
	org := testhelpers.NewTestOrg(t, db)
	integ := testhelpers.NewTestInIntegration(t, db, "github")
	user := testhelpers.NewTestUser(t, db, org.ID)
	conn := testhelpers.NewTestInConnection(t, db, org.ID, user.ID, integ.ID)
	src := testhelpers.NewTestRAGSource(t, db, org.ID, conn.ID)

	// Three attempts: not_started, in_progress (old), success.
	oldProgress := time.Now().Add(-2 * time.Hour)
	attempts := []ragmodel.RAGIndexAttempt{
		{
			OrgID:       org.ID,
			RAGSourceID: src.ID,
			Status:      ragmodel.IndexingStatusNotStarted,
		},
		{
			OrgID:            org.ID,
			RAGSourceID:      src.ID,
			Status:           ragmodel.IndexingStatusInProgress,
			LastProgressTime: &oldProgress,
		},
		{
			OrgID:       org.ID,
			RAGSourceID: src.ID,
			Status:      ragmodel.IndexingStatusSuccess,
		},
	}
	for i := range attempts {
		if err := db.WithContext(ctx).Create(&attempts[i]).Error; err != nil {
			t.Fatalf("create attempt %d: %v", i, err)
		}
	}
	t.Cleanup(func() {
		db.Where("org_id = ?", org.ID).Delete(&ragmodel.RAGIndexAttempt{})
	})

	// ANALYZE so the planner has up-to-date stats for the partial
	// index selectivity estimate.
	if err := db.Exec("ANALYZE rag_index_attempts").Error; err != nil {
		t.Fatalf("ANALYZE: %v", err)
	}

	// With only a handful of seed rows the planner chooses a seq scan
	// because any index scan's startup cost dominates. That's fine at
	// 3-row test scale, NOT at production scale (tens of thousands of
	// historical attempts). We force the planner to evaluate
	// alternatives by disabling seq-scan for this one EXPLAIN — if the
	// partial index is shaped wrong (predicate mismatch, wrong column
	// order) the planner will fall back to a full index scan or a
	// bitmap heap with no index. We want to see a plan that explicitly
	// references the heartbeat index.
	//
	// `SET LOCAL` only takes effect inside a transaction so we wrap
	// the EXPLAIN in one. The Rollback at the end guarantees the
	// enable_seqscan override does not leak to other tests sharing
	// this connection pool.
	var raw []byte
	var plan []map[string]any
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SET LOCAL enable_seqscan = off").Error; err != nil {
			return err
		}
		return tx.Raw(`
			EXPLAIN (FORMAT JSON)
			SELECT id FROM rag_index_attempts
			WHERE status = 'in_progress'
			  AND last_progress_time < NOW() - INTERVAL '30 minutes'
		`).Row().Scan(&raw)
	})
	if err != nil {
		t.Fatalf("EXPLAIN: %v", err)
	}
	if err := json.Unmarshal(raw, &plan); err != nil {
		t.Fatalf("unmarshal plan: %v: %s", err, string(raw))
	}
	planStr := string(raw)
	if !strings.Contains(planStr, "idx_rag_index_attempt_heartbeat") {
		t.Fatalf("watchdog query did not use idx_rag_index_attempt_heartbeat\nplan: %s", planStr)
	}
}

// TestRAGIndexAttemptError_AttemptCascadeDelete verifies that deleting
// a parent RAGIndexAttempt removes its error rows via Postgres ON
// DELETE CASCADE. The error log is meaningless without its attempt
// context; orphaned rows would accumulate indefinitely and leak PII
// from deleted connections.
func TestRAGIndexAttemptError_AttemptCascadeDelete(t *testing.T) {
	db := setupIndexAttemptDB(t)
	ctx := context.Background()
	org := testhelpers.NewTestOrg(t, db)
	integ := testhelpers.NewTestInIntegration(t, db, "github")
	user := testhelpers.NewTestUser(t, db, org.ID)
	conn := testhelpers.NewTestInConnection(t, db, org.ID, user.ID, integ.ID)
	src := testhelpers.NewTestRAGSource(t, db, org.ID, conn.ID)

	attempt := &ragmodel.RAGIndexAttempt{
		OrgID:       org.ID,
		RAGSourceID: src.ID,
		Status:      ragmodel.IndexingStatusFailed,
	}
	if err := db.WithContext(ctx).Create(attempt).Error; err != nil {
		t.Fatalf("create attempt: %v", err)
	}
	errRow := &ragmodel.RAGIndexAttemptError{
		OrgID:          org.ID,
		IndexAttemptID: attempt.ID,
		RAGSourceID:    src.ID,
		FailureMessage: "boom",
	}
	if err := db.WithContext(ctx).Create(errRow).Error; err != nil {
		t.Fatalf("create error row: %v", err)
	}
	t.Cleanup(func() {
		db.Where("org_id = ?", org.ID).Delete(&ragmodel.RAGIndexAttemptError{})
		db.Where("org_id = ?", org.ID).Delete(&ragmodel.RAGIndexAttempt{})
	})

	// Sanity check that the error row exists pre-delete.
	var before int64
	if err := db.Model(&ragmodel.RAGIndexAttemptError{}).Where("id = ?", errRow.ID).Count(&before).Error; err != nil {
		t.Fatalf("pre-delete count: %v", err)
	}
	if before != 1 {
		t.Fatalf("expected 1 error row pre-delete, got %d", before)
	}

	// Delete the parent attempt — cascade should sweep the error row.
	if err := db.Delete(attempt).Error; err != nil {
		t.Fatalf("delete attempt: %v", err)
	}

	var after int64
	if err := db.Model(&ragmodel.RAGIndexAttemptError{}).Where("id = ?", errRow.ID).Count(&after).Error; err != nil {
		t.Fatalf("post-delete count: %v", err)
	}
	if after != 0 {
		t.Fatalf("expected cascade to delete error row; still %d left (id=%s)", after, errRow.ID)
	}
}

