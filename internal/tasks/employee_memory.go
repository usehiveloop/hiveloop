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

	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/hindsight"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

const employeeMemoryRetainTimeout = 220 * time.Second

type EmployeeMemoryRetainHandler struct {
	db       *gorm.DB
	memory   *hindsight.Client
	enqueuer enqueue.TaskEnqueuer
}

func NewEmployeeMemoryRetainHandler(db *gorm.DB, memory *hindsight.Client, enqueuer enqueue.TaskEnqueuer) *EmployeeMemoryRetainHandler {
	return &EmployeeMemoryRetainHandler{db: db, memory: memory, enqueuer: enqueuer}
}

func (h *EmployeeMemoryRetainHandler) Handle(ctx context.Context, task *asynq.Task) error {
	if h == nil || h.db == nil || h.memory == nil {
		return nil
	}
	var payload EmployeeMemoryRetainPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal employee memory retain payload: %w", err)
	}
	if payload.EmployeeID == uuid.Nil || payload.SandboxID == uuid.Nil || strings.TrimSpace(payload.SessionID) == "" {
		return nil
	}

	var agent model.Employee
	if err := h.db.WithContext(ctx).Where("id = ?", payload.EmployeeID).First(&agent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return fmt.Errorf("load employee for memory retain: %w", err)
	}
	if agent.OrgID == nil {
		return nil
	}

	events, err := h.loadPendingEvents(ctx, payload)
	if err != nil {
		return err
	}
	item, ok := buildEmployeeRetainItem(&agent, payload, events)
	if !ok {
		return nil
	}

	bankID := hindsight.OrgBankID(*agent.OrgID)
	if err := h.memory.ConfigureBank(ctx, bankID, hindsight.DefaultMemoryConfig().ToBankConfigUpdate()); err != nil {
		logging.Capture(ctx, fmt.Errorf("employee memory retain: configure bank %s: %w", bankID, err))
		return fmt.Errorf("configure memory bank: %w", err)
	}
	retainCtx, cancel := context.WithTimeout(ctx, employeeMemoryRetainTimeout)
	defer cancel()
	if _, err := h.memory.Retain(retainCtx, bankID, &hindsight.RetainRequest{Items: []hindsight.RetainItem{item}, Async: true}); err != nil {
		logging.Capture(ctx, fmt.Errorf("employee memory retain: retain bank_id=%s employee_id=%s: %w", bankID, agent.ID, err))
		return fmt.Errorf("retain employee memory: %w", err)
	}

	now := time.Now().UTC()
	if err := h.db.WithContext(ctx).
		Model(&model.EmployeeMemoryEvent{}).
		Where("id IN ?", employeeMemoryEventIDs(events)).
		Update("retained_at", now).Error; err != nil {
		return fmt.Errorf("mark employee memory events retained: %w", err)
	}

	h.enqueueRefresh(ctx, payload.EmployeeID, payload.SandboxID)
	return nil
}

func (h *EmployeeMemoryRetainHandler) loadPendingEvents(ctx context.Context, payload EmployeeMemoryRetainPayload) ([]model.EmployeeMemoryEvent, error) {
	var events []model.EmployeeMemoryEvent
	if err := h.db.WithContext(ctx).
		Where("employee_id = ? AND sandbox_id = ? AND session_id = ? AND retained_at IS NULL",
			payload.EmployeeID, payload.SandboxID, payload.SessionID).
		Order("event_at ASC, created_at ASC").
		Find(&events).Error; err != nil {
		return nil, fmt.Errorf("load employee memory events: %w", err)
	}
	return events, nil
}

func (h *EmployeeMemoryRetainHandler) enqueueRefresh(ctx context.Context, agentID, sandboxID uuid.UUID) {
	if h.enqueuer == nil {
		return
	}
	h.updateAgentMemoryRefreshStatus(ctx, agentID, "queued", "")
	task, err := NewEmployeeMemoryRefreshTask(EmployeeMemoryRefreshPayload{
		EmployeeID: agentID,
		SandboxID:  sandboxID,
		Reason:     "hindsight_retain",
	})
	if err != nil {
		logging.Capture(ctx, err)
		return
	}
	if _, err := h.enqueuer.EnqueueContext(ctx, task,
		asynq.Unique(2*time.Minute),
		asynq.TaskID("employee-memory-refresh:"+agentID.String()),
	); err != nil && !errors.Is(err, asynq.ErrDuplicateTask) {
		logging.Capture(ctx, fmt.Errorf("employee memory retain: enqueue refresh: %w", err))
	}
}

func (h *EmployeeMemoryRetainHandler) updateAgentMemoryRefreshStatus(ctx context.Context, agentID uuid.UUID, status, message string) {
	if h == nil || h.db == nil || agentID == uuid.Nil {
		return
	}
	updates := map[string]any{
		"memory_refresh_status": status,
		"memory_refresh_error":  truncateMemoryRefreshError(message),
	}
	if err := h.db.WithContext(ctx).Model(&model.Employee{}).Where("id = ?", agentID).Updates(updates).Error; err != nil {
		logging.Capture(ctx, fmt.Errorf("employee memory retain: update refresh status: %w", err))
	}
}
