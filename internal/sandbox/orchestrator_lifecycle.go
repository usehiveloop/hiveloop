package sandbox

import (
	"context"
	"errors"
	"time"

	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/model"
)

const (
	sandboxIdleTimeoutMinutes = 10
	sandboxArchiveAfterHours  = 24
)

func (o *Orchestrator) RunSandboxLifecycle(ctx context.Context) {
	now := time.Now()
	idleCutoff := now.Add(-time.Duration(sandboxIdleTimeoutMinutes) * time.Minute)
	archiveCutoff := now.Add(-time.Duration(sandboxArchiveAfterHours) * time.Hour)

	var idleRunning []model.Sandbox
	if err := o.db.Where(
		`status = ? AND last_active_at IS NOT NULL AND last_active_at < ?
		 AND NOT EXISTS (SELECT 1 FROM agents a WHERE a.id = sandboxes.agent_id AND a.harness IN ('hermes', 'employee-sandbox'))`,
		string(StatusRunning),
		idleCutoff,
	).Find(&idleRunning).Error; err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "sandbox lifecycle: query idle running sandboxes failed", "error", err)
	} else {
		for i := range idleRunning {
			sb := &idleRunning[i]
			if err := o.StopSandbox(ctx, sb); err != nil && !errors.Is(err, ErrSandboxNotFound) {
				logging.FromContext(ctx).WarnContext(ctx, "sandbox lifecycle: failed to stop idle sandbox",
					"sandbox_id", sb.ID, "error", err)
				logging.Capture(ctx, err)
			}
		}
	}

	var staleStopped []model.Sandbox
	if err := o.db.Where(
		`status = ? AND stopped_at IS NOT NULL AND stopped_at < ?
		 AND NOT EXISTS (SELECT 1 FROM agents a WHERE a.id = sandboxes.agent_id AND a.harness IN ('hermes', 'employee-sandbox'))`,
		string(StatusStopped),
		archiveCutoff,
	).Find(&staleStopped).Error; err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "sandbox lifecycle: query stale stopped sandboxes failed", "error", err)
	} else {
		for i := range staleStopped {
			sb := &staleStopped[i]
			if err := o.ArchiveSandbox(ctx, sb); err != nil && !errors.Is(err, ErrSandboxNotFound) {
				logging.FromContext(ctx).WarnContext(ctx, "sandbox lifecycle: failed to archive stopped sandbox",
					"sandbox_id", sb.ID, "error", err)
				logging.Capture(ctx, err)
			}
		}
	}
}
