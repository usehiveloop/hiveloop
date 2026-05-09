package handler

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/crypto"
	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/sandbox"
)

type CloudAgentHandler struct {
	db           *gorm.DB
	encKey       *crypto.SymmetricKey
	orchestrator *sandbox.Orchestrator
	pusher       *sandbox.Pusher
}

func NewCloudAgentHandler(db *gorm.DB, encKey *crypto.SymmetricKey, orchestrator *sandbox.Orchestrator, pusher *sandbox.Pusher) *CloudAgentHandler {
	return &CloudAgentHandler{db: db, encKey: encKey, orchestrator: orchestrator, pusher: pusher}
}

// authEmployee verifies the bridge bearer token for the employee in the URL.
// On failure it writes the error response and returns nil — callers must return.
func (h *CloudAgentHandler) authEmployee(w http.ResponseWriter, r *http.Request) *model.Agent {
	if h.encKey == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "cloud agent endpoints not configured"})
		return nil
	}

	agentID, err := uuid.Parse(chi.URLParam(r, "employeeID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid employee_id"})
		return nil
	}

	bearer := bearerFromHeader(r.Header.Get("Authorization"))
	if bearer == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing authorization"})
		return nil
	}

	var agent model.Agent
	if err := h.db.Where("id = ? AND is_employee = true AND deleted_at IS NULL", agentID).First(&agent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "employee not found"})
			return nil
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load employee"})
		return nil
	}

	var sb model.Sandbox
	if err := h.db.
		Where("agent_id = ? AND status NOT IN (?, ?)", agentID, "archived", "error").
		Order("created_at DESC").
		First(&sb).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "sandbox not found for employee"})
			return nil
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load sandbox"})
		return nil
	}

	wantKey, err := h.encKey.DecryptString(sb.EncryptedBridgeAPIKey)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "decrypt bridge api key", "agent_id", agentID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to verify credentials"})
		return nil
	}
	if subtle.ConstantTimeCompare([]byte(bearer), []byte(wantKey)) != 1 {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid bridge api key"})
		return nil
	}

	return &agent
}

// validateSubagent checks that agentID is a subagent of the employee.
func (h *CloudAgentHandler) validateSubagent(w http.ResponseWriter, employeeID, agentID uuid.UUID) bool {
	var link model.AgentSubagent
	if err := h.db.Where("agent_id = ? AND subagent_id = ?", employeeID, agentID).First(&link).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "cloud agent not found for this employee"})
			return false
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to validate cloud agent"})
		return false
	}
	return true
}

// ListCloudAgents returns all cloud agents for the employee with their top 3
// most recent tasks and top 10 most recent events per task.
func (h *CloudAgentHandler) ListCloudAgents(w http.ResponseWriter, r *http.Request) {
	employee := h.authEmployee(w, r)
	if employee == nil {
		return
	}

	var links []model.AgentSubagent
	if err := h.db.Where("agent_id = ?", employee.ID).Find(&links).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load cloud agents"})
		return
	}

	if len(links) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"cloud_agents": []any{}})
		return
	}

	subagentIDs := make([]uuid.UUID, len(links))
	for i, l := range links {
		subagentIDs[i] = l.SubagentID
	}

	var agents []model.Agent
	if err := h.db.Where("id IN ?", subagentIDs).Find(&agents).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load agent details"})
		return
	}

	// Load top 3 tasks per cloud agent
	var allTasks []model.CloudAgentTask
	if err := h.db.
		Where("employee_agent_id = ? AND cloud_agent_id IN ?", employee.ID, subagentIDs).
		Order("created_at DESC").
		Find(&allTasks).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load tasks"})
		return
	}

	// Group tasks by cloud agent, keep top 3
	tasksByAgent := make(map[uuid.UUID][]model.CloudAgentTask)
	for _, t := range allTasks {
		list := tasksByAgent[t.CloudAgentID]
		if len(list) < 3 {
			tasksByAgent[t.CloudAgentID] = append(list, t)
		}
	}

	// Collect all conversation IDs for loaded tasks
	var convIDs []uuid.UUID
	for _, tasks := range tasksByAgent {
		for _, t := range tasks {
			convIDs = append(convIDs, t.ConversationID)
		}
	}

	// Load top 10 events per conversation using a window function
	eventsByConv := make(map[uuid.UUID][]model.ConversationEvent)
	if len(convIDs) > 0 {
		type eventRow struct {
			model.ConversationEvent
			Rn int `gorm:"column:rn"`
		}
		var rows []eventRow
		h.db.Raw(`
			SELECT id, conversation_id, event_type, data, created_at, rn
			FROM (
				SELECT id, conversation_id, event_type, data, created_at,
					ROW_NUMBER() OVER (PARTITION BY conversation_id ORDER BY created_at DESC) AS rn
				FROM conversation_events
				WHERE conversation_id IN ?
			) ranked
			WHERE rn <= 10
		`, convIDs).Scan(&rows)

		for _, row := range rows {
			eventsByConv[row.ConversationID] = append(eventsByConv[row.ConversationID], row.ConversationEvent)
		}
	}

	// Assemble response
	result := make([]cloudAgentWithTasks, 0, len(agents))
	for _, agent := range agents {
		ca := cloudAgentWithTasks{
			ID:           agent.ID.String(),
			Name:         agent.Name,
			SystemPrompt: agent.SystemPrompt,
			Model:        agent.Model,
			Tools:        agent.Tools,
			Skills:       agent.Skills,
			RecentTasks:  []cloudAgentTaskWithEvents{},
		}

		for _, task := range tasksByAgent[agent.ID] {
			tw := cloudAgentTaskWithEvents{
				ID:             task.ID.String(),
				Brief:          task.Brief,
				Metadata:       task.Metadata,
				ConversationID: task.ConversationID.String(),
				SandboxID:      task.SandboxID.String(),
				CreatedAt:      task.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
				RecentEvents:   []cloudAgentEventBrief{},
			}
			for _, ev := range eventsByConv[task.ConversationID] {
				tw.RecentEvents = append(tw.RecentEvents, cloudAgentEventBrief{
					EventType: ev.EventType,
					Data:      ev.Data,
					CreatedAt: ev.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
				})
			}
			ca.RecentTasks = append(ca.RecentTasks, tw)
		}

		result = append(result, ca)
	}

	writeJSON(w, http.StatusOK, map[string]any{"cloud_agents": result})
}

