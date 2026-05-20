package handler

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/bridge"
	"github.com/usehiveloop/hiveloop/internal/bridgeevents"
	"github.com/usehiveloop/hiveloop/internal/crypto"
	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/sandbox"
)

// CloudAgentBridgeClient is the subset of bridge runtime operations used by
// cloud-agent coordination. Keeping this seam small makes the bridge contract
// testable without provisioning a real sandbox.
type CloudAgentBridgeClient interface {
	CreateConversation(ctx context.Context, agentID string) (*bridge.CreateConversationResponse, error)
	SendMessage(ctx context.Context, convID string, content string) error
	EndConversation(ctx context.Context, convID string) error
}

type CloudAgentHandlerHooks struct {
	CreateDedicatedSandbox  func(ctx context.Context, agent *model.Agent, extraEnv map[string]string) (*model.Sandbox, error)
	PushAgentToSandbox      func(ctx context.Context, agent *model.Agent, sb *model.Sandbox) error
	GetBridgeClient         func(ctx context.Context, sb *model.Sandbox) (CloudAgentBridgeClient, error)
	StopSandbox             func(ctx context.Context, sb *model.Sandbox) error
	DeleteSandbox           func(ctx context.Context, sb *model.Sandbox) error
	TaskDriveUploadURL      func(employeeID uuid.UUID, taskID uuid.UUID) string
	EmployeeCallbackRuntime employeeCallbackSandboxRuntime
}

type CloudAgentHandler struct {
	db     *gorm.DB
	encKey *crypto.SymmetricKey
	hooks  CloudAgentHandlerHooks
}

func NewCloudAgentHandler(db *gorm.DB, encKey *crypto.SymmetricKey, orchestrator *sandbox.Orchestrator, pusher *sandbox.Pusher) *CloudAgentHandler {
	hooks := CloudAgentHandlerHooks{}
	if orchestrator != nil {
		hooks.CreateDedicatedSandbox = orchestrator.CreateDedicatedSandboxWithEnv
		hooks.GetBridgeClient = func(ctx context.Context, sb *model.Sandbox) (CloudAgentBridgeClient, error) {
			return orchestrator.GetBridgeClient(ctx, sb)
		}
		hooks.StopSandbox = orchestrator.StopSandbox
		hooks.DeleteSandbox = orchestrator.DeleteSandboxResource
		hooks.TaskDriveUploadURL = orchestrator.EmployeeTaskDriveUploadURL
		hooks.EmployeeCallbackRuntime = orchestrator
	}
	if pusher != nil {
		hooks.PushAgentToSandbox = pusher.PushAgentToSandbox
	}
	return NewCloudAgentHandlerWithHooks(db, encKey, hooks)
}

func NewCloudAgentHandlerWithHooks(db *gorm.DB, encKey *crypto.SymmetricKey, hooks CloudAgentHandlerHooks) *CloudAgentHandler {
	return &CloudAgentHandler{db: db, encKey: encKey, hooks: hooks}
}

// authEmployee verifies the bridge bearer token for the employee in the URL.
// On failure it writes the error response and returns nil — callers must return.
func (h *CloudAgentHandler) authEmployee(w http.ResponseWriter, r *http.Request) *model.Agent {
	if h.encKey == nil {
		captureCloudAgentFailure(r.Context(), "auth", errors.New("encryption key is not configured"), cloudAgentSentryContext{
			Operation: "configuration",
		})
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
	if err := h.db.Where("id = ? AND is_employee = true", agentID).First(&agent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "employee not found"})
			return nil
		}
		captureCloudAgentFailure(r.Context(), "auth", err, cloudAgentSentryContext{
			Operation:  "load_employee",
			EmployeeID: agentID,
		})
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
		captureCloudAgentFailure(r.Context(), "auth", err, cloudAgentSentryContext{
			Operation:  "load_employee_sandbox",
			EmployeeID: agentID,
			OrgID:      uuidValue(agent.OrgID),
		})
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load sandbox"})
		return nil
	}

	wantKey, err := h.encKey.DecryptString(sb.EncryptedBridgeAPIKey)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "decrypt bridge api key", "agent_id", agentID, "error", err)
		captureCloudAgentFailure(r.Context(), "auth", err, cloudAgentSentryContext{
			Operation:  "decrypt_bridge_key",
			EmployeeID: agentID,
			OrgID:      uuidValue(agent.OrgID),
			SandboxID:  sb.ID,
		})
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to verify credentials"})
		return nil
	}
	if subtle.ConstantTimeCompare([]byte(bearer), []byte(wantKey)) != 1 {
		captureCloudAgentWarning(r.Context(), "auth", errors.New("invalid bridge api key"), cloudAgentSentryContext{
			Operation:  "invalid_bridge_key",
			EmployeeID: agentID,
			OrgID:      uuidValue(agent.OrgID),
			SandboxID:  sb.ID,
		})
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid bridge api key"})
		return nil
	}

	return &agent
}

