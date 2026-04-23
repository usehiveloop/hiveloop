package sandbox

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/usehiveloop/hiveloop/internal/model"
)

func (o *Orchestrator) WakeSandbox(ctx context.Context, sb *model.Sandbox) (*model.Sandbox, error) {
	if err := o.provider.StartSandbox(ctx, sb.ExternalID); err != nil {
		return nil, fmt.Errorf("starting sandbox %s: %w", sb.ID, err)
	}

	if err := o.refreshBridgeURL(ctx, sb); err != nil {
		return nil, fmt.Errorf("refreshing bridge URL after wake: %w", err)
	}

	now := time.Now()
	o.db.Model(sb).Updates(map[string]any{
		"status":         "running",
		"last_active_at": now,
		"stopped_at":     nil,
		"error_message":  nil,
	})
	sb.Status = "running"
	sb.LastActiveAt = &now
	sb.StoppedAt = nil

	if err := o.waitForBridgeHealthy(ctx, sb); err != nil {
		o.db.Model(sb).Updates(map[string]any{
			"status":        "error",
			"error_message": fmt.Sprintf("bridge not healthy after wake: %v", err),
		})
		return nil, fmt.Errorf("bridge not healthy after wake: %w", err)
	}

	slog.Info("sandbox woken", "sandbox_id", sb.ID, "external_id", sb.ExternalID)
	return sb, nil
}
