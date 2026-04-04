package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	bridgepkg "github.com/ziraloop/ziraloop/internal/bridge"
	"github.com/ziraloop/ziraloop/internal/middleware"
	"github.com/ziraloop/ziraloop/internal/model"
	"github.com/ziraloop/ziraloop/internal/sandbox"
	"github.com/ziraloop/ziraloop/internal/streaming"
)

// ConversationHandler proxies conversation operations to Bridge.
type ConversationHandler struct {
	db           *gorm.DB
	orchestrator *sandbox.Orchestrator
	pusher       *sandbox.Pusher
	eventBus     *streaming.EventBus // nil = use legacy Bridge SSE proxy
}

// NewConversationHandler creates a conversation handler.
func NewConversationHandler(db *gorm.DB, orchestrator *sandbox.Orchestrator, pusher *sandbox.Pusher, eventBus *streaming.EventBus) *ConversationHandler {
	return &ConversationHandler{db: db, orchestrator: orchestrator, pusher: pusher, eventBus: eventBus}
}

type createConversationRequest struct {
	ToolNames        []string `json:"tool_names,omitempty"`
	McpServerNames   []string `json:"mcp_server_names,omitempty"`
}

type conversationResponse struct {
	ID        string  `json:"id"`
	AgentID   string  `json:"agent_id"`
	Status    string  `json:"status"`
	StreamURL string  `json:"stream_url"`
	CreatedAt string  `json:"created_at"`
}

type conversationEventResponse struct {
	ID        string     `json:"id"`
	EventType string     `json:"event_type"`
	Payload   model.JSON `json:"payload"`
	CreatedAt string     `json:"created_at"`
}

// Create handles POST /v1/agents/{agentID}/conversations.
// @Summary Create a conversation
// @Description Creates a new conversation for an agent. For shared agents, reuses the existing sandbox. For dedicated agents, spins up a new sandbox.
// @Tags conversations
// @Produce json
// @Param agentID path string true "Agent ID"
// @Success 201 {object} conversationResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Failure 503 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/{agentID}/conversations [post]
func (h *ConversationHandler) Create(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	agentID := chi.URLParam(r, "agentID")

	// Load agent with associations
	var agent model.Agent
	if err := h.db.Preload("Credential").Preload("Identity").
		Where("id = ? AND org_id = ? AND status = 'active'", agentID, org.ID).First(&agent).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load agent"})
		return
	}

	if h.orchestrator == nil || h.pusher == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sandbox orchestrator not configured"})
		return
	}

	ctx := r.Context()

	// Resolve sandbox based on agent type
	var sb *model.Sandbox
	var err error

	if agent.SandboxType == "shared" {
		// Agent should already have a pool sandbox assigned from PushAgent at creation.
		// If not, reassign now.
		if agent.SandboxID == nil {
			if pushErr := h.pusher.PushAgent(ctx, &agent); pushErr != nil {
				slog.Error("failed to assign pool sandbox", "agent_id", agent.ID, "error", pushErr)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to provision sandbox"})
				return
			}
			// Reload agent to get updated SandboxID
			h.db.Where("id = ?", agent.ID).First(&agent)
		}

		var existing model.Sandbox
		if err := h.db.Where("id = ?", *agent.SandboxID).First(&existing).Error; err != nil {
			slog.Error("failed to load assigned sandbox", "agent_id", agent.ID, "sandbox_id", *agent.SandboxID, "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load sandbox"})
			return
		}
		sb = &existing

		// Wake if stopped
		if sb.Status == "stopped" {
			woken, wakeErr := h.orchestrator.WakeSandbox(ctx, sb)
			if wakeErr != nil {
				slog.Error("failed to wake sandbox", "sandbox_id", sb.ID, "error", wakeErr)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to wake sandbox"})
				return
			}
			sb = woken
		}

		// Ensure agent is pushed to Bridge (idempotent)
		if err := h.pusher.PushAgentToSandbox(ctx, &agent, sb); err != nil {
			slog.Error("failed to push shared agent to sandbox", "agent_id", agent.ID, "sandbox_id", sb.ID, "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to initialize agent in sandbox"})
			return
		}
	} else {
		// Dedicated: create a new sandbox for this conversation
		sb, err = h.orchestrator.CreateDedicatedSandbox(ctx, &agent)
		if err != nil {
			slog.Error("failed to create dedicated sandbox", "agent_id", agent.ID, "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to provision sandbox"})
			return
		}
		// Push agent to the new dedicated sandbox
		if err := h.pusher.PushAgentToSandbox(ctx, &agent, sb); err != nil {
			slog.Error("failed to push agent to dedicated sandbox", "agent_id", agent.ID, "sandbox_id", sb.ID, "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to initialize agent in sandbox"})
			return
		}
	}

	// Get Bridge client
	client, err := h.orchestrator.GetBridgeClient(ctx, sb)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to connect to sandbox"})
		return
	}

	// Create conversation in Bridge
	bridgeResp, err := client.CreateConversation(ctx, agent.ID.String())
	if err != nil {
		slog.Error("failed to create conversation in bridge", "agent_id", agent.ID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create conversation"})
		return
	}

	// Save conversation record
	conv := model.AgentConversation{
		OrgID:                org.ID,
		AgentID:              agent.ID,
		SandboxID:            sb.ID,
		BridgeConversationID: bridgeResp.ConversationId,
		Status:               "active",
	}
	if err := h.db.Create(&conv).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save conversation"})
		return
	}

	// Update sandbox last active
	h.db.Model(sb).Update("last_active_at", time.Now())

	slog.Info("conversation created",
		"conversation_id", conv.ID,
		"agent_id", agent.ID,
		"sandbox_id", sb.ID,
		"bridge_conversation_id", bridgeResp.ConversationId,
	)

	writeJSON(w, http.StatusCreated, conversationResponse{
		ID:        conv.ID.String(),
		AgentID:   agent.ID.String(),
		Status:    "active",
		StreamURL: fmt.Sprintf("/v1/conversations/%s/stream", conv.ID),
		CreatedAt: conv.CreatedAt.Format(time.RFC3339),
	})
}

