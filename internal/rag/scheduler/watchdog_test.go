package scheduler_test

import (
	"testing"
	"time"

	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
	"github.com/usehiveloop/hiveloop/internal/rag/scheduler"
)

// Without crash-recovery, the source's ingest scan is permanently
// blocked by the in-flight predicate.
func TestWatchdog_FailsStaleAttempts(t *testing.T) {
	f := setupScheduler(t)
	src := f.makeSource(t)
	stale := time.Now().Add(-31 * time.Minute)
	att := &ragmodel.RAGIndexAttempt{
		OrgID:            src.OrgIDValue,
		RAGSourceID:      src.ID,
		Status:           ragmodel.IndexingStatusInProgress,
		LastProgressTime: &stale,
	}
	if err := f.DB.Create(att).Error; err != nil {
		t.Fatalf("create attempt: %v", err)
	}
	t.Cleanup(func() { f.DB.Where("id = ?", att.ID).Delete(&ragmodel.RAGIndexAttempt{}) })

	n, err := scheduler.ScanStuckAttempts(ctxBg(), f.DB, f.Cfg)
	if err != nil {
		t.Fatalf("ScanStuckAttempts: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 attempt failed, got %d", n)
	}

	var got ragmodel.RAGIndexAttempt
	if err := f.DB.First(&got, "id = ?", att.ID).Error; err != nil {
		t.Fatalf("reload attempt: %v", err)
	}
	if got.Status != ragmodel.IndexingStatusFailed {
		t.Fatalf("attempt status = %s, want %s", got.Status, ragmodel.IndexingStatusFailed)
	}
	if got.ErrorMsg == nil || *got.ErrorMsg == "" {
		t.Fatalf("expected error_msg to be populated by watchdog")
	}
}

func TestWatchdog_LeavesFreshAttemptsAlone(t *testing.T) {
	f := setupScheduler(t)
	src := f.makeSource(t)
	fresh := time.Now().Add(-1 * time.Minute)
	att := &ragmodel.RAGIndexAttempt{
		OrgID:            src.OrgIDValue,
		RAGSourceID:      src.ID,
		Status:           ragmodel.IndexingStatusInProgress,
		LastProgressTime: &fresh,
	}
	if err := f.DB.Create(att).Error; err != nil {
		t.Fatalf("create attempt: %v", err)
	}
	t.Cleanup(func() { f.DB.Where("id = ?", att.ID).Delete(&ragmodel.RAGIndexAttempt{}) })

	n, err := scheduler.ScanStuckAttempts(ctxBg(), f.DB, f.Cfg)
	if err != nil {
		t.Fatalf("ScanStuckAttempts: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 attempts failed, got %d", n)
	}

	var got ragmodel.RAGIndexAttempt
	if err := f.DB.First(&got, "id = ?", att.ID).Error; err != nil {
		t.Fatalf("reload attempt: %v", err)
	}
	if got.Status != ragmodel.IndexingStatusInProgress {
		t.Fatalf("attempt status = %s, want unchanged %s", got.Status, ragmodel.IndexingStatusInProgress)
	}
}
