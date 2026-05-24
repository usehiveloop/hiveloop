package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

// SendTaskMessage forwards coordinator feedback to an existing specialist task.
func (h *SpecialistTaskHandler) SendTaskMessage(w http.ResponseWriter, r *http.Request) {
	employee := h.authEmployee(w, r)
	if employee == nil {
		return
	}
	if h.hooks.GetBridgeClient == nil {
		captureSpecialistFailure(r.Context(), "send_message", errors.New("sandbox bridge client hook is not configured"), specialistSentryContext{
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
	if _, _, ok := h.validateSpecialist(r.Context(), w, employee, specialistSlugParam(r)); !ok {
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
		captureSpecialistFailure(ctx, "send_message", err, specialistSentryContext{
			Operation:      "get_bridge_client",
			OrgID:          uuidValue(employee.OrgID),
			EmployeeID:     employee.ID,
			SpecialistID:   agentID,
			TaskID:         task.ID,
			SandboxID:      sb.ID,
			ConversationID: conv.ID,
		})
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to connect to sandbox"})
		return
	}
	if err := client.SendMessage(ctx, conv.RuntimeConversationID, req.Message); err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "failed to send message to specialist task", "task_id", task.ID, "conversation_id", conv.RuntimeConversationID, "error", err)
		captureSpecialistFailure(ctx, "send_message", err, specialistSentryContext{
			Operation:      "send_bridge_message",
			OrgID:          uuidValue(employee.OrgID),
			EmployeeID:     employee.ID,
			SpecialistID:   agentID,
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

// TerminateTask logically ends a specialist task and asks the runtime to stop
// the backing conversation. Sandbox deletion is best-effort so the task record
// and terminal event remain durable even when infrastructure cleanup is delayed.
func (h *SpecialistTaskHandler) TerminateTask(w http.ResponseWriter, r *http.Request) {
	employee := h.authEmployee(w, r)
	if employee == nil {
		return
	}

	agentID, taskID, ok := h.parseAgentAndTaskIDs(w, r)
	if !ok {
		return
	}
	if _, _, ok := h.validateSpecialist(r.Context(), w, employee, specialistSlugParam(r)); !ok {
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
	h.endBridgeConversation(ctx, employee, agentID, *task, *conv, sb)

	now := time.Now().UTC()
	if err := h.db.Model(&model.EmployeeConversation{}).
		Where("id = ?", conv.ID).
		Updates(map[string]any{"status": "ended", "ended_at": now}).Error; err != nil {
		captureSpecialistFailure(ctx, "terminate_task", err, specialistSentryContext{
			Operation:      "mark_conversation_ended",
			OrgID:          uuidValue(employee.OrgID),
			EmployeeID:     employee.ID,
			SpecialistID:   agentID,
			TaskID:         task.ID,
			SandboxID:      sb.ID,
			ConversationID: conv.ID,
		})
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to mark task ended"})
		return
	}
	h.ensureConversationEndedEvent(ctx, *task, *conv, reason, now)
	h.cleanupTaskSandbox(ctx, employee, agentID, *task, *conv, sb)

	writeJSON(w, http.StatusOK, map[string]any{"success": true, "message": "task terminated"})
}

func (h *SpecialistTaskHandler) endBridgeConversation(ctx context.Context, employee *model.Employee, agentID uuid.UUID, task model.SpecialistTask, conv model.EmployeeConversation, sb *model.Sandbox) {
	if h.hooks.GetBridgeClient == nil {
		return
	}
	client, err := h.hooks.GetBridgeClient(ctx, sb)
	if err != nil {
		logging.FromContext(ctx).WarnContext(ctx, "failed to get bridge client for task termination", "sandbox_id", sb.ID, "error", err)
		captureSpecialistWarning(ctx, "terminate_task", err, terminateTaskSentryContext("get_bridge_client", employee, agentID, task, conv, sb.ID))
		return
	}
	if err := client.EndConversation(ctx, conv.RuntimeConversationID); err != nil {
		logging.FromContext(ctx).WarnContext(ctx, "failed to end specialist bridge conversation", "task_id", task.ID, "conversation_id", conv.RuntimeConversationID, "error", err)
		captureSpecialistWarning(ctx, "terminate_task", err, terminateTaskSentryContext("end_bridge_conversation", employee, agentID, task, conv, sb.ID))
	}
}

func (h *SpecialistTaskHandler) cleanupTaskSandbox(ctx context.Context, employee *model.Employee, agentID uuid.UUID, task model.SpecialistTask, conv model.EmployeeConversation, sb *model.Sandbox) {
	if h.hooks.DeleteSandbox != nil {
		if err := h.hooks.DeleteSandbox(ctx, sb); err != nil {
			logging.FromContext(ctx).WarnContext(ctx, "failed to delete specialist sandbox after termination", "task_id", task.ID, "sandbox_id", sb.ID, "error", err)
			captureSpecialistWarning(ctx, "terminate_task", err, terminateTaskSentryContext("delete_sandbox", employee, agentID, task, conv, sb.ID))
		}
		return
	}
	if h.hooks.StopSandbox != nil {
		if err := h.hooks.StopSandbox(ctx, sb); err != nil {
			logging.FromContext(ctx).WarnContext(ctx, "failed to stop specialist sandbox after termination", "task_id", task.ID, "sandbox_id", sb.ID, "error", err)
			captureSpecialistWarning(ctx, "terminate_task", err, terminateTaskSentryContext("stop_sandbox", employee, agentID, task, conv, sb.ID))
		}
	}
}