func specialistID(slug string) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte("hiveloop-specialist:"+slug))
}

func specialistAgentFromTemplate(employee *model.Agent, template *employeeAgentTemplate) model.Agent {
	description := template.Description
	orgID := uuidValue(employee.OrgID)
	return model.Agent{
		ID:          specialistID(template.Slug),
		OrgID:       &orgID,
		Name:        template.Name,
		Description: &description,
		Model:       employee.Model,
		Tools:       employee.Tools,
		McpServers:  employee.McpServers,
		Skills:      employee.Skills,
		AgentConfig: employee.AgentConfig,
		Permissions: employee.Permissions,
		Resources:   employee.Resources,
		Harness:     employeeCloudAgentHarness,
		Status:      "active",
	}
}

// validateSpecialist checks that slug refers to an enabled code-catalog specialist.
func (h *CloudAgentHandler) validateSpecialist(ctx context.Context, w http.ResponseWriter, employee *model.Agent, slug string) (*employeeAgentTemplate, uuid.UUID, bool) {
	template := employeeAgentTemplateBySlug(slug)
	if template == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "specialist not found for this employee"})
		return nil, uuid.Nil, false
	}
	if disabledSpecialistSet(employee.DisabledSpecialists)[slug] {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "specialist disabled for this employee"})
		return nil, uuid.Nil, false
	}
	return template, specialistID(slug), true
}

