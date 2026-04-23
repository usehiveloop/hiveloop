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

