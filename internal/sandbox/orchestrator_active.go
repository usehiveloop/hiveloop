package sandbox

import (
	"context"
	"fmt"
	"time"

	"github.com/usehivy/hivy/internal/model"
)

func (o *Orchestrator) EnsureSandboxActive(ctx context.Context, sb *model.Sandbox) (*model.Sandbox, error) {
	if err := o.ensureSandboxProvider(sb); err != nil {
		return nil, err
	}
	switch sb.Status {
	case string(StatusRunning):
		return sb, nil

	case string(StatusStopped):
		return o.WakeSandbox(ctx, sb)

	case string(StatusArchived), string(StatusArchiving):
		return o.UnarchiveSandbox(ctx, sb)

	case string(StatusCreating), string(StatusStarting):
		if err := o.waitForEmployeeRuntimeLive(ctx, sb); err != nil {
			return nil, fmt.Errorf("waiting for in-flight sandbox: %w", err)
		}
		now := time.Now()
		o.db.Model(sb).Updates(map[string]any{
			"status":         "running",
			"last_active_at": now,
		})
		sb.Status = "running"
		sb.LastActiveAt = &now
		return sb, nil

	case string(StatusError):
		return nil, fmt.Errorf("sandbox %s is in error state", sb.ID)

	default:
		status, err := o.provider.GetStatus(ctx, sb.ExternalID)
		if err != nil {
			return nil, fmt.Errorf("getting provider status for sandbox %s: %w", sb.ID, err)
		}
		sb.Status = string(status)
		o.db.Model(sb).Update("status", sb.Status)
		if sb.Status == string(StatusRunning) {
			return sb, nil
		}
		return o.EnsureSandboxActive(ctx, sb)
	}
}
