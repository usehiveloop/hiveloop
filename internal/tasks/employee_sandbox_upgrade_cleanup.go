package tasks

import (
	"context"
	"errors"
	"fmt"

	"github.com/hibiken/asynq"

	"github.com/usehivy/hivy/internal/model"
)

func (h *EmployeeSandboxUpgradeHandler) scheduleOldSandboxRetirement(
	ctx context.Context,
	upgrade *model.EmployeeSandboxUpgrade,
	oldSandbox *model.Sandbox,
) error {
	if h.enqueuer == nil {
		return fmt.Errorf("task enqueuer not configured")
	}
	task, opts, err := NewEmployeeSandboxRetireTask(EmployeeSandboxRetirePayload{
		UpgradeID: upgrade.ID,
		AgentID:   upgrade.AgentID,
		SandboxID: oldSandbox.ID,
	})
	if err != nil {
		return err
	}
	if _, err := h.enqueuer.EnqueueContext(ctx, task, opts...); err != nil {
		if errors.Is(err, asynq.ErrDuplicateTask) || errors.Is(err, asynq.ErrTaskIDConflict) {
			return nil
		}
		return fmt.Errorf("enqueue old sandbox retirement: %w", err)
	}
	return nil
}
