package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/employeeruntime"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
)

type EmployeeSandboxUpgradeHandler struct {
	db           *gorm.DB
	orchestrator *sandbox.Orchestrator
	compileDeps  employeeruntime.CompileDeps
	enqueuer     enqueue.TaskEnqueuer
}

func NewEmployeeSandboxUpgradeHandler(
	db *gorm.DB,
	orchestrator *sandbox.Orchestrator,
	compileDeps employeeruntime.CompileDeps,
	enqueuer enqueue.TaskEnqueuer,
) *EmployeeSandboxUpgradeHandler {
	return &EmployeeSandboxUpgradeHandler{
		db:           db,
		orchestrator: orchestrator,
		compileDeps:  compileDeps,
		enqueuer:     enqueuer,
	}
}

func (h *EmployeeSandboxUpgradeHandler) Handle(ctx context.Context, task *asynq.Task) error {
	if h == nil || h.db == nil || h.orchestrator == nil || h.compileDeps.EncKey == nil || h.enqueuer == nil {
		return fmt.Errorf("employee sandbox upgrade handler not configured")
	}
	var payload EmployeeSandboxUpgradePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal employee sandbox upgrade payload: %w", err)
	}
	if payload.UpgradeID == uuid.Nil || payload.EmployeeID == uuid.Nil {
		return fmt.Errorf("employee sandbox upgrade payload missing ids")
	}
	return h.run(ctx, payload)
}

func (h *EmployeeSandboxUpgradeHandler) run(ctx context.Context, payload EmployeeSandboxUpgradePayload) error {
	log := logging.FromContext(ctx)

	upgrade, agent, oldSandbox, err := h.loadAndStart(ctx, payload)
	if err != nil {
		return err
	}
	if upgrade == nil {
		return nil
	}
	annotateEmployeeSandboxUpgradeSentry(ctx, upgrade, agent, oldSandbox)

	var oldPaused bool
	var newSandbox *model.Sandbox
	fail := func(phase string, cause error) error {
		msg := cause.Error()
		recordEmployeeSandboxUpgradeFailure(ctx, upgrade, phase, msg)
		log.ErrorContext(ctx, "employee sandbox upgrade failed",
			"upgrade_id", upgrade.ID,
			"employee_id", upgrade.EmployeeID,
			"phase", phase,
			"error", msg,
		)
		if newSandbox != nil && newSandbox.ID != uuid.Nil {
			if err := h.orchestrator.DeleteSandbox(ctx, newSandbox); err != nil {
				msg += "; failed to delete new sandbox during rollback: " + err.Error()
			}
		}
		if oldSandbox != nil && oldSandbox.ID != uuid.Nil {
			if oldPaused {
				if err := h.orchestrator.StartEmployeeSandbox(ctx, oldSandbox); err != nil {
					msg += "; failed to restart old sandbox during rollback: " + err.Error()
				} else if err := h.syncEmployeeRuntime(ctx, agent, oldSandbox); err != nil {
					msg += "; failed to sync old sandbox during rollback: " + err.Error()
				}
			} else {
				_ = h.db.WithContext(ctx).Model(oldSandbox).Updates(map[string]any{
					"status":        string(sandbox.StatusRunning),
					"error_message": nil,
				}).Error
				oldSandbox.Status = string(sandbox.StatusRunning)
				oldSandbox.ErrorMessage = nil
			}
		}
		h.markFailed(ctx, upgrade, phase, msg)
		return cause
	}

	// Phase 1: Create new sandbox
	if err := h.markPhase(ctx, upgrade, model.EmployeeSandboxUpgradePhaseCreatingNew); err != nil {
		return err
	}
	secrets, err := employeeruntime.PrepareStartup(ctx, h.compileDeps, agent)
	if err != nil {
		return fail(model.EmployeeSandboxUpgradePhaseCreatingNew, err)
	}
	newSandbox, err = h.orchestrator.CreateEmployeeSandbox(ctx, agent, secrets)
	if err != nil {
		return fail(model.EmployeeSandboxUpgradePhaseCreatingNew, err)
	}
	if err := h.db.WithContext(ctx).Model(upgrade).Update("new_sandbox_id", newSandbox.ID).Error; err != nil {
		return fail(model.EmployeeSandboxUpgradePhaseCreatingNew, err)
	}
	upgrade.NewSandboxID = &newSandbox.ID
	recordEmployeeSandboxUpgradeNewSandbox(ctx, upgrade, newSandbox)

	// Phase 2: Sync config to new sandbox
	if err := h.markPhase(ctx, upgrade, model.EmployeeSandboxUpgradePhaseSync); err != nil {
		return fail(model.EmployeeSandboxUpgradePhaseSync, err)
	}
	if err := h.syncEmployeeRuntime(ctx, agent, newSandbox); err != nil {
		return fail(model.EmployeeSandboxUpgradePhaseSync, err)
	}

	// Phase 3: Pause old sandbox
	if err := h.markPhase(ctx, upgrade, model.EmployeeSandboxUpgradePhasePausingOld); err != nil {
		return fail(model.EmployeeSandboxUpgradePhasePausingOld, err)
	}
	if err := h.orchestrator.StopSandbox(ctx, oldSandbox); err != nil {
		return fail(model.EmployeeSandboxUpgradePhasePausingOld, err)
	}
	oldPaused = true

	// Phase 4: Schedule old sandbox retirement
	if err := h.markPhase(ctx, upgrade, model.EmployeeSandboxUpgradePhaseCleanupOld); err != nil {
		return fail(model.EmployeeSandboxUpgradePhaseCleanupOld, err)
	}
	if err := h.scheduleOldSandboxRetirement(ctx, upgrade, oldSandbox); err != nil {
		logging.Capture(ctx, fmt.Errorf("employee sandbox upgrade %s: schedule old sandbox retirement failed: %w", upgrade.ID, err))
	}

	// Phase 5: Mark succeeded
	now := time.Now().UTC()
	if err := h.db.WithContext(ctx).Model(upgrade).Updates(map[string]any{
		"status":       model.EmployeeSandboxUpgradeStatusSucceeded,
		"phase":        model.EmployeeSandboxUpgradePhaseCompleted,
		"completed_at": now,
	}).Error; err != nil {
		return fail(model.EmployeeSandboxUpgradePhaseCleanupOld, fmt.Errorf("mark employee sandbox upgrade succeeded: %w", err))
	}
	upgrade.Status = model.EmployeeSandboxUpgradeStatusSucceeded
	upgrade.Phase = model.EmployeeSandboxUpgradePhaseCompleted
	upgrade.CompletedAt = &now
	recordEmployeeSandboxUpgradeSuccess(ctx, upgrade)
	log.InfoContext(ctx, "employee sandbox upgrade succeeded",
		"upgrade_id", upgrade.ID,
		"employee_id", upgrade.EmployeeID,
		"old_sandbox_id", upgrade.OldSandboxID,
		"new_sandbox_id", upgrade.NewSandboxID,
	)
	return nil
}
