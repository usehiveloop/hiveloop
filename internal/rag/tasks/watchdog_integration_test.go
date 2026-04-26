package tasks_test

import (
	"context"
	"testing"
	"time"

	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
	"github.com/usehiveloop/hiveloop/internal/rag/scheduler"
)

// Verifies the full crash-recovery path: a cancelled run leaves the
// attempt IN_PROGRESS with a stale last_progress_time, and the
// watchdog flips it to FAILED so the next ingest scan can re-enqueue.
func TestWatchdogIntegration_PicksUpDeadWorker(t *testing.T) {
	f := setupTask(t)
	kind := nextStubKind()
	stub := &stubConnector{
		docs:         genDocs("dead", 10),
		delayBetween: 50 * time.Millisecond,
	}
	registerStub(kind, stub)
	src := f.makeSource(t, kind)

	ctx, cancel := context.WithCancel(context.Background())
	doneCh := make(chan error, 1)
	go func() {
		doneCh <- f.runIngestNow(ctx, t, src.ID)
	}()
	time.Sleep(120 * time.Millisecond)
	cancel()
	<-doneCh

	// Re-mark in_progress: the cancel path may have finalised the
	// attempt, but we want to assert the watchdog's behaviour against
	// the real-world crash signature where no finalisation ran.
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
