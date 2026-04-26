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

// startHeartbeat launches a goroutine that periodically updates the
// last_heartbeat_time / heartbeat_counter columns on the supplied
// RAGIndexAttempt, plus last_progress_time when the caller reports any
// new completed work via touchProgress(). Returns a stop function that
// blocks until the goroutine has exited so callers can guarantee no
// further writes after they finalise the row.
//
// Direct port of the heartbeat loop in Onyx's docfetching_proxy_task at
// backend/onyx/background/celery/tasks/docfetching/tasks.py:312-682.
// Onyx uses Redis fencing for the same role; we use the columns
// already on the row (the watchdog reads last_progress_time directly).
//
// The stop function is idempotent — safe to call from defer plus an
// explicit success path.
type heartbeatHandle struct {
	progress  atomic.Bool
	stopFn    func()
	stoppedCh chan struct{}
}

// touchProgress signals that some forward progress was made (a batch
// completed, a document was upserted). The next heartbeat tick will
// include `last_progress_time = NOW()` in its UPDATE.
func (h *heartbeatHandle) touchProgress() { h.progress.Store(true) }

// stop terminates the heartbeat goroutine and waits for it to exit.
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

// writeHeartbeat issues the UPDATE that bumps the heartbeat columns.
// When progress is true, last_progress_time is also bumped — that's
// the column the watchdog reads. The two are decoupled so a
// long-running fetch with no per-doc progress still surfaces as
// "alive" via heartbeat_counter, and the operator can distinguish
// "stalled producer" from "dead worker".
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