// ListTasks returns paginated tasks for a specific cloud agent.
func (h *CloudAgentHandler) ListTasks(w http.ResponseWriter, r *http.Request) {
	employee := h.authEmployee(w, r)
	if employee == nil {
		return
	}

	agentID, err := uuid.Parse(chi.URLParam(r, "agentID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent_id"})
		return
	}
	if !h.validateSubagent(w, employee.ID, agentID) {
		return
	}

	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	q := h.db.Where("org_id = ? AND employee_agent_id = ? AND cloud_agent_id = ?", employee.OrgID, employee.ID, agentID)
	q = applyPagination(q, cursor, limit)

	var tasks []model.CloudAgentTask
	if err := q.Find(&tasks).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load tasks"})
		return
	}

	hasMore := len(tasks) > limit
	if hasMore {
		tasks = tasks[:limit]
	}

	items := make([]cloudAgentTaskResponse, len(tasks))
	for i, t := range tasks {
		items[i] = taskToResponse(t)
	}

	var nextCursor *string
	if hasMore && len(tasks) > 0 {
		last := tasks[len(tasks)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		nextCursor = &c
	}

	writeJSON(w, http.StatusOK, paginatedResponse[cloudAgentTaskResponse]{
		Data:       items,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	})
}

// GetTask returns a single task by ID.
func (h *CloudAgentHandler) GetTask(w http.ResponseWriter, r *http.Request) {
	employee := h.authEmployee(w, r)
	if employee == nil {
		return
	}

	agentID, err := uuid.Parse(chi.URLParam(r, "agentID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent_id"})
		return
	}
	if !h.validateSubagent(w, employee.ID, agentID) {
		return
	}

	taskID, err := uuid.Parse(chi.URLParam(r, "taskID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid task_id"})
		return
	}

	var task model.CloudAgentTask
	if err := h.db.Where("id = ? AND employee_agent_id = ? AND cloud_agent_id = ?", taskID, employee.ID, agentID).First(&task).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "task not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load task"})
		return
	}

	writeJSON(w, http.StatusOK, taskToResponse(task))
}

