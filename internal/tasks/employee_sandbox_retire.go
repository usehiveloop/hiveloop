package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
)

type EmployeeSandboxRetireHandler struct {
	db           *gorm.DB
	orchestrator *sandbox.Orchestrator
}

func NewEmployeeSandboxRetireHandler(db *gorm.DB, orchestrator *sandbox.Orchestrator) *EmployeeSandboxRetireHandler {
	return &EmployeeSandboxRetireHandler{db: db, orchestrator: orchestrator}
}

func (h *EmployeeSandboxRetireHandler) Handle(ctx context.Context, task *asynq.Task) error {
	if h == nil || h.db == nil || h.orchestrator == nil {
		return fmt.Errorf("employee sandbox retire handler not configured")
	}
	var payload EmployeeSandboxRetirePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal employee sandbox retire payload: %w", err)
	}
	if payload.UpgradeID == uuid.Nil || payload.AgentID == uuid.Nil || payload.SandboxID == uuid.Nil {
		return fmt.Errorf("employee sandbox retire payload missing ids")
	}
	return h.retire(ctx, payload)
}

func (h *EmployeeSandboxRetireHandler) retire(ctx context.Context, payload EmployeeSandboxRetirePayload) error {
	var upgrade model.EmployeeSandboxUpgrade
	if err := h.db.WithContext(ctx).First(&upgrade, "id = ? AND agent_id = ?", payload.UpgradeID, payload.AgentID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return fmt.Errorf("load employee sandbox upgrade: %w", err)
	}
	if upgrade.Status != model.EmployeeSandboxUpgradeStatusSucceeded ||
		upgrade.Phase != model.EmployeeSandboxUpgradePhaseCompleted ||
		upgrade.OldSandboxID == nil ||
		*upgrade.OldSandboxID != payload.SandboxID {
		return nil
	}

	var sb model.Sandbox
	if err := h.db.WithContext(ctx).First(&sb, "id = ?", payload.SandboxID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return fmt.Errorf("load old employee sandbox: %w", err)
	}
	if sb.AgentID == nil || *sb.AgentID != payload.AgentID || sb.Status != string(sandbox.StatusStopped) {
		return nil
	}
	if upgrade.NewSandboxID != nil && *upgrade.NewSandboxID == sb.ID {
		return nil
	}
	recordEmployeeSandboxRetire(ctx, &upgrade, &sb)
	logging.FromContext(ctx).InfoContext(ctx, "retiring old employee sandbox",
		"upgrade_id", upgrade.ID,
		"agent_id", payload.AgentID,
		"sandbox_id", sb.ID,
		"external_id", sb.ExternalID,
	)
	if err := h.orchestrator.DeleteSandbox(ctx, &sb); err != nil {
		return fmt.Errorf("delete retired employee sandbox: %w", err)
	}
	return nil
}
