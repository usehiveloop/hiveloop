package sandbox

import (
	"context"
	"log/slog"
	"time"

	"github.com/usehiveloop/hiveloop/internal/model"
)

const (
	sandboxIdleTimeoutMinutes  = 10
	sandboxArchiveAfterHours   = 24
)

func (o *Orchestrator) RunSandboxLifecycle(ctx context.Context) {
	now := time.Now()
	idleCutoff := now.Add(-time.Duration(sandboxIdleTimeoutMinutes) * time.Minute)
	archiveCutoff := now.Add(-time.Duration(sandboxArchiveAfterHours) * time.Hour)

	var idleRunning []model.Sandbox
	if err := o.db.Where(
		"status = ? AND sandbox_type != ? AND last_active_at IS NOT NULL AND last_active_at < ?",
		string(StatusRunning),
		"system",
		idleCutoff,
	).Find(&idleRunning).Error; err != nil {
		slog.Error("sandbox lifecycle: query idle running sandboxes failed", "error", err)
	} else {
		for i := range idleRunning {
			sb := &idleRunning[i]
			if sb.SandboxType == "shared" {
				var agentCount int64
				o.db.Model(&model.Agent{}).Where("sandbox_id = ?", sb.ID).Count(&agentCount)
				if agentCount > 0 {
					continue
				}
			}
			slog.Info("sandbox lifecycle: stopping idle sandbox",
				"sandbox_id", sb.ID,
				"external_id", sb.ExternalID,
				"idle_minutes", int(now.Sub(*sb.LastActiveAt).Minutes()),
			)
			if err := o.StopSandbox(ctx, sb); err != nil {
				slog.Error("sandbox lifecycle: failed to stop idle sandbox",
					"sandbox_id", sb.ID, "error", err)
			}
		}
	}

	var staleStopped []model.Sandbox
	if err := o.db.Where(
		"status = ? AND sandbox_type != ? AND stopped_at IS NOT NULL AND stopped_at < ?",
		string(StatusStopped),
		"system",
		archiveCutoff,
	).Find(&staleStopped).Error; err != nil {
		slog.Error("sandbox lifecycle: query stale stopped sandboxes failed", "error", err)
	} else {
		for i := range staleStopped {
			sb := &staleStopped[i]
			slog.Info("sandbox lifecycle: archiving stale stopped sandbox",
				"sandbox_id", sb.ID,
				"external_id", sb.ExternalID,
				"stopped_hours", int(now.Sub(*sb.StoppedAt).Hours()),
			)
			if err := o.ArchiveSandbox(ctx, sb); err != nil {
				slog.Error("sandbox lifecycle: failed to archive stopped sandbox",
					"sandbox_id", sb.ID, "error", err)
			}
		}
	}
}
