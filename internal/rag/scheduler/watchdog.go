package scheduler

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
)

// ScanStuckAttempts is the only crash-recovery path: without it, a
// worker SIGKILL leaves the attempt row IN_PROGRESS and the ingest scan
// predicate (which excludes sources with an in-flight attempt) will
// never re-enqueue.
func ScanStuckAttempts(ctx context.Context, db *gorm.DB, cfg Config) (int, error) {
	cutoff := time.Now().Add(-cfg.WatchdogTimeout)
	const errMsg = "watchdog: attempt exceeded heartbeat timeout"

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
