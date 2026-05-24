package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/employeeruntime"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

type EmployeeMemoryRefreshHandler struct {
	db          *gorm.DB
	compileDeps employeeruntime.CompileDeps
}

func NewEmployeeMemoryRefreshHandler(db *gorm.DB, compileDeps employeeruntime.CompileDeps) *EmployeeMemoryRefreshHandler {
	return &EmployeeMemoryRefreshHandler{db: db, compileDeps: compileDeps}
}

func (h *EmployeeMemoryRefreshHandler) Handle(ctx context.Context, task *asynq.Task) error {
	if h == nil || h.db == nil || h.compileDeps.EncKey == nil {
		return nil
	}
	var payload EmployeeMemoryRefreshPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal employee memory refresh payload: %w", err)
	}
	if payload.EmployeeID == uuid.Nil {
		return nil
	}
	h.updateRefreshStatus(ctx, payload.EmployeeID, "running", "", nil)
	if err := h.refresh(ctx, payload); err != nil {
		h.updateRefreshStatus(ctx, payload.EmployeeID, "failed", err.Error(), nil)
		return err
	}
	now := time.Now().UTC()
	h.updateRefreshStatus(ctx, payload.EmployeeID, "succeeded", "", &now)
	return nil
}

func (h *EmployeeMemoryRefreshHandler) refresh(ctx context.Context, payload EmployeeMemoryRefreshPayload) error {
	var agent model.Employee
	if err := h.db.WithContext(ctx).Where("id = ? AND status <> ?", payload.EmployeeID, "archived").First(&agent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return fmt.Errorf("load employee for memory refresh: %w", err)
	}
	sb, err := h.loadSandbox(ctx, payload)
	if err != nil {
		return err
	}
	if sb == nil {
		return nil
	}
	apiKey, err := h.compileDeps.EncKey.DecryptString(sb.EncryptedBridgeAPIKey)
	if err != nil {
		return fmt.Errorf("decrypt employee runtime secret: %w", err)
	}
	def, err := employeeruntime.Compile(ctx, h.compileDeps, &agent)
	if err != nil {
		return fmt.Errorf("compile employee config for memory refresh: %w", err)
	}
	def.OutboundChannels = employeeruntime.ControlPlaneOutboundChannels(h.compileDeps.Cfg, sb.ID)
	client := employeeruntime.NewClient(sb.BridgeURL, apiKey)
	if err := client.Healthz(ctx); err != nil {
		return fmt.Errorf("employee runtime healthz: %w", err)
	}
	if _, err := client.PutConfig(ctx, def); err != nil {
		return fmt.Errorf("employee runtime put config: %w", err)
	}
	if err := client.Readyz(ctx); err != nil {
		return fmt.Errorf("employee runtime readyz: %w", err)
	}
	logging.FromContext(ctx).InfoContext(ctx, "employee memory refreshed",
		"employee_id", agent.ID,
		"sandbox_id", sb.ID,
		"reason", payload.Reason,
	)
	return nil
}

func (h *EmployeeMemoryRefreshHandler) updateRefreshStatus(ctx context.Context, agentID uuid.UUID, status, message string, refreshedAt *time.Time) {
	if h == nil || h.db == nil || agentID == uuid.Nil {
		return
	}
	updates := map[string]any{
		"memory_refresh_status": status,
		"memory_refresh_error":  truncateMemoryRefreshError(message),
	}
	if refreshedAt != nil {
		updates["last_memory_refreshed_at"] = *refreshedAt
	}
	if err := h.db.WithContext(ctx).Model(&model.Employee{}).Where("id = ?", agentID).Updates(updates).Error; err != nil {
		logging.Capture(ctx, fmt.Errorf("employee memory refresh: update status: %w", err))
	}
}

func truncateMemoryRefreshError(message string) string {
	message = strings.TrimSpace(message)
	if len(message) <= 2000 {
		return message
	}
	return message[:2000]
}

func (h *EmployeeMemoryRefreshHandler) loadSandbox(ctx context.Context, payload EmployeeMemoryRefreshPayload) (*model.Sandbox, error) {
	var sb model.Sandbox
	q := h.db.WithContext(ctx).Where("employee_id = ? AND status <> ?", payload.EmployeeID, "error")
	if payload.SandboxID != uuid.Nil {
		q = q.Where("id = ?", payload.SandboxID)
	}
	err := q.Order("created_at DESC").First(&sb).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("load employee sandbox for memory refresh: %w", err)
	}
	return &sb, nil
}
