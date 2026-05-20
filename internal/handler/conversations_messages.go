package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	bridgepkg "github.com/usehivy/hivy/internal/bridge"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

// SendMessage handles POST /v1/conversations/{convID}/messages.
// @Summary Send a message
// @Description Sends a message to the agent in the conversation. Returns 202 immediately; response streams via SSE.
// @Tags conversations
// @Accept json
// @Produce json
// @Param convID path string true "Conversation ID"
// @Param body body object{content=string} true "Message content"
// @Success 202 {object} map[string]string
// @Failure 400 {object} errorResponse
// @Failure 410 {object} errorResponse
// @Security BearerAuth
// @Router /v1/conversations/{convID}/messages [post]
func (h *ConversationHandler) SendMessage(w http.ResponseWriter, r *http.Request) {
	conv, ok := h.loadConversation(w, r)
	if !ok {
		return
	}

	var req struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Content == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "content is required"})
		return
	}

	if h.pusher != nil && conv.CredentialID == nil && h.pusher.NeedsTokenRotation(conv.AgentID.String()) {
		var agent model.Agent
		if err := h.db.Where("id = ?", conv.AgentID).First(&agent).Error; err == nil {
			if err := h.pusher.RotateAgentToken(r.Context(), &agent, &conv.Sandbox); err != nil {
				logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to rotate agent token", "agent_id", conv.AgentID, "error", err)

			}
		}
	}

	client, ok := h.getBridgeClient(w, r, conv)
	if !ok {
		return
	}

	if err := client.SendMessage(r.Context(), conv.RuntimeConversationID, req.Content); err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to send message to bridge", "conversation_id", conv.ID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to send message"})
		return
	}

	h.db.Model(&conv.Sandbox).Update("last_active_at", time.Now())

	writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
}

// Abort handles POST /v1/conversations/{convID}/abort.
// @Summary Abort current turn
// @Description Cancels the current in-flight LLM call or tool execution.
// @Tags conversations
// @Produce json
// @Param convID path string true "Conversation ID"
// @Success 200 {object} map[string]string
// @Failure 410 {object} errorResponse
// @Security BearerAuth
// @Router /v1/conversations/{convID}/abort [post]
func (h *ConversationHandler) Abort(w http.ResponseWriter, r *http.Request) {
	conv, ok := h.loadConversation(w, r)
	if !ok {
		return
	}

	client, ok := h.getBridgeClient(w, r, conv)
	if !ok {
		return
	}

	if err := client.AbortConversation(r.Context(), conv.RuntimeConversationID); err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to abort conversation", "conversation_id", conv.ID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to abort"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "aborted"})
}

// End handles DELETE /v1/conversations/{convID}.
// @Summary End a conversation
// @Description Permanently ends a conversation. Subsequent operations return 410.
// @Tags conversations
// @Produce json
// @Param convID path string true "Conversation ID"
// @Success 200 {object} map[string]string
// @Failure 410 {object} errorResponse
// @Security BearerAuth
// @Router /v1/conversations/{convID} [delete]
func (h *ConversationHandler) End(w http.ResponseWriter, r *http.Request) {
	conv, ok := h.loadConversation(w, r)
	if !ok {
		return
	}

	client, ok := h.getBridgeClient(w, r, conv)
	if !ok {
		return
	}

	if err := client.EndConversation(r.Context(), conv.RuntimeConversationID); err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to end conversation in bridge", "conversation_id", conv.ID, "error", err)

	}

	now := time.Now()
	h.db.Model(conv).Updates(map[string]any{
		"status":   "ended",
		"ended_at": now,
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "ended"})
}

// ListApprovals handles GET /v1/conversations/{convID}/approvals.
// @Summary List pending tool approvals
// @Description Returns pending tool approval requests for the conversation.
// @Tags conversations
// @Produce json
// @Param convID path string true "Conversation ID"
// @Success 200 {array} map[string]interface{}
// @Failure 410 {object} errorResponse
// @Security BearerAuth
// @Router /v1/conversations/{convID}/approvals [get]
func (h *ConversationHandler) ListApprovals(w http.ResponseWriter, r *http.Request) {
	conv, ok := h.loadConversation(w, r)
	if !ok {
		return
	}

	client, ok := h.getBridgeClient(w, r, conv)
	if !ok {
		return
	}

	approvals, err := client.ListApprovals(r.Context(), conv.AgentID.String(), conv.RuntimeConversationID)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to list approvals", "conversation_id", conv.ID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list approvals"})
		return
	}

	writeJSON(w, http.StatusOK, approvals)
}

// ResolveApproval handles POST /v1/conversations/{convID}/approvals/{requestID}.
// @Summary Resolve a tool approval
// @Description Approves or denies a pending tool execution request.
// @Tags conversations
// @Accept json
// @Produce json
// @Param convID path string true "Conversation ID"
// @Param requestID path string true "Approval request ID"
// @Param body body object{decision=string} true "Decision: approve or deny"
// @Success 200 {object} map[string]string
// @Failure 400 {object} errorResponse
// @Failure 410 {object} errorResponse
// @Security BearerAuth
// @Router /v1/conversations/{convID}/approvals/{requestID} [post]
func (h *ConversationHandler) ResolveApproval(w http.ResponseWriter, r *http.Request) {
	conv, ok := h.loadConversation(w, r)
	if !ok {
		return
	}

	requestID := chi.URLParam(r, "requestID")

	var req struct {
		Decision string `json:"decision"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Decision != "approve" && req.Decision != "deny" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "decision must be 'approve' or 'deny'"})
		return
	}

	client, ok := h.getBridgeClient(w, r, conv)
	if !ok {
		return
	}

	decision := bridgepkg.ApprovalDecisionApprove
	if req.Decision == "deny" {
		decision = bridgepkg.ApprovalDecisionDeny
	}
	if err := client.ResolveApproval(r.Context(), conv.AgentID.String(), conv.RuntimeConversationID, requestID, decision); err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to resolve approval", "conversation_id", conv.ID, "request_id", requestID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to resolve approval"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "resolved"})
}
