package tasks

import (
	"context"
	"database/sql"
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

const (
	employeeMemoryRetainTimeout = 220 * time.Second
	employeeMemoryQuietWindow   = 3 * time.Minute
	employeeMemoryCheckDelay    = 10 * time.Minute
)

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
	if payload.EmployeeID == uuid.Nil || payload.SandboxID == uuid.Nil {
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

	session, err := h.loadSession(ctx, payload)
	if err != nil {
		return err
	}
	if session == nil {
		return nil
	}
	if payload.EmployeeSessionID == uuid.Nil {
		payload.EmployeeSessionID = session.ID
	}
	if strings.TrimSpace(payload.SessionID) == "" {
		payload.SessionID = session.RuntimeConversationID
	}
	latest, err := h.latestSessionActivity(ctx, session.ID)
	if err != nil {
		return err
	}
	if latest.IsZero() {
		latest = session.CreatedAt
	}
	if time.Since(latest) < employeeMemoryQuietWindow {
		h.enqueueRetainCheck(ctx, payload)
		return nil
	}

	events, err := h.loadSessionEvents(ctx, session.ID)
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
		Model(&model.EmployeeSessionEvent{}).
		Where("id IN ?", employeeSessionEventIDs(events)).
		Update("retained_at", now).Error; err != nil {
		return fmt.Errorf("mark employee session events retained: %w", err)
	}

	h.enqueueRefresh(ctx, payload.EmployeeID, payload.SandboxID)
	return nil
}

func (h *EmployeeMemoryRetainHandler) loadPendingEvents(ctx context.Context, payload EmployeeMemoryRetainPayload) ([]model.EmployeeSessionEvent, error) {
	if payload.EmployeeSessionID != uuid.Nil {
		return h.loadSessionEvents(ctx, payload.EmployeeSessionID)
	}
	var events []model.EmployeeSessionEvent
	if err := h.db.WithContext(ctx).
		Where("employee_id = ? AND sandbox_id = ? AND runtime_session_id = ? AND retained_at IS NULL",
			payload.EmployeeID, payload.SandboxID, payload.SessionID).
		Order("event_at ASC, created_at ASC").
		Find(&events).Error; err != nil {
		return nil, fmt.Errorf("load employee session events: %w", err)
	}
	return events, nil
}

func (h *EmployeeMemoryRetainHandler) loadSession(ctx context.Context, payload EmployeeMemoryRetainPayload) (*model.EmployeeConversation, error) {
	var session model.EmployeeConversation
	query := h.db.WithContext(ctx).Where("employee_id = ? AND sandbox_id = ?", payload.EmployeeID, payload.SandboxID)
	if payload.EmployeeSessionID != uuid.Nil {
		query = query.Where("id = ?", payload.EmployeeSessionID)
	} else if strings.TrimSpace(payload.SessionID) != "" {
		query = query.Where("runtime_conversation_id = ?", strings.TrimSpace(payload.SessionID))
	} else {
		return nil, nil
	}
	if err := query.First(&session).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("load employee session for memory retain: %w", err)
	}
	return &session, nil
}

func (h *EmployeeMemoryRetainHandler) latestSessionActivity(ctx context.Context, employeeSessionID uuid.UUID) (time.Time, error) {
	var latest sql.NullTime
	if err := h.db.WithContext(ctx).
		Model(&model.EmployeeSessionEvent{}).
		Where("employee_session_id = ?", employeeSessionID).
		Select("MAX(event_at)").
		Scan(&latest).Error; err != nil {
		return time.Time{}, fmt.Errorf("load latest employee session activity: %w", err)
	}
	if !latest.Valid {
		return time.Time{}, nil
	}
	return latest.Time.UTC(), nil
}

func (h *EmployeeMemoryRetainHandler) loadSessionEvents(ctx context.Context, employeeSessionID uuid.UUID) ([]model.EmployeeSessionEvent, error) {
	var events []model.EmployeeSessionEvent
	if err := h.db.WithContext(ctx).
		Where("employee_session_id = ?", employeeSessionID).
		Order("event_at ASC, created_at ASC").
		Find(&events).Error; err != nil {
		return nil, fmt.Errorf("load employee session events: %w", err)
	}
	return events, nil
}

func (h *EmployeeMemoryRetainHandler) enqueueRetainCheck(ctx context.Context, payload EmployeeMemoryRetainPayload) {
	if h.enqueuer == nil {
		return
	}
	payload.Reason = "session_still_active"
	task, err := NewEmployeeMemoryRetainTask(payload)
	if err != nil {
		logging.Capture(ctx, err)
		return
	}
	taskID := "employee-memory-retain:" + payload.EmployeeSessionID.String()
	if payload.EmployeeSessionID == uuid.Nil {
		taskID = "employee-memory-retain:" + payload.SandboxID.String() + ":" + payload.SessionID
	}
	if _, err := h.enqueuer.EnqueueContext(ctx, task,
		asynq.ProcessIn(employeeMemoryCheckDelay),
		asynq.TaskID(taskID),
	); err != nil && !errors.Is(err, asynq.ErrDuplicateTask) {
		logging.Capture(ctx, fmt.Errorf("employee memory retain: requeue quiet check: %w", err))
	}
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
