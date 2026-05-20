package tasks

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
)

// EmployeeCleanupHandler cleans up provider sandbox resources left behind after an employee hard delete.
type EmployeeCleanupHandler struct {
	db           *gorm.DB
	orchestrator *sandbox.Orchestrator
	pusher       *sandbox.Pusher
}

// NewEmployeeCleanupHandler creates a new employee cleanup handler.
func NewEmployeeCleanupHandler(db *gorm.DB, orchestrator *sandbox.Orchestrator, pusher *sandbox.Pusher) *EmployeeCleanupHandler {
	return &EmployeeCleanupHandler{db: db, orchestrator: orchestrator, pusher: pusher}
}

// Handle processes an employee:cleanup task.
func (h *EmployeeCleanupHandler) Handle(ctx context.Context, t *asynq.Task) error {
	var payload EmployeeCleanupPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal employee cleanup payload: %w", err)
	}

	if len(payload.SandboxExternalIDs) > 0 {
		h.cleanupExternalSandboxes(ctx, payload.SandboxExternalIDs)
		return nil
	}

	var employee model.Agent
	if err := h.db.Where("id = ?", payload.EmployeeID).First(&employee).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil
		}
		return fmt.Errorf("loading employee: %w", err)
	}

	h.cleanupEmployeeSandboxes(ctx, &employee)

	if err := h.db.Where("id = ?", employee.ID).Delete(&model.Agent{}).Error; err != nil {
		return fmt.Errorf("hard-deleting employee: %w", err)
	}

	logging.FromContext(ctx).InfoContext(ctx, "employee cleanup complete", "employee_id", employee.ID)
	return nil
}

func (h *EmployeeCleanupHandler) cleanupExternalSandboxes(ctx context.Context, externalIDs []string) {
	if h.orchestrator == nil {
		return
	}
	for _, externalID := range externalIDs {
		if externalID == "" {
			continue
		}
		if err := h.orchestrator.DeleteSandboxExternal(ctx, externalID); err != nil {
			logging.Capture(ctx, fmt.Errorf("delete provider sandbox %s: %w", externalID, err))
		}
	}
}

func (h *EmployeeCleanupHandler) cleanupEmployeeSandboxes(ctx context.Context, employee *model.Agent) {
	if h.orchestrator == nil {
		return
	}

	var sandboxes []model.Sandbox
	if err := h.db.Where("agent_id = ?", employee.ID).Find(&sandboxes).Error; err != nil {
		logging.Capture(ctx, fmt.Errorf("find sandboxes for employee %s: %w", employee.ID, err))
		return
	}

	for _, sb := range sandboxes {
		if err := h.orchestrator.DeleteSandbox(ctx, &sb); err != nil {
			logging.Capture(ctx, fmt.Errorf("delete sandbox %s for employee %s: %w", sb.ID, employee.ID, err))
		}
	}
}