// ListCloudAgents returns all cloud agents for the employee with their top 3
// most recent tasks and top 10 most recent events per task.
func (h *CloudAgentHandler) ListCloudAgents(w http.ResponseWriter, r *http.Request) {
	employee := h.authEmployee(w, r)
	if employee == nil {
		return
	}

	disabled := disabledSpecialistSet(employee.DisabledSpecialists)
	agents := make([]model.Agent, 0, len(employeeAgentTemplates))
	subagentIDs := make([]uuid.UUID, 0, len(employeeAgentTemplates))
	for i := range employeeAgentTemplates {
		template := &employeeAgentTemplates[i]
		if disabled[template.Slug] {
			continue
		}
		agent := specialistAgentFromTemplate(employee, template)
		agents = append(agents, agent)
		subagentIDs = append(subagentIDs, agent.ID)
	}

	// Load top 3 tasks per cloud agent
	var allTasks []model.CloudAgentTask
	if err := h.db.
		Where("employee_agent_id = ? AND cloud_agent_id IN ?", employee.ID, subagentIDs).
		Order("created_at DESC").
		Find(&allTasks).Error; err != nil {
		captureCloudAgentFailure(r.Context(), "list_cloud_agents", err, cloudAgentSentryContext{
			Operation:  "load_recent_tasks",
			OrgID:      uuidValue(employee.OrgID),
			EmployeeID: employee.ID,
		})
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
		if err := h.db.Raw(`
			SELECT id, conversation_id, event_type, data, created_at, rn
			FROM (
				SELECT id, conversation_id, event_type, data, created_at,
					ROW_NUMBER() OVER (PARTITION BY conversation_id ORDER BY created_at DESC) AS rn
				FROM conversation_events
				WHERE conversation_id IN ?
			) ranked
			WHERE rn <= 10
		`, convIDs).Scan(&rows).Error; err != nil {
			captureCloudAgentFailure(r.Context(), "list_cloud_agents", err, cloudAgentSentryContext{
				Operation:  "load_recent_events",
				OrgID:      uuidValue(employee.OrgID),
				EmployeeID: employee.ID,
			})
		}

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

	_, agentID, ok := h.validateSpecialist(r.Context(), w, employee, chi.URLParam(r, "specialistSlug"))
	if !ok {
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
		captureCloudAgentFailure(r.Context(), "list_tasks", err, cloudAgentSentryContext{
			Operation:    "load_tasks",
			OrgID:        uuidValue(employee.OrgID),
			EmployeeID:   employee.ID,
			CloudAgentID: agentID,
		})
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load tasks"})
		return
	}

	hasMore := len(tasks) > limit
	if hasMore {
		tasks = tasks[:limit]
	}

	eventsByConv := h.recentEventsByConversation(conversationIDsForTasks(tasks), 10)

	items := make([]cloudAgentTaskResponse, len(tasks))
	for i, t := range tasks {
		items[i] = taskToResponse(t, eventsByConv[t.ConversationID])
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

	_, agentID, ok := h.validateSpecialist(r.Context(), w, employee, chi.URLParam(r, "specialistSlug"))
	if !ok {
		return
	}

	taskID, err := uuid.Parse(chi.URLParam(r, "taskID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid task_id"})
		return
	}

	task, ok := h.loadTask(r.Context(), w, employee, agentID, taskID)
	if !ok {
		return
	}

	events := h.recentEventsForConversation(task.ConversationID, 10)
	writeJSON(w, http.StatusOK, taskToResponse(*task, events))
}

// CreateTask handles POST /internal/employees/{employeeID}/cloud-agents/{agentID}/tasks.
// It synchronously provisions a sandbox, pushes the agent, creates a conversation,
// sends the brief as the first message, and returns the task ID.
func (h *CloudAgentHandler) CreateTask(w http.ResponseWriter, r *http.Request) {
	employee := h.authEmployee(w, r)
	if employee == nil {
		return
	}

	if h.hooks.CreateDedicatedSandbox == nil || h.hooks.PushAgentToSandbox == nil || h.hooks.GetBridgeClient == nil {
		captureCloudAgentFailure(r.Context(), "create_task", errors.New("sandbox orchestrator hook is not configured"), cloudAgentSentryContext{
			Operation:  "configuration",
			OrgID:      uuidValue(employee.OrgID),
			EmployeeID: employee.ID,
		})
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sandbox orchestrator not configured"})
		return
	}

	template, agentID, ok := h.validateSpecialist(r.Context(), w, employee, chi.URLParam(r, "specialistSlug"))
	if !ok {
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

	cloudAgent := specialistAgentFromTemplate(employee, template)

	ctx := r.Context()

	taskID := uuid.New()
	extraEnv := map[string]string{}
	if h.hooks.TaskDriveUploadURL != nil {
		extraEnv["HIVELOOP_DRIVE_UPLOAD_URL"] = h.hooks.TaskDriveUploadURL(employee.ID, taskID)
	}

	sb, err := h.hooks.CreateDedicatedSandbox(ctx, &cloudAgent, extraEnv)
	if err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "failed to create sandbox for cloud agent", "agent_id", agentID, "error", err)
		captureCloudAgentFailure(ctx, "create_task", err, cloudAgentSentryContext{
			Operation:    "create_dedicated_sandbox",
			OrgID:        uuidValue(employee.OrgID),
			EmployeeID:   employee.ID,
			CloudAgentID: agentID,
			TaskID:       taskID,
		})
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to provision sandbox"})
		return
	}

	if err := h.hooks.PushAgentToSandbox(ctx, &cloudAgent, sb); err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "failed to push agent to sandbox", "agent_id", agentID, "sandbox_id", sb.ID, "error", err)
		captureCloudAgentFailure(ctx, "create_task", err, cloudAgentSentryContext{
			Operation:    "push_agent_to_sandbox",
			OrgID:        uuidValue(employee.OrgID),
			EmployeeID:   employee.ID,
			CloudAgentID: agentID,
			TaskID:       taskID,
			SandboxID:    sb.ID,
		})
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to initialize agent in sandbox"})
		return
	}

	client, err := h.hooks.GetBridgeClient(ctx, sb)
	if err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "failed to get bridge client", "sandbox_id", sb.ID, "error", err)
		captureCloudAgentFailure(ctx, "create_task", err, cloudAgentSentryContext{
			Operation:    "get_bridge_client",
			OrgID:        uuidValue(employee.OrgID),
			EmployeeID:   employee.ID,
			CloudAgentID: agentID,
			TaskID:       taskID,
			SandboxID:    sb.ID,
		})
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to connect to sandbox"})
		return
	}

	bridgeResp, err := client.CreateConversation(ctx, agentID.String())
	if err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "failed to create conversation in bridge", "agent_id", agentID, "error", err)
		captureCloudAgentFailure(ctx, "create_task", err, cloudAgentSentryContext{
			Operation:    "create_bridge_conversation",
			OrgID:        uuidValue(employee.OrgID),
			EmployeeID:   employee.ID,
			CloudAgentID: agentID,
			TaskID:       taskID,
			SandboxID:    sb.ID,
		})
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create conversation"})
		return
	}

	var metadata model.JSON
	if req.Metadata != nil {
		metadata = model.JSON(req.Metadata)
	}

	orgID := *employee.OrgID
	task := model.CloudAgentTask{
		ID:                     taskID,
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
		OrgID:                 orgID,
		AgentID:               agentID,
		SandboxID:             sb.ID,
		RuntimeConversationID: bridgeResp.ConversationId,
		Status:                "active",
	}
	if err := h.db.Create(&conv).Error; err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "failed to save conversation", "error", err)
		captureCloudAgentFailure(ctx, "create_task", err, cloudAgentSentryContext{
			Operation:      "save_conversation",
			OrgID:          orgID,
			EmployeeID:     employee.ID,
			CloudAgentID:   agentID,
			TaskID:         taskID,
			SandboxID:      sb.ID,
			ConversationID: conv.ID,
		})
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save conversation"})
		return
	}

	task.ConversationID = conv.ID
	if err := h.db.Create(&task).Error; err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "failed to save cloud agent task", "error", err)
		captureCloudAgentFailure(ctx, "create_task", err, cloudAgentSentryContext{
			Operation:      "save_task",
			OrgID:          orgID,
			EmployeeID:     employee.ID,
			CloudAgentID:   agentID,
			TaskID:         taskID,
			SandboxID:      sb.ID,
			ConversationID: conv.ID,
		})
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save task"})
		return
	}

	if err := client.SendMessage(ctx, bridgeResp.ConversationId, req.Brief); err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "failed to send brief to cloud agent", "conversation_id", bridgeResp.ConversationId, "error", err)
		captureCloudAgentFailure(ctx, "create_task", err, cloudAgentSentryContext{
			Operation:      "send_initial_message",
			OrgID:          orgID,
			EmployeeID:     employee.ID,
			CloudAgentID:   agentID,
			TaskID:         taskID,
			SandboxID:      sb.ID,
			ConversationID: conv.ID,
		})
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to send task brief"})
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
		"message": fmt.Sprintf("You may use the tool cloud_agent_task_status(%s) to get progress events from the task, and the tool cloud_agent_task_send_message to send messages to this agent.", task.ID.String()),
	})
}

