package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/employeeruntime"
	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/sandbox"
	"github.com/usehiveloop/hiveloop/internal/storage"
)

const employeeSandboxUpgradeCommandTimeout = 60 * time.Minute

type employeeSandboxUpgradeBackupStore interface {
	Head(ctx context.Context, key string) (*storage.S3ObjectInfo, error)
	PresignedURL(ctx context.Context, key string, ttl time.Duration) (string, error)
}

type EmployeeSandboxUpgradeHandler struct {
	db           *gorm.DB
	orchestrator *sandbox.Orchestrator
	store        employeeSandboxUpgradeBackupStore
	compileDeps  employeeruntime.CompileDeps
}

func NewEmployeeSandboxUpgradeHandler(
	db *gorm.DB,
	orchestrator *sandbox.Orchestrator,
	store employeeSandboxUpgradeBackupStore,
	compileDeps employeeruntime.CompileDeps,
) *EmployeeSandboxUpgradeHandler {
	return &EmployeeSandboxUpgradeHandler{
		db:           db,
		orchestrator: orchestrator,
		store:        store,
		compileDeps:  compileDeps,
	}
}

func (h *EmployeeSandboxUpgradeHandler) Handle(ctx context.Context, task *asynq.Task) error {
	if h == nil || h.db == nil || h.orchestrator == nil || h.store == nil || h.compileDeps.EncKey == nil {
		return fmt.Errorf("employee sandbox upgrade handler not configured")
	}
	var payload EmployeeSandboxUpgradePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal employee sandbox upgrade payload: %w", err)
	}
	if payload.UpgradeID == uuid.Nil || payload.AgentID == uuid.Nil {
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

	var oldStopped bool
	var newSandbox *model.Sandbox
	fail := func(phase string, cause error) error {
		msg := cause.Error()
		log.ErrorContext(ctx, "employee sandbox upgrade failed",
			"upgrade_id", upgrade.ID,
			"agent_id", upgrade.AgentID,
			"phase", phase,
			"error", msg,
		)
		if newSandbox != nil && newSandbox.ID != uuid.Nil {
			if err := h.orchestrator.DeleteSandbox(ctx, newSandbox); err != nil {
				msg += "; failed to delete new sandbox during rollback: " + err.Error()
			}
		}
		if oldSandbox != nil && oldSandbox.ID != uuid.Nil {
			if oldStopped {
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

	if err := h.markPhase(ctx, upgrade, model.EmployeeSandboxUpgradePhaseBackup); err != nil {
		return err
	}
	if err := h.db.WithContext(ctx).Model(oldSandbox).Updates(map[string]any{
		"status":        "upgrading",
		"error_message": nil,
	}).Error; err != nil {
		return fail(model.EmployeeSandboxUpgradePhaseBackup, fmt.Errorf("mark old sandbox upgrading: %w", err))
	}
	oldSandbox.Status = "upgrading"
	oldSandbox.ErrorMessage = nil

	backupMeta, err := h.runBackup(ctx, upgrade, agent, oldSandbox)
	if err != nil {
		return fail(model.EmployeeSandboxUpgradePhaseBackup, err)
	}
	if err := h.verifyAndRecordBackup(ctx, upgrade, backupMeta); err != nil {
		return fail(model.EmployeeSandboxUpgradePhaseBackup, err)
	}

	if err := h.markPhase(ctx, upgrade, model.EmployeeSandboxUpgradePhaseStoppingOld); err != nil {
		return fail(model.EmployeeSandboxUpgradePhaseStoppingOld, err)
	}
	if err := h.orchestrator.StopSandbox(ctx, oldSandbox); err != nil {
		return fail(model.EmployeeSandboxUpgradePhaseStoppingOld, err)
	}
	oldStopped = true

	if err := h.markPhase(ctx, upgrade, model.EmployeeSandboxUpgradePhaseCreatingNew); err != nil {
		return fail(model.EmployeeSandboxUpgradePhaseCreatingNew, err)
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

	if err := h.markPhase(ctx, upgrade, model.EmployeeSandboxUpgradePhaseRestore); err != nil {
		return fail(model.EmployeeSandboxUpgradePhaseRestore, err)
	}
	if err := h.runRestore(ctx, backupMeta, newSandbox); err != nil {
		return fail(model.EmployeeSandboxUpgradePhaseRestore, err)
	}

	if err := h.markPhase(ctx, upgrade, model.EmployeeSandboxUpgradePhaseRestartNew); err != nil {
		return fail(model.EmployeeSandboxUpgradePhaseRestartNew, err)
	}
	if err := h.orchestrator.RestartEmployeeSandbox(ctx, newSandbox); err != nil {
		return fail(model.EmployeeSandboxUpgradePhaseRestartNew, err)
	}

	if err := h.markPhase(ctx, upgrade, model.EmployeeSandboxUpgradePhaseSync); err != nil {
		return fail(model.EmployeeSandboxUpgradePhaseSync, err)
	}
	if err := h.syncEmployeeRuntime(ctx, agent, newSandbox); err != nil {
		return fail(model.EmployeeSandboxUpgradePhaseSync, err)
	}
	if payload.SmokeTest {
		if err := h.runSmokeTest(ctx, newSandbox); err != nil {
			return fail(model.EmployeeSandboxUpgradePhaseSync, err)
		}
	}

	if err := h.markPhase(ctx, upgrade, model.EmployeeSandboxUpgradePhaseCleanupOld); err != nil {
		return fail(model.EmployeeSandboxUpgradePhaseCleanupOld, err)
	}
	if err := h.orchestrator.DeleteSandbox(ctx, oldSandbox); err != nil {
		logging.Capture(ctx, fmt.Errorf("employee sandbox upgrade %s: old sandbox delete failed: %w", upgrade.ID, err))
	}

	now := time.Now().UTC()
	if err := h.db.WithContext(ctx).Model(upgrade).Updates(map[string]any{
		"status":       model.EmployeeSandboxUpgradeStatusSucceeded,
		"phase":        model.EmployeeSandboxUpgradePhaseCompleted,
		"completed_at": now,
	}).Error; err != nil {
		return fmt.Errorf("mark employee sandbox upgrade succeeded: %w", err)
	}
	log.InfoContext(ctx, "employee sandbox upgrade succeeded",
		"upgrade_id", upgrade.ID,
		"agent_id", upgrade.AgentID,
		"old_sandbox_id", upgrade.OldSandboxID,
		"new_sandbox_id", upgrade.NewSandboxID,
	)
	return nil
}
