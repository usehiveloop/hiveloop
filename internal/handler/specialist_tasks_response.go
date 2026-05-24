package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

type specialistTaskResponse struct {
	ID                     string                 `json:"id"`
	SpecialistID           string                 `json:"specialist_id"`
	SandboxID              string                 `json:"sandbox_id"`
	ConversationID         string                 `json:"conversation_id"`
	ParentConversationType string                 `json:"parent_conversation_type"`
	ParentConversationID   string                 `json:"parent_conversation_id"`
	Brief                  string                 `json:"brief"`
	Metadata               map[string]any         `json:"metadata,omitempty"`
	CreatedAt              string                 `json:"created_at"`
	RecentEvents           []specialistEventBrief `json:"recent_events,omitempty"`
}

type specialistWithTasks struct {
	ID           string                     `json:"id"`
	Name         string                     `json:"name"`
	SystemPrompt string                     `json:"system_prompt"`
	Model        string                     `json:"model"`
	Tools        model.JSON                 `json:"tools"`
	Skills       model.JSON                 `json:"skills"`
	RecentTasks  []specialistTaskWithEvents `json:"recent_tasks"`
}

type specialistTaskWithEvents struct {
	ID             string                 `json:"id"`
	Brief          string                 `json:"brief"`
	Metadata       model.JSON             `json:"metadata,omitempty"`
	ConversationID string                 `json:"conversation_id"`
	SandboxID      string                 `json:"sandbox_id"`
	CreatedAt      string                 `json:"created_at"`
	RecentEvents   []specialistEventBrief `json:"recent_events"`
}

type specialistEventBrief struct {
	EventType string        `json:"event_type"`
	Data      model.RawJSON `json:"data"`
	CreatedAt string        `json:"created_at"`
}

func taskToResponse(t model.SpecialistTask, events []model.ConversationEvent) specialistTaskResponse {
	var meta map[string]any
	if t.Metadata != nil {
		meta = map[string]any(t.Metadata)
	}
	resp := specialistTaskResponse{
		ID:                     t.ID.String(),
		SpecialistID:           t.SpecialistID.String(),
		SandboxID:              t.SandboxID.String(),
		ConversationID:         t.ConversationID.String(),
		ParentConversationType: t.ParentConversationType,
		ParentConversationID:   t.ParentConversationID,
		Brief:                  t.Brief,
		Metadata:               meta,
		CreatedAt:              t.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
	for _, ev := range events {
		resp.RecentEvents = append(resp.RecentEvents, specialistEventBrief{
			EventType: ev.EventType,
			Data:      ev.Data,
			CreatedAt: ev.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		})
	}
	return resp
}

func specialistSlugParam(r *http.Request) string {
	return chi.URLParam(r, "specialistSlug")
}

func (h *SpecialistTaskHandler) parseAgentAndTaskIDs(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	slug := specialistSlugParam(r)
	if _, ok := h.catalog.BySlug(slug); !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid specialist_slug"})
		return uuid.Nil, uuid.Nil, false
	}
	agentID := specialistID(slug)
	taskID, err := uuid.Parse(chi.URLParam(r, "taskID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid task_id"})
		return uuid.Nil, uuid.Nil, false
	}
	return agentID, taskID, true
}

func (h *SpecialistTaskHandler) loadTask(ctx context.Context, w http.ResponseWriter, employee *model.Employee, agentID, taskID uuid.UUID) (*model.SpecialistTask, bool) {
	var task model.SpecialistTask
	if err := h.db.Where("id = ? AND employee_id = ? AND specialist_id = ?", taskID, employee.ID, agentID).First(&task).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "task not found"})
			return nil, false
		}
		captureSpecialistFailure(ctx, "load_task", err, specialistSentryContext{
			Operation:    "load_task",
			OrgID:        uuidValue(employee.OrgID),
			EmployeeID:   employee.ID,
			SpecialistID: agentID,
			TaskID:       taskID,
		})
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load task"})
		return nil, false
	}
	return &task, true
}

func (h *SpecialistTaskHandler) loadTaskRuntime(ctx context.Context, w http.ResponseWriter, task model.SpecialistTask) (*model.EmployeeConversation, *model.Sandbox, bool) {
	var conv model.EmployeeConversation
	if err := h.db.Where("id = ?", task.ConversationID).First(&conv).Error; err != nil {
		captureSpecialistFailure(ctx, "load_task_runtime", err, specialistSentryContext{
			Operation:      "load_conversation",
			OrgID:          task.OrgID,
			EmployeeID:     task.EmployeeID,
			SpecialistID:   task.SpecialistID,
			TaskID:         task.ID,
			SandboxID:      task.SandboxID,
			ConversationID: task.ConversationID,
		})
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load task conversation"})
		return nil, nil, false
	}
	var sb model.Sandbox
	if err := h.db.Where("id = ?", task.SandboxID).First(&sb).Error; err != nil {
		captureSpecialistFailure(ctx, "load_task_runtime", err, specialistSentryContext{
			Operation:      "load_sandbox",
			OrgID:          task.OrgID,
			EmployeeID:     task.EmployeeID,
			SpecialistID:   task.SpecialistID,
			TaskID:         task.ID,
			SandboxID:      task.SandboxID,
			ConversationID: task.ConversationID,
		})
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load task sandbox"})
		return nil, nil, false
	}
	return &conv, &sb, true
}

func conversationIDsForTasks(tasks []model.SpecialistTask) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(tasks))
	for _, task := range tasks {
		ids = append(ids, task.ConversationID)
	}
	return ids
}

func (h *SpecialistTaskHandler) recentEventsForConversation(conversationID uuid.UUID, limit int) []model.ConversationEvent {
	eventsByConv := h.recentEventsByConversation([]uuid.UUID{conversationID}, limit)
	return eventsByConv[conversationID]
}

func (h *SpecialistTaskHandler) recentEventsByConversation(conversationIDs []uuid.UUID, limit int) map[uuid.UUID][]model.ConversationEvent {
	eventsByConv := make(map[uuid.UUID][]model.ConversationEvent)
	if len(conversationIDs) == 0 {
		return eventsByConv
	}

	type eventRow struct {
		model.ConversationEvent
		Rn int `gorm:"column:rn"`
	}
	var rows []eventRow
	if err := h.db.Raw(`
		SELECT id, conversation_id, event_type, data, created_at, rn
		FROM (
			SELECT id, conversation_id, event_type, data, created_at,
				ROW_NUMBER() OVER (PARTITION BY conversation_id ORDER BY created_at DESC) AS rn
			FROM conversation_events
			WHERE conversation_id IN ?
		) ranked
		WHERE rn <= ?
	`, conversationIDs, limit).Scan(&rows).Error; err != nil {
		captureSpecialistFailure(context.Background(), "load_recent_events", err, specialistSentryContext{
			Operation: "load_conversation_events",
		})
		return eventsByConv
	}

	for _, row := range rows {
		eventsByConv[row.ConversationID] = append(eventsByConv[row.ConversationID], row.ConversationEvent)
	}
	return eventsByConv
}
