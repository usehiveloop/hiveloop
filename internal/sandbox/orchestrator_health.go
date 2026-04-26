package sandbox

import (
	"context"
	"log/slog"

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
}