// SendTaskMessage forwards coordinator feedback to an existing cloud-agent task.
func (h *CloudAgentHandler) SendTaskMessage(w http.ResponseWriter, r *http.Request) {
	employee := h.authEmployee(w, r)
	if employee == nil {
		return
	}
	if h.hooks.GetBridgeClient == nil {
		captureCloudAgentFailure(r.Context(), "send_message", errors.New("sandbox bridge client hook is not configured"), cloudAgentSentryContext{
			Operation:  "configuration",
			OrgID:      uuidValue(employee.OrgID),
			EmployeeID: employee.ID,
		})
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sandbox bridge client not configured"})
		return
	}

	agentID, taskID, ok := h.parseAgentAndTaskIDs(w, r)
	if !ok {
		return
	}
	if _, _, ok := h.validateSpecialist(r.Context(), w, employee, chi.URLParam(r, "specialistSlug")); !ok {
		return
	}

	var req struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if strings.TrimSpace(req.Message) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "message is required"})
		return
	}

	task, ok := h.loadTask(r.Context(), w, employee, agentID, taskID)
	if !ok {
		return
	}
	conv, sb, ok := h.loadTaskRuntime(r.Context(), w, *task)
	if !ok {
		return
	}

	ctx := r.Context()
	client, err := h.hooks.GetBridgeClient(ctx, sb)
	if err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "failed to get bridge client", "sandbox_id", sb.ID, "error", err)
		captureCloudAgentFailure(ctx, "send_message", err, cloudAgentSentryContext{
			Operation:      "get_bridge_client",
			OrgID:          uuidValue(employee.OrgID),
			EmployeeID:     employee.ID,
			CloudAgentID:   agentID,
			TaskID:         task.ID,
			SandboxID:      sb.ID,
			ConversationID: conv.ID,
		})
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to connect to sandbox"})
		return
	}
	if err := client.SendMessage(ctx, conv.RuntimeConversationID, req.Message); err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "failed to send message to cloud agent task", "task_id", task.ID, "conversation_id", conv.RuntimeConversationID, "error", err)
		captureCloudAgentFailure(ctx, "send_message", err, cloudAgentSentryContext{
			Operation:      "send_bridge_message",
			OrgID:          uuidValue(employee.OrgID),
			EmployeeID:     employee.ID,
			CloudAgentID:   agentID,
			TaskID:         task.ID,
			SandboxID:      sb.ID,
			ConversationID: conv.ID,
		})
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to send message"})
		return
	}
	h.db.Model(sb).Update("last_active_at", time.Now())

	writeJSON(w, http.StatusOK, map[string]any{"success": true, "message": "message sent"})
}

