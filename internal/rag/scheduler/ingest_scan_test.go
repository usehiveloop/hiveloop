package scheduler_test

import (
	"testing"
	"time"

	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
	"github.com/usehiveloop/hiveloop/internal/rag/scheduler"
)

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

// Without this gate, a duplicate ingest task would race the live one
// through the same checkpoint.
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

// refresh_freq IS NULL means on-demand only; the periodic loop must
// not schedule it.
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

// Asynq's Unique option keys on (typename, payload), so identical
// scans collapse to a single task on the queue.
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