// CreateTask handles POST /internal/employees/{employeeID}/cloud-agents/{agentID}/tasks.
// It synchronously provisions a sandbox, pushes the agent, creates a conversation,
// sends the brief as the first message, and returns the task ID.
func (h *CloudAgentHandler) CreateTask(w http.ResponseWriter, r *http.Request) {
	employee := h.authEmployee(w, r)
	if employee == nil {
		return
	}

	if h.orchestrator == nil || h.pusher == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sandbox orchestrator not configured"})
		return
	}

	agentID, err := uuid.Parse(chi.URLParam(r, "agentID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent_id"})
		return
	}
	if !h.validateSubagent(w, employee.ID, agentID) {
		return
	}

	var req struct {
		Brief                  string         `json:"brief"`
		Metadata               map[string]any `json:"metadata"`
		ParentConversationType string         `json:"parent_conversation_type"`
		ParentConversationID   string         `json:"parent_conversation_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Brief == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "brief is required"})
		return
	}
	if req.ParentConversationType == "" || req.ParentConversationID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "parent_conversation_type and parent_conversation_id are required"})
		return
	}

	var cloudAgent model.Agent
	if err := h.db.Where("id = ?", agentID).First(&cloudAgent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "cloud agent not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load cloud agent"})
		return
	}

	ctx := r.Context()

	sb, err := h.orchestrator.CreateDedicatedSandbox(ctx, &cloudAgent)
	if err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "failed to create sandbox for cloud agent", "agent_id", agentID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to provision sandbox"})
		return
	}

	if err := h.pusher.PushAgentToSandbox(ctx, &cloudAgent, sb); err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "failed to push agent to sandbox", "agent_id", agentID, "sandbox_id", sb.ID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to initialize agent in sandbox"})
		return
	}

	client, err := h.orchestrator.GetBridgeClient(ctx, sb)
	if err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "failed to get bridge client", "sandbox_id", sb.ID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to connect to sandbox"})
		return
	}

	bridgeResp, err := client.CreateConversation(ctx, agentID.String())
	if err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "failed to create conversation in bridge", "agent_id", agentID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create conversation"})
		return
	}

	if err := client.SendMessage(ctx, bridgeResp.ConversationId, req.Brief); err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "failed to send brief to cloud agent", "conversation_id", bridgeResp.ConversationId, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to send task brief"})
		return
	}

	var metadata model.JSON
	if req.Metadata != nil {
		metadata = model.JSON(req.Metadata)
	}

	orgID := *employee.OrgID
	task := model.CloudAgentTask{
		OrgID:                  orgID,
		EmployeeAgentID:        employee.ID,
		CloudAgentID:           agentID,
		SandboxID:              sb.ID,
		ConversationID:         uuid.Nil, // will be set below
		ParentConversationType: req.ParentConversationType,
		ParentConversationID:   req.ParentConversationID,
		Brief:                  req.Brief,
		Metadata:               metadata,
	}

	// We need the AgentConversation ID. Create it now.
	conv := model.AgentConversation{
		OrgID:                orgID,
		AgentID:              agentID,
		SandboxID:            sb.ID,
		BridgeConversationID: bridgeResp.ConversationId,
		Status:               "active",
	}
	if err := h.db.Create(&conv).Error; err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "failed to save conversation", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save conversation"})
		return
	}

	task.ConversationID = conv.ID
	if err := h.db.Create(&task).Error; err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "failed to save cloud agent task", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save task"})
		return
	}

	h.db.Model(sb).Update("last_active_at", time.Now())

	logging.FromContext(ctx).InfoContext(ctx, "cloud agent task created",
		"task_id", task.ID,
		"employee_agent_id", employee.ID,
		"cloud_agent_id", agentID,
		"sandbox_id", sb.ID,
		"conversation_id", conv.ID,
	)

	writeJSON(w, http.StatusCreated, map[string]any{
		"task_id": task.ID.String(),
		"message": fmt.Sprintf("You may use the tool cloud_agent_status(%s) to get progress events from the task, and the tool cloud_agent_send_message to send messages to this agent.", task.ID.String()),
	})
}

// --- Response types ---

type cloudAgentTaskResponse struct {
	ID                     string         `json:"id"`
	CloudAgentID           string         `json:"cloud_agent_id"`
	SandboxID              string         `json:"sandbox_id"`
	ConversationID         string         `json:"conversation_id"`
	ParentConversationType string         `json:"parent_conversation_type"`
	ParentConversationID   string         `json:"parent_conversation_id"`
	Brief                  string         `json:"brief"`
	Metadata               map[string]any `json:"metadata,omitempty"`
	CreatedAt              string         `json:"created_at"`
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

func taskToResponse(t model.CloudAgentTask) cloudAgentTaskResponse {
	var meta map[string]any
	if t.Metadata != nil {
		meta = map[string]any(t.Metadata)
	}
	return cloudAgentTaskResponse{
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
}