// TerminateTask logically ends a cloud-agent task and asks the runtime to stop
// the backing conversation. Sandbox deletion is best-effort so the task record
// and terminal event remain durable even when infrastructure cleanup is delayed.
func (h *CloudAgentHandler) TerminateTask(w http.ResponseWriter, r *http.Request) {
	employee := h.authEmployee(w, r)
	if employee == nil {
		return
	}

	agentID, taskID, ok := h.parseAgentAndTaskIDs(w, r)
	if !ok {
		return
	}
	if _, _, ok := h.validateSpecialist(r.Context(), w, employee, chi.URLParam(r, "specialistSlug")); !ok {
		return
	}

	reason := "terminated by coordinator"
	var req struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if strings.TrimSpace(req.Reason) != "" {
		reason = strings.TrimSpace(req.Reason)
	}

	task, ok := h.loadTask(r.Context(), w, employee, agentID, taskID)
	if !ok {
		return
	}
	conv, sb, ok := h.loadTaskRuntime(r.Context(), w, *task)
	if !ok {
		return
	}

	ctx := r.Context()
	if h.hooks.GetBridgeClient != nil {
		client, err := h.hooks.GetBridgeClient(ctx, sb)
		if err != nil {
			logging.FromContext(ctx).WarnContext(ctx, "failed to get bridge client for task termination", "sandbox_id", sb.ID, "error", err)
			captureCloudAgentWarning(ctx, "terminate_task", err, cloudAgentSentryContext{
				Operation:      "get_bridge_client",
				OrgID:          uuidValue(employee.OrgID),
				EmployeeID:     employee.ID,
				CloudAgentID:   agentID,
				TaskID:         task.ID,
				SandboxID:      sb.ID,
				ConversationID: conv.ID,
			})
		} else if err := client.EndConversation(ctx, conv.RuntimeConversationID); err != nil {
			logging.FromContext(ctx).WarnContext(ctx, "failed to end cloud agent bridge conversation", "task_id", task.ID, "conversation_id", conv.RuntimeConversationID, "error", err)
			captureCloudAgentWarning(ctx, "terminate_task", err, cloudAgentSentryContext{
				Operation:      "end_bridge_conversation",
				OrgID:          uuidValue(employee.OrgID),
				EmployeeID:     employee.ID,
				CloudAgentID:   agentID,
				TaskID:         task.ID,
				SandboxID:      sb.ID,
				ConversationID: conv.ID,
			})
		}
	}

	now := time.Now().UTC()
	if err := h.db.Model(&model.AgentConversation{}).
		Where("id = ?", conv.ID).
		Updates(map[string]any{"status": "ended", "ended_at": now}).Error; err != nil {
		captureCloudAgentFailure(ctx, "terminate_task", err, cloudAgentSentryContext{
			Operation:      "mark_conversation_ended",
			OrgID:          uuidValue(employee.OrgID),
			EmployeeID:     employee.ID,
			CloudAgentID:   agentID,
			TaskID:         task.ID,
			SandboxID:      sb.ID,
			ConversationID: conv.ID,
		})
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to mark task ended"})
		return
	}
	h.ensureConversationEndedEvent(ctx, *task, *conv, reason, now)

	if h.hooks.DeleteSandbox != nil {
		if err := h.hooks.DeleteSandbox(ctx, sb); err != nil {
			logging.FromContext(ctx).WarnContext(ctx, "failed to delete cloud agent sandbox after termination", "task_id", task.ID, "sandbox_id", sb.ID, "error", err)
			captureCloudAgentWarning(ctx, "terminate_task", err, cloudAgentSentryContext{
				Operation:      "delete_sandbox",
				OrgID:          uuidValue(employee.OrgID),
				EmployeeID:     employee.ID,
				CloudAgentID:   agentID,
				TaskID:         task.ID,
				SandboxID:      sb.ID,
				ConversationID: conv.ID,
			})
		}
	} else if h.hooks.StopSandbox != nil {
		if err := h.hooks.StopSandbox(ctx, sb); err != nil {
			logging.FromContext(ctx).WarnContext(ctx, "failed to stop cloud agent sandbox after termination", "task_id", task.ID, "sandbox_id", sb.ID, "error", err)
			captureCloudAgentWarning(ctx, "terminate_task", err, cloudAgentSentryContext{
				Operation:      "stop_sandbox",
				OrgID:          uuidValue(employee.OrgID),
				EmployeeID:     employee.ID,
				CloudAgentID:   agentID,
				TaskID:         task.ID,
				SandboxID:      sb.ID,
				ConversationID: conv.ID,
			})
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"success": true, "message": "task terminated"})
}

