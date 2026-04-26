package scheduler_test

import (
	"testing"
	"time"

	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
	"github.com/usehiveloop/hiveloop/internal/rag/scheduler"
)

// TestIngestScan_EnqueuesWhenDue exercises the happy path: a source
// with a refresh frequency older than its last index time should
// produce one rag:ingest task on the work queue.
func TestIngestScan_EnqueuesWhenDue(t *testing.T) {
	f := setupScheduler(t)
	refresh := 60
	f.makeSource(t,
		withRefresh(&refresh),
		withLastIndex(minutesAgo(5)),
	)

	n, err := scheduler.ScanIngestDue(ctxBg(), f.DB, f.Enq, f.Cfg)
	if err != nil {
		t.Fatalf("ScanIngestDue: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 enqueue, got %d", n)
	}
	if got := f.queueDepth(t); got != 1 {
		t.Fatalf("queue depth = %d, want 1", got)
	}
}

// TestIngestScan_SkipsNotDue: last index time is recent enough to be
// inside the refresh window — no enqueue.
func TestIngestScan_SkipsNotDue(t *testing.T) {
	f := setupScheduler(t)
	refresh := 3600
	f.makeSource(t,
		withRefresh(&refresh),
		withLastIndex(time.Now().Add(-30*time.Second)),
	)

	n, err := scheduler.ScanIngestDue(ctxBg(), f.DB, f.Enq, f.Cfg)
	if err != nil {
		t.Fatalf("ScanIngestDue: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 enqueues, got %d", n)
	}
}

// TestIngestScan_SkipsDisabled: an admin-disabled source must not be
// scheduled, regardless of how overdue it is.
func TestIngestScan_SkipsDisabled(t *testing.T) {
	f := setupScheduler(t)
	refresh := 60
	f.makeSource(t,
		withEnabled(false),
		withRefresh(&refresh),
		withLastIndex(minutesAgo(60)),
	)

	n, err := scheduler.ScanIngestDue(ctxBg(), f.DB, f.Enq, f.Cfg)
	if err != nil {
		t.Fatalf("ScanIngestDue: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 enqueues, got %d", n)
	}
}

// TestIngestScan_SkipsPaused: a PAUSED source is parked — admin took
// it out of rotation, scheduler must respect that.
func TestIngestScan_SkipsPaused(t *testing.T) {
	f := setupScheduler(t)
	refresh := 60
	f.makeSource(t,
		withStatus(ragmodel.RAGSourceStatusPaused),
		withRefresh(&refresh),
		withLastIndex(minutesAgo(60)),
	)

	n, err := scheduler.ScanIngestDue(ctxBg(), f.DB, f.Enq, f.Cfg)
	if err != nil {
		t.Fatalf("ScanIngestDue: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 enqueues, got %d", n)
	}
}

// TestIngestScan_SkipsInProgress: an in-flight RAGIndexAttempt
// suppresses re-scheduling. Without this gate, a duplicate ingest task
// would race the live one through the same checkpoint.
func TestIngestScan_SkipsInProgress(t *testing.T) {
	f := setupScheduler(t)
	refresh := 60
	src := f.makeSource(t,
		withRefresh(&refresh),
		withLastIndex(minutesAgo(60)),
	)
	now := time.Now()
	att := &ragmodel.RAGIndexAttempt{
		OrgID:            src.OrgIDValue,
		RAGSourceID:      src.ID,
		Status:           ragmodel.IndexingStatusInProgress,
		LastProgressTime: &now,
	}
	if err := f.DB.Create(att).Error; err != nil {
		t.Fatalf("create attempt: %v", err)
	}
	t.Cleanup(func() { f.DB.Where("id = ?", att.ID).Delete(&ragmodel.RAGIndexAttempt{}) })

	n, err := scheduler.ScanIngestDue(ctxBg(), f.DB, f.Enq, f.Cfg)
	if err != nil {
		t.Fatalf("ScanIngestDue: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 enqueues, got %d", n)
	}
}

// TestIngestScan_NullRefreshFreqSkips: a source with refresh_freq IS
// NULL is on-demand only; the periodic loop must not schedule it.
// Mirrors Onyx's should_index() returning False.
func TestIngestScan_NullRefreshFreqSkips(t *testing.T) {
	f := setupScheduler(t)
	f.makeSource(t,
		withRefresh(nil),
		withLastIndex(minutesAgo(60)),
	)

	n, err := scheduler.ScanIngestDue(ctxBg(), f.DB, f.Enq, f.Cfg)
	if err != nil {
		t.Fatalf("ScanIngestDue: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 enqueues, got %d", n)
	}
}

// TestUniqueEnqueue_DedupesRepeatedScans: firing the scan five times
// in rapid succession must produce exactly one task on the queue.
// Asynq's Unique option keys on (typename, payload) — the source ID
// is in the payload, so identical scans collapse server-side.
func TestUniqueEnqueue_DedupesRepeatedScans(t *testing.T) {
	f := setupScheduler(t)
	refresh := 60
	f.makeSource(t,
		withRefresh(&refresh),
		withLastIndex(minutesAgo(5)),
	)

	for i := 0; i < 5; i++ {
		_, err := scheduler.ScanIngestDue(ctxBg(), f.DB, f.Enq, f.Cfg)
		if err != nil {
			t.Fatalf("ScanIngestDue iter %d: %v", i, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
	if got := f.queueDepth(t); got != 1 {
		t.Fatalf("queue depth = %d after 5 scans, want 1 (Unique should dedupe)", got)
	}
}
