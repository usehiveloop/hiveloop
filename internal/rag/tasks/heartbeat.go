package tasks

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
)

// stop is idempotent — safe to call from defer plus an explicit
// success path. It blocks until the goroutine exits so callers can
// guarantee no further writes after they finalise the row.
type heartbeatHandle struct {
	progress  atomic.Bool
	stopFn    func()
	stoppedCh chan struct{}
}

func (h *heartbeatHandle) touchProgress() { h.progress.Store(true) }

func (h *heartbeatHandle) stop() {
	if h == nil || h.stopFn == nil {
		return
	}
	h.stopFn()
	<-h.stoppedCh
}

func startHeartbeat(
	ctx context.Context,
	db *gorm.DB,
	attemptID uuid.UUID,
	tick time.Duration,
) *heartbeatHandle {
	if tick <= 0 {
		tick = 30 * time.Second
	}
	h := &heartbeatHandle{stoppedCh: make(chan struct{})}
	hbCtx, cancel := context.WithCancel(ctx)
	h.stopFn = cancel

	go func() {
		defer close(h.stoppedCh)
		t := time.NewTicker(tick)
		defer t.Stop()
		for {
			select {
			case <-hbCtx.Done():
				return
			case <-t.C:
				writeHeartbeat(hbCtx, db, attemptID, h.progress.Swap(false))
			}
		}
	}()
	return h
}

// last_heartbeat_time and last_progress_time are decoupled so a
// long-running fetch with no per-doc progress still surfaces as alive
// via heartbeat_counter; the watchdog reads last_progress_time only.
func writeHeartbeat(ctx context.Context, db *gorm.DB, attemptID uuid.UUID, progress bool) {
	now := time.Now()
	updates := map[string]any{
		"last_heartbeat_time": now,
		"heartbeat_counter":   gorm.Expr("heartbeat_counter + 1"),
	}
	if progress {
		updates["last_progress_time"] = now
	}
	res := db.WithContext(ctx).
		Model(&ragmodel.RAGIndexAttempt{}).
		Where("id = ? AND status = ?", attemptID, ragmodel.IndexingStatusInProgress).
		Updates(updates)
	if res.Error != nil {
		slog.Warn("rag heartbeat write failed",
			"attempt_id", attemptID, "err", res.Error)
	}
}
