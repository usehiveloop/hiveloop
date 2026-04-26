package tasks_test

import (
	"context"
	"testing"
	"time"

	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
	"github.com/usehiveloop/hiveloop/internal/rag/scheduler"
)

// TestWatchdogIntegration_PicksUpDeadWorker exercises the full
// crash-recovery path: an ingest run starts, the goroutine driving it
// is cancelled mid-stream, the attempt is left IN_PROGRESS with a
// stale last_progress_time, and the watchdog scan converts it to
// FAILED so the next ingest scan can re-eligibility the source.
//
// We forge the staleness by manually backdating last_progress_time to
// before the watchdog timeout — exercising the same SQL path the real
// watchdog uses without sleeping for 30 minutes.
func TestWatchdogIntegration_PicksUpDeadWorker(t *testing.T) {
	f := setupTask(t)
	kind := nextStubKind()
	stub := &stubConnector{
		docs:         genDocs("dead", 10),
		delayBetween: 50 * time.Millisecond,
	}
	registerStub(kind, stub)
	src := f.makeSource(t, kind)

	// Start the handler with a context we can cancel mid-flight.
	ctx, cancel := context.WithCancel(context.Background())
	doneCh := make(chan error, 1)
	go func() {
		doneCh <- f.runIngestNow(ctx, t, src.ID)
	}()
	// Give the connector enough time to emit a few docs and create
	// the attempt row.
	time.Sleep(120 * time.Millisecond)
	cancel()
	<-doneCh

	// Find the attempt the cancelled run opened, force its progress
	// timestamp to "stale", and re-mark it as in_progress (the cancel
	// path may have left it in a finalised state — we want to test
	// the watchdog's behaviour against an attempt that the worker
	// never finalised, which is the real-world crash signature).
	att := reloadAttempt(t, f.DB, src.ID)
	stale := time.Now().Add(-90 * time.Minute)
	if err := f.DB.Model(&ragmodel.RAGIndexAttempt{}).
		Where("id = ?", att.ID).
		Updates(map[string]any{
			"status":             ragmodel.IndexingStatusInProgress,
			"last_progress_time": stale,
			"error_msg":          nil,
		}).Error; err != nil {
		t.Fatalf("force-stale attempt: %v", err)
	}

	cfg := scheduler.Config{WatchdogTimeout: 30 * time.Minute}
	n, err := scheduler.ScanStuckAttempts(context.Background(), f.DB, cfg)
	if err != nil {
		t.Fatalf("ScanStuckAttempts: %v", err)
	}
	if n < 1 {
		t.Fatalf("watchdog reaped %d, want >= 1", n)
	}
	final := reloadAttempt(t, f.DB, src.ID)
	if final.Status != ragmodel.IndexingStatusFailed {
		t.Fatalf("attempt status = %s, want FAILED after watchdog", final.Status)
	}
}
