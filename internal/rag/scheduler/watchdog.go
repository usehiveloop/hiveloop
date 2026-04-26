package scheduler

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
)

// ScanStuckAttempts marks every in-progress RAGIndexAttempt whose
// last_progress_time is older than cfg.WatchdogTimeout as FAILED with a
// terse error message. This is the only crash-recovery path: without it,
// a worker SIGKILL leaves the attempt row IN_PROGRESS and the source's
// ingest scan will never re-enqueue (because the predicate excludes
// sources with an in-flight attempt).
//
// Port of Onyx monitor_indexing_attempt_progress at
// backend/onyx/background/celery/tasks/docprocessing/tasks.py:294-385,
// stall logic at tasks.py:419-465. The Onyx version uses Redis fencing
// + Celery task IDs; we use the heartbeat columns already on the row
// (LastProgressTime, etc.).
//
// Returns the number of attempts that were transitioned to FAILED.
func ScanStuckAttempts(ctx context.Context, db *gorm.DB, cfg Config) (int, error) {
	cutoff := time.Now().Add(-cfg.WatchdogTimeout)
	const errMsg = "watchdog: attempt exceeded heartbeat timeout"

	// The partial index idx_rag_index_attempt_heartbeat
	// (status='in_progress', last_progress_time) makes this UPDATE a
	// single-index scan even at high attempt-history volume.
	res := db.WithContext(ctx).
		Model(&ragmodel.RAGIndexAttempt{}).
		Where("status = ?", ragmodel.IndexingStatusInProgress).
		Where("(last_progress_time IS NOT NULL AND last_progress_time < ?) OR "+
			"(last_progress_time IS NULL AND time_created < ?)", cutoff, cutoff).
		Updates(map[string]any{
			"status":       ragmodel.IndexingStatusFailed,
			"error_msg":    errMsg,
			"time_updated": time.Now(),
		})
	if res.Error != nil {
		return 0, fmt.Errorf("watchdog: update stuck attempts: %w", res.Error)
	}
	return int(res.RowsAffected), nil
}
