package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

// CreateTask handles POST /internal/employees/{employeeID}/cloud-agents/{agentID}/tasks.
// It synchronously provisions a sandbox, pushes the agent, creates a conversation,
// sends the brief as the first message, and returns the task ID.
func (h *SpecialistTaskHandler) CreateTask(w http.ResponseWriter, r *http.Request) {
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
		extraEnv["HIVY_DRIVE_UPLOAD_URL"] = h.hooks.TaskDriveUploadURL(employee.ID, taskID)
	}

	sb, err := h.hooks.CreateDedicatedSandbox(ctx, &cloudAgent, extraEnv)
	if err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "failed to create sandbox for specialist", "agent_id", agentID, "error", err)
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
		ConversationID:         uuid.Nil,
		ParentConversationType: req.ParentConversationType,
		ParentConversationID:   req.ParentConversationID,
		Brief:                  req.Brief,
		Metadata:               metadata,
	}

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
		logging.FromContext(ctx).ErrorContext(ctx, "failed to save specialist task", "error", err)
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
		logging.FromContext(ctx).ErrorContext(ctx, "failed to send brief to specialist", "conversation_id", bridgeResp.ConversationId, "error", err)
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
	logging.FromContext(ctx).InfoContext(ctx, "specialist task created",
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