// List handles GET /v1/agents/{agentID}/conversations.
// @Summary List conversations for an agent
// @Description Returns conversations for the specified agent.
// @Tags conversations
// @Produce json
// @Param agentID path string true "Agent ID"
// @Param status query string false "Filter by status (active, ended, error)"
// @Param limit query int false "Page size"
// @Param cursor query string false "Pagination cursor"
// @Success 200 {object} paginatedResponse[conversationResponse]
// @Security BearerAuth
// @Router /v1/agents/{agentID}/conversations [get]
func (h *ConversationHandler) List(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	agentID := chi.URLParam(r, "agentID")
	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	q := h.db.Where("org_id = ? AND agent_id = ?", org.ID, agentID)
	if status := r.URL.Query().Get("status"); status != "" {
		q = q.Where("status = ?", status)
	}
	q = applyPagination(q, cursor, limit)

	var convs []model.AgentConversation
	if err := q.Find(&convs).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list conversations"})
		return
	}

	hasMore := len(convs) > limit
	if hasMore {
		convs = convs[:limit]
	}

	resp := make([]conversationResponse, len(convs))
	for i, c := range convs {
		resp[i] = conversationResponse{
			ID:        c.ID.String(),
			AgentID:   c.AgentID.String(),
			Status:    c.Status,
			StreamURL: fmt.Sprintf("/v1/conversations/%s/stream", c.ID),
			CreatedAt: c.CreatedAt.Format(time.RFC3339),
		}
	}

	result := paginatedResponse[conversationResponse]{Data: resp, HasMore: hasMore}
	if hasMore {
		last := convs[len(convs)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		result.NextCursor = &c
	}
	writeJSON(w, http.StatusOK, result)
}

