package sandbox

import (
	"context"

	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/model"
)

func (o *Orchestrator) RunHealthCheck(ctx context.Context) {
	var sandboxes []model.Sandbox
	if err := o.db.WithContext(ctx).Where("status = 'running'").Find(&sandboxes).Error; err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "health check: failed to query sandboxes", "error", err)
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
		logging.FromContext(ctx).DebugContext(ctx, "health check: status changed", "sandbox_id", sb.ID, "old", sb.Status, "new", providerStatus)
		o.db.WithContext(ctx).Model(sb).Update("status", providerStatus)
		sb.Status = providerStatus
	}
}
