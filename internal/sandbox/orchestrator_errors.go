package sandbox

import (
	"context"

	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/model"
)

func (o *Orchestrator) markSandboxError(ctx context.Context, sb *model.Sandbox, updates map[string]any) {
	if err := o.db.Model(sb).Updates(updates).Error; err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "mark sandbox error",
			"error", err, "sandbox_id", sb.ID)
	}
}