// Get handles GET /v1/conversations/{convID}.
// @Summary Get a conversation
// @Description Returns a conversation by ID.
// @Tags conversations
// @Produce json
// @Param convID path string true "Conversation ID"
// @Success 200 {object} conversationResponse
// @Failure 404 {object} errorResponse
// @Failure 410 {object} errorResponse "Conversation has ended"
// @Security BearerAuth
// @Router /v1/conversations/{convID} [get]
func (h *ConversationHandler) Get(w http.ResponseWriter, r *http.Request) {
	conv, ok := h.loadConversation(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, conversationResponse{
		ID:        conv.ID.String(),
		AgentID:   conv.AgentID.String(),
		Status:    conv.Status,
		StreamURL: fmt.Sprintf("/v1/conversations/%s/stream", conv.ID),
		CreatedAt: conv.CreatedAt.Format(time.RFC3339),
	})
}

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

	// Lazy token rotation — refresh if near expiry before sending.
	// Skip for system agent conversations (per-conversation token override TODO).
	if h.pusher != nil && conv.CredentialID == nil && h.pusher.NeedsTokenRotation(conv.AgentID.String()) {
		var agent model.Agent
		if err := h.db.Where("id = ?", conv.AgentID).First(&agent).Error; err == nil {
			if err := h.pusher.RotateAgentToken(r.Context(), &agent, &conv.Sandbox); err != nil {
				slog.Error("failed to rotate agent token", "agent_id", conv.AgentID, "error", err)
				// Non-fatal — try sending with existing token
			}
		}
	}
	// TODO: Add per-conversation token rotation for system agents when Bridge supports it

	client, ok := h.getBridgeClient(w, r, conv)
	if !ok {
		return
	}

	if err := client.SendMessage(r.Context(), conv.BridgeConversationID, req.Content); err != nil {
		slog.Error("failed to send message to bridge", "conversation_id", conv.ID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to send message"})
		return
	}

	h.db.Model(&conv.Sandbox).Update("last_active_at", time.Now())

	writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
}

// Stream handles GET /v1/conversations/{convID}/stream (SSE proxy).
// @Summary Stream conversation events (SSE)
// @Description Opens a Server-Sent Events stream for real-time agent responses. Events include message_start, content_delta, tool_call_start, tool_call_result, message_end, done.
// @Tags conversations
// @Produce text/event-stream
// @Param convID path string true "Conversation ID"
// @Success 200 {string} string "SSE event stream"
// @Failure 410 {object} errorResponse
// @Security BearerAuth
// @Router /v1/conversations/{convID}/stream [get]
func (h *ConversationHandler) Stream(w http.ResponseWriter, r *http.Request) {
	conv, ok := h.loadConversation(w, r)
	if !ok {
		return
	}

	h.streamFromRedis(w, r, conv)
}