// --- Response types ---

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

func (h *CloudAgentHandler) parseAgentAndTaskIDs(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	slug := chi.URLParam(r, "specialistSlug")
	if employeeAgentTemplateBySlug(slug) == nil {
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

func (h *CloudAgentHandler) loadTask(ctx context.Context, w http.ResponseWriter, employee *model.Agent, agentID, taskID uuid.UUID) (*model.CloudAgentTask, bool) {
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

func (h *CloudAgentHandler) loadTaskRuntime(ctx context.Context, w http.ResponseWriter, task model.CloudAgentTask) (*model.AgentConversation, *model.Sandbox, bool) {
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

func (h *CloudAgentHandler) recentEventsForConversation(conversationID uuid.UUID, limit int) []model.ConversationEvent {
	eventsByConv := h.recentEventsByConversation([]uuid.UUID{conversationID}, limit)
	return eventsByConv[conversationID]
}

func (h *CloudAgentHandler) recentEventsByConversation(conversationIDs []uuid.UUID, limit int) map[uuid.UUID][]model.ConversationEvent {
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

func (h *CloudAgentHandler) ensureConversationEndedEvent(ctx context.Context, task model.CloudAgentTask, conv model.AgentConversation, reason string, now time.Time) {
	var count int64
	if err := h.db.Model(&model.ConversationEvent{}).
		Where("conversation_id = ? AND event_type IN ?", conv.ID, []string{bridgeevents.EventConversationEnded, "ConversationEnded"}).
		Count(&count).Error; err != nil || count > 0 {
		if err != nil {
			logging.FromContext(ctx).ErrorContext(ctx, "failed to check existing conversation ended event", "conversation_id", conv.ID, "error", err)
			captureCloudAgentFailure(ctx, "terminate_task", err, cloudAgentSentryContext{
				Operation:      "check_existing_ended_event",
				OrgID:          task.OrgID,
				EmployeeID:     task.EmployeeAgentID,
				CloudAgentID:   task.CloudAgentID,
				TaskID:         task.ID,
				SandboxID:      task.SandboxID,
				ConversationID: conv.ID,
			})
		}
		return
	}

	var maxSequence int64
	if err := h.db.Model(&model.ConversationEvent{}).
		Select("COALESCE(MAX(sequence_number), 0)").
		Where("conversation_id = ?", conv.ID).
		Scan(&maxSequence).Error; err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "failed to load conversation event sequence", "conversation_id", conv.ID, "error", err)
		captureCloudAgentFailure(ctx, "terminate_task", err, cloudAgentSentryContext{
			Operation:      "load_event_sequence",
			OrgID:          task.OrgID,
			EmployeeID:     task.EmployeeAgentID,
			CloudAgentID:   task.CloudAgentID,
			TaskID:         task.ID,
			SandboxID:      task.SandboxID,
			ConversationID: conv.ID,
		})
		return
	}

	data, _ := json.Marshal(map[string]any{
		"reason": reason,
		"source": "cloud_agent_terminate",
	})
	event := model.ConversationEvent{
		OrgID:                 conv.OrgID,
		ConversationID:        conv.ID,
		EventID:               uuid.New().String(),
		EventType:             bridgeevents.EventConversationEnded,
		AgentID:               conv.AgentID.String(),
		RuntimeConversationID: conv.RuntimeConversationID,
		Timestamp:             now,
		SequenceNumber:        maxSequence + 1,
		Data:                  model.RawJSON(data),
	}
	if err := h.db.Create(&event).Error; err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "failed to append conversation ended event", "conversation_id", conv.ID, "error", err)
		captureCloudAgentFailure(ctx, "terminate_task", err, cloudAgentSentryContext{
			Operation:      "append_ended_event",
			OrgID:          task.OrgID,
			EmployeeID:     task.EmployeeAgentID,
			CloudAgentID:   task.CloudAgentID,
			TaskID:         task.ID,
			SandboxID:      task.SandboxID,
			ConversationID: conv.ID,
		})
		return
	}

	if err := dispatchCloudAgentCallback(ctx, h.db, h.encKey, h.hooks.EmployeeCallbackRuntime, task, event); err != nil {
		logging.FromContext(ctx).WarnContext(ctx, "failed to forward cloud agent termination to employee bridge",
			"task_id", task.ID,
			"conversation_id", conv.ID,
			"error", err,
		)
		captureCloudAgentWarning(ctx, "terminate_task", err, cloudAgentSentryContext{
			Operation:      "dispatch_termination_callback",
			OrgID:          task.OrgID,
			EmployeeID:     task.EmployeeAgentID,
			CloudAgentID:   task.CloudAgentID,
			TaskID:         task.ID,
			SandboxID:      task.SandboxID,
			ConversationID: conv.ID,
		})
	}
}
