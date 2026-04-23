package sandbox

import (
	"context"
	"log/slog"
	"time"

	"github.com/usehiveloop/hiveloop/internal/model"
)

func (o *Orchestrator) RunHealthCheck(ctx context.Context) {
	var sandboxes []model.Sandbox
	if err := o.db.Where("status = 'running'").Find(&sandboxes).Error; err != nil {
		slog.Error("health check: failed to query sandboxes", "error", err)
		return
	}

	for i := range sandboxes {
		sb := &sandboxes[i]
		o.checkSandboxHealth(ctx, sb)
	}
}

func (o *Orchestrator) checkSandboxHealth(ctx context.Context, sb *model.Sandbox) {
	status, err := o.provider.GetStatus(ctx, sb.ExternalID)
	if err != nil {
		return
	}

	providerStatus := string(status)
	if providerStatus != sb.Status {
		slog.Info("health check: status changed", "sandbox_id", sb.ID, "old", sb.Status, "new", providerStatus)
		o.db.Model(sb).Update("status", providerStatus)
		sb.Status = providerStatus
	}

	if sb.Status == "error" && sb.SandboxType == "shared" {
		o.handleSharedSandboxError(sb)
		return
	}

	if sb.SandboxType == "system" {
		if sb.Status == "stopped" {
			slog.Warn("system sandbox is stopped, attempting wake", "sandbox_id", sb.ID)
			if _, err := o.WakeSandbox(ctx, sb); err != nil {
				slog.Error("failed to wake system sandbox", "sandbox_id", sb.ID, "error", err)
			}
		}
		return
	}

	if sb.Status != "running" || sb.LastActiveAt == nil {
		return
	}

	idleMinutes := time.Since(*sb.LastActiveAt).Minutes()

	if sb.SandboxType == "shared" {
		var agentCount int64
		o.db.Model(&model.Agent{}).Where("sandbox_id = ?", sb.ID).Count(&agentCount)
		if agentCount > 0 {
			return
		}
		threshold := o.cfg.PoolSandboxIdleTimeoutMins
		if threshold > 0 && int(idleMinutes) >= threshold {
			slog.Info("health check: auto-stopping empty shared sandbox",
				"sandbox_id", sb.ID, "idle_mins", int(idleMinutes))
			if err := o.StopSandbox(ctx, sb); err != nil {
				slog.Error("health check: failed to stop shared sandbox", "sandbox_id", sb.ID, "error", err)
			}
		}
	}
}

func (o *Orchestrator) handleSharedSandboxError(sb *model.Sandbox) {
	result := o.db.Model(&model.Agent{}).
		Where("sandbox_id = ?", sb.ID).
		Update("sandbox_id", nil)
	if result.RowsAffected > 0 {
		slog.Warn("unassigned agents from errored shared sandbox",
			"sandbox_id", sb.ID, "agents_affected", result.RowsAffected)
	}
}
