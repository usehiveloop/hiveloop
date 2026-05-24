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

// CreateTask handles POST /internal/employees/{employeeID}/specialists/{specialistSlug}/tasks.
// It synchronously provisions a sandbox, pushes the agent, creates a conversation,
// sends the brief as the first message, and returns the task ID.
func (h *SpecialistTaskHandler) CreateTask(w http.ResponseWriter, r *http.Request) {
	employee := h.authEmployee(w, r)
	if employee == nil {
		return
	}

	if h.hooks.CreateSpecialistSandbox == nil || h.hooks.PushSpecialistToSandbox == nil || h.hooks.GetBridgeClient == nil {
		captureSpecialistFailure(r.Context(), "create_task", errors.New("sandbox orchestrator hook is not configured"), specialistSentryContext{
			Operation:  "configuration",
			OrgID:      uuidValue(employee.OrgID),
			EmployeeID: employee.ID,
		})
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sandbox orchestrator not configured"})
		return
	}

	def, agentID, ok := h.validateSpecialist(r.Context(), w, employee, chi.URLParam(r, "specialistSlug"))
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

	specialistAgent := specialistAgentFromDefinition(employee, *def)
	ctx := r.Context()
	taskID := uuid.New()
	extraEnv := map[string]string{}
	if h.hooks.TaskDriveUploadURL != nil {
		extraEnv["HIVY_DRIVE_UPLOAD_URL"] = h.hooks.TaskDriveUploadURL(employee.ID, taskID)
	}

	sb, err := h.hooks.CreateSpecialistSandbox(ctx, &specialistAgent, extraEnv)
	if err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "failed to create sandbox for specialist", "employee_id", agentID, "error", err)
		captureSpecialistFailure(ctx, "create_task", err, specialistSentryContext{
			Operation:    "create_specialist_sandbox",
			OrgID:        uuidValue(employee.OrgID),
			EmployeeID:   employee.ID,
			SpecialistID: agentID,
			TaskID:       taskID,
		})
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to provision sandbox"})
		return
	}

	if err := h.hooks.PushSpecialistToSandbox(ctx, &specialistAgent, sb); err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "failed to push agent to sandbox", "employee_id", agentID, "sandbox_id", sb.ID, "error", err)
		captureSpecialistFailure(ctx, "create_task", err, specialistSentryContext{
			Operation:    "push_agent_to_sandbox",
			OrgID:        uuidValue(employee.OrgID),
			EmployeeID:   employee.ID,
			SpecialistID: agentID,
			TaskID:       taskID,
			SandboxID:    sb.ID,
		})
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to initialize agent in sandbox"})
		return
	}

	client, err := h.hooks.GetBridgeClient(ctx, sb)
	if err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "failed to get bridge client", "sandbox_id", sb.ID, "error", err)
		captureSpecialistFailure(ctx, "create_task", err, specialistSentryContext{
			Operation:    "get_bridge_client",
			OrgID:        uuidValue(employee.OrgID),
			EmployeeID:   employee.ID,
			SpecialistID: agentID,
			TaskID:       taskID,
			SandboxID:    sb.ID,
		})
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to connect to sandbox"})
		return
	}

	bridgeResp, err := client.CreateConversation(ctx, agentID.String())
	if err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "failed to create conversation in bridge", "employee_id", agentID, "error", err)
		captureSpecialistFailure(ctx, "create_task", err, specialistSentryContext{
			Operation:    "create_bridge_conversation",
			OrgID:        uuidValue(employee.OrgID),
			EmployeeID:   employee.ID,
			SpecialistID: agentID,
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
	task := model.SpecialistTask{
		ID:                     taskID,
		OrgID:                  orgID,
		EmployeeID:             employee.ID,
		SpecialistID:           agentID,
		SandboxID:              sb.ID,
		ConversationID:         uuid.Nil,
		ParentConversationType: req.ParentConversationType,
		ParentConversationID:   req.ParentConversationID,
		Brief:                  req.Brief,
		Metadata:               metadata,
	}

	conv := model.EmployeeConversation{
		OrgID:                 orgID,
		EmployeeID:            agentID,
		SandboxID:             sb.ID,
		RuntimeConversationID: bridgeResp.ConversationId,
		Status:                "active",
	}
	if err := h.db.Create(&conv).Error; err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "failed to save conversation", "error", err)
		captureSpecialistFailure(ctx, "create_task", err, specialistSentryContext{
			Operation:      "save_conversation",
			OrgID:          orgID,
			EmployeeID:     employee.ID,
			SpecialistID:   agentID,
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
		captureSpecialistFailure(ctx, "create_task", err, specialistSentryContext{
			Operation:      "save_task",
			OrgID:          orgID,
			EmployeeID:     employee.ID,
			SpecialistID:   agentID,
			TaskID:         taskID,
			SandboxID:      sb.ID,
			ConversationID: conv.ID,
		})
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save task"})
		return
	}

	if err := client.SendMessage(ctx, bridgeResp.ConversationId, req.Brief); err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "failed to send brief to specialist", "conversation_id", bridgeResp.ConversationId, "error", err)
		captureSpecialistFailure(ctx, "create_task", err, specialistSentryContext{
			Operation:      "send_initial_message",
			OrgID:          orgID,
			EmployeeID:     employee.ID,
			SpecialistID:   agentID,
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
		"employee_id", employee.ID,
		"specialist_id", agentID,
		"sandbox_id", sb.ID,
		"conversation_id", conv.ID,
	)

	writeJSON(w, http.StatusCreated, map[string]any{
		"task_id": task.ID.String(),
		"message": fmt.Sprintf("You may use the tool specialist_task_status(%s) to get progress events from the task, and the tool specialist_task_send_message to send messages to this specialist.", task.ID.String()),
	})
}