// streamFromRedis streams events from Redis Streams (multi-subscriber, resumable).
func (h *ConversationHandler) streamFromRedis(w http.ResponseWriter, r *http.Request, conv *model.AgentConversation) {
	// Parse Last-Event-ID for resume support
	cursor := r.Header.Get("Last-Event-ID")
	if cursor == "" {
		cursor = "0" // replay all available events
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	rc := http.NewResponseController(w)
	h.db.Model(&conv.Sandbox).Update("last_active_at", time.Now())

	// Subscribe to the conversation's Redis Stream
	events := h.eventBus.Subscribe(r.Context(), conv.ID.String(), cursor)

	// Keep-alive ping ticker
	pingTicker := time.NewTicker(15 * time.Second)
	defer pingTicker.Stop()

	for {
		select {
		case event, ok := <-events:
			if !ok {
				return // channel closed
			}

			// Write SSE frame: id, event, data
			frame := fmt.Sprintf("id: %s\nevent: %s\ndata: %s\n\n",
				event.ID, event.EventType, string(event.Data))

			if _, err := w.Write([]byte(frame)); err != nil {
				slog.Debug("SSE client disconnected", "conversation_id", conv.ID)
				return
			}
			if err := rc.Flush(); err != nil {
				return
			}

		case <-pingTicker.C:
			if _, err := w.Write([]byte(": ping\n\n")); err != nil {
				return
			}
			if err := rc.Flush(); err != nil {
				return
			}

		case <-r.Context().Done():
			return
		}
	}
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

	if err := client.AbortConversation(r.Context(), conv.BridgeConversationID); err != nil {
		slog.Error("failed to abort conversation", "conversation_id", conv.ID, "error", err)
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

	if err := client.EndConversation(r.Context(), conv.BridgeConversationID); err != nil {
		slog.Error("failed to end conversation in bridge", "conversation_id", conv.ID, "error", err)
		// Continue to update our DB even if Bridge fails
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

	approvals, err := client.ListApprovals(r.Context(), conv.AgentID.String(), conv.BridgeConversationID)
	if err != nil {
		slog.Error("failed to list approvals", "conversation_id", conv.ID, "error", err)
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
		Decision string `json:"decision"` // "approve" or "deny"
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
	if err := client.ResolveApproval(r.Context(), conv.AgentID.String(), conv.BridgeConversationID, requestID, decision); err != nil {
		slog.Error("failed to resolve approval", "conversation_id", conv.ID, "request_id", requestID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to resolve approval"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "resolved"})
}

// ListEvents handles GET /v1/conversations/{convID}/events.
// @Summary List conversation events
// @Description Returns webhook events persisted for the conversation. Filterable by event type.
// @Tags conversations
// @Produce json
// @Param convID path string true "Conversation ID"
// @Param type query string false "Filter by event type (e.g. MessageReceived, ResponseCompleted)"
// @Param limit query int false "Page size"
// @Param cursor query string false "Pagination cursor"
// @Success 200 {object} paginatedResponse[conversationEventResponse]
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/conversations/{convID}/events [get]
func (h *ConversationHandler) ListEvents(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	convID := chi.URLParam(r, "convID")
	var conv model.AgentConversation
	if err := h.db.Where("id = ? AND org_id = ?", convID, org.ID).First(&conv).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "conversation not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load conversation"})
		return
	}

	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	q := h.db.Where("conversation_id = ?", conv.ID)
	if eventType := r.URL.Query().Get("type"); eventType != "" {
		q = q.Where("event_type = ?", eventType)
	}
	q = applyPagination(q, cursor, limit)

	var events []model.ConversationEvent
	if err := q.Find(&events).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list events"})
		return
	}

	hasMore := len(events) > limit
	if hasMore {
		events = events[:limit]
	}

	resp := make([]conversationEventResponse, len(events))
	for i, e := range events {
		resp[i] = conversationEventResponse{
			ID:        e.ID.String(),
			EventType: e.EventType,
			Payload:   e.Payload,
			CreatedAt: e.CreatedAt.Format(time.RFC3339),
		}
	}

	result := paginatedResponse[conversationEventResponse]{Data: resp, HasMore: hasMore}
	if hasMore {
		last := events[len(events)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		result.NextCursor = &c
	}
	writeJSON(w, http.StatusOK, result)
}

// --- helpers ---

// loadConversation loads and validates a conversation from the URL param + org context.
func (h *ConversationHandler) loadConversation(w http.ResponseWriter, r *http.Request) (*model.AgentConversation, bool) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return nil, false
	}

	convID := chi.URLParam(r, "convID")
	var conv model.AgentConversation
	if err := h.db.Preload("Sandbox").Where("id = ? AND org_id = ?", convID, org.ID).First(&conv).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "conversation not found"})
			return nil, false
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load conversation"})
		return nil, false
	}

	if conv.Status != "active" {
		writeJSON(w, http.StatusGone, map[string]string{"error": "conversation has ended"})
		return nil, false
	}

	return &conv, true
}

// getFlusher extracts http.Flusher from a ResponseWriter, unwrapping middleware wrappers if needed.
func getFlusher(w http.ResponseWriter) (http.Flusher, bool) {
	if f, ok := w.(http.Flusher); ok {
		return f, true
	}
	// Try to unwrap (chi middleware wraps ResponseWriter)
	type unwrapper interface {
		Unwrap() http.ResponseWriter
	}
	if u, ok := w.(unwrapper); ok {
		return getFlusher(u.Unwrap())
	}
	// Go 1.20+ http.ResponseController can flush any writer
	rc := http.NewResponseController(w)
	if rc.Flush() == nil {
		return &responseControllerFlusher{rc: rc}, true
	}
	return nil, false
}

// responseControllerFlusher wraps http.ResponseController as an http.Flusher.
type responseControllerFlusher struct {
	rc *http.ResponseController
}

func (f *responseControllerFlusher) Flush() {
	f.rc.Flush()
}


// getBridgeClient returns a Bridge client for the conversation's sandbox.
func (h *ConversationHandler) getBridgeClient(w http.ResponseWriter, r *http.Request, conv *model.AgentConversation) (*bridgepkg.BridgeClient, bool) {
	client, err := h.orchestrator.GetBridgeClient(r.Context(), &conv.Sandbox)
	if err != nil {
		slog.Error("failed to get bridge client", "conversation_id", conv.ID, "sandbox_id", conv.SandboxID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to connect to sandbox"})
		return nil, false
	}
	return client, true
}
