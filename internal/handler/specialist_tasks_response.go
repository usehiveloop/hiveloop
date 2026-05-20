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

type cloudAgentTaskResponse struct {
	ID                     string                 `json:"id"`
	CloudAgentID           string                 `json:"cloud_agent_id"`
	SandboxID              string                 `json:"sandbox_id"`
	ConversationID         string                 `json:"conversation_id"`
	ParentConversationType string                 `json:"parent_conversation_type"`
	ParentConversationID   string                 `json:"parent_conversation_id"`
	Brief                  string                 `json:"brief"`
	Metadata               map[string]any         `json:"metadata,omitempty"`
	CreatedAt              string                 `json:"created_at"`
	RecentEvents           []cloudAgentEventBrief `json:"recent_events,omitempty"`
}

type cloudAgentWithTasks struct {
	ID           string                     `json:"id"`
	Name         string                     `json:"name"`
	SystemPrompt string                     `json:"system_prompt"`
	Model        string                     `json:"model"`
	Tools        model.JSON                 `json:"tools"`
	Skills       model.JSON                 `json:"skills"`
	RecentTasks  []cloudAgentTaskWithEvents `json:"recent_tasks"`
}

type cloudAgentTaskWithEvents struct {
	ID             string                 `json:"id"`
	Brief          string                 `json:"brief"`
	Metadata       model.JSON             `json:"metadata,omitempty"`
	ConversationID string                 `json:"conversation_id"`
	SandboxID      string                 `json:"sandbox_id"`
	CreatedAt      string                 `json:"created_at"`
	RecentEvents   []cloudAgentEventBrief `json:"recent_events"`
}

type cloudAgentEventBrief struct {
	EventType string        `json:"event_type"`
	Data      model.RawJSON `json:"data"`
	CreatedAt string        `json:"created_at"`
}

func taskToResponse(t model.CloudAgentTask, events []model.ConversationEvent) cloudAgentTaskResponse {
	var meta map[string]any
	if t.Metadata != nil {
		meta = map[string]any(t.Metadata)
	}
	resp := cloudAgentTaskResponse{
		ID:                     t.ID.String(),
		CloudAgentID:           t.CloudAgentID.String(),
		SandboxID:              t.SandboxID.String(),
		ConversationID:         t.ConversationID.String(),
		ParentConversationType: t.ParentConversationType,
		ParentConversationID:   t.ParentConversationID,
		Brief:                  t.Brief,
		Metadata:               meta,
		CreatedAt:              t.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
	for _, ev := range events {
		resp.RecentEvents = append(resp.RecentEvents, cloudAgentEventBrief{
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
	if specialistTemplateBySlug(slug) == nil {
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

func (h *SpecialistTaskHandler) loadTask(ctx context.Context, w http.ResponseWriter, employee *model.Agent, agentID, taskID uuid.UUID) (*model.CloudAgentTask, bool) {
	var task model.CloudAgentTask
	if err := h.db.Where("id = ? AND employee_agent_id = ? AND cloud_agent_id = ?", taskID, employee.ID, agentID).First(&task).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "task not found"})
			return nil, false
		}
		captureCloudAgentFailure(ctx, "load_task", err, cloudAgentSentryContext{
			Operation:    "load_task",
			OrgID:        uuidValue(employee.OrgID),
			EmployeeID:   employee.ID,
			CloudAgentID: agentID,
			TaskID:       taskID,
		})
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load task"})
		return nil, false
	}
	return &task, true
}

func (h *SpecialistTaskHandler) loadTaskRuntime(ctx context.Context, w http.ResponseWriter, task model.CloudAgentTask) (*model.AgentConversation, *model.Sandbox, bool) {
	var conv model.AgentConversation
	if err := h.db.Where("id = ?", task.ConversationID).First(&conv).Error; err != nil {
		captureCloudAgentFailure(ctx, "load_task_runtime", err, cloudAgentSentryContext{
			Operation:      "load_conversation",
			OrgID:          task.OrgID,
			EmployeeID:     task.EmployeeAgentID,
			CloudAgentID:   task.CloudAgentID,
			TaskID:         task.ID,
			SandboxID:      task.SandboxID,
			ConversationID: task.ConversationID,
		})
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load task conversation"})
		return nil, nil, false
	}
	var sb model.Sandbox
	if err := h.db.Where("id = ?", task.SandboxID).First(&sb).Error; err != nil {
		captureCloudAgentFailure(ctx, "load_task_runtime", err, cloudAgentSentryContext{
			Operation:      "load_sandbox",
			OrgID:          task.OrgID,
			EmployeeID:     task.EmployeeAgentID,
			CloudAgentID:   task.CloudAgentID,
			TaskID:         task.ID,
			SandboxID:      task.SandboxID,
			ConversationID: task.ConversationID,
		})
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load task sandbox"})
		return nil, nil, false
	}
	return &conv, &sb, true
}

func conversationIDsForTasks(tasks []model.CloudAgentTask) []uuid.UUID {
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
		captureCloudAgentFailure(context.Background(), "load_recent_events", err, cloudAgentSentryContext{
			Operation: "load_conversation_events",
		})
		return eventsByConv
	}

	for _, row := range rows {
		eventsByConv[row.ConversationID] = append(eventsByConv[row.ConversationID], row.ConversationEvent)
	}
	return eventsByConv
}
