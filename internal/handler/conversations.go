package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	bridgepkg "github.com/usehiveloop/hiveloop/internal/bridge"
	"github.com/usehiveloop/hiveloop/internal/enqueue"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/sandbox"
	"github.com/usehiveloop/hiveloop/internal/streaming"
	"github.com/usehiveloop/hiveloop/internal/tasks"
)

// ConversationHandler proxies conversation operations to Bridge.
type ConversationHandler struct {
	db           *gorm.DB
	orchestrator *sandbox.Orchestrator
	pusher       *sandbox.Pusher
	eventBus     *streaming.EventBus    // nil = use legacy Bridge SSE proxy
	enqueuer     enqueue.TaskEnqueuer
}

// NewConversationHandler creates a conversation handler.
func NewConversationHandler(db *gorm.DB, orchestrator *sandbox.Orchestrator, pusher *sandbox.Pusher, eventBus *streaming.EventBus) *ConversationHandler {
	return &ConversationHandler{db: db, orchestrator: orchestrator, pusher: pusher, eventBus: eventBus}
}

// SetEnqueuer sets the task enqueuer.
func (h *ConversationHandler) SetEnqueuer(enqueuer enqueue.TaskEnqueuer) {
	h.enqueuer = enqueuer
}

type createConversationRequest struct {
	ToolNames        []string `json:"tool_names,omitempty"`
	McpServerNames   []string `json:"mcp_server_names,omitempty"`
}

type conversationResponse struct {
	ID        string  `json:"id"`
	AgentID   string  `json:"agent_id"`
	Name      string  `json:"name,omitempty"`
	Status    string  `json:"status"`
	StreamURL string  `json:"stream_url"`
	CreatedAt string  `json:"created_at"`
}

type conversationEventResponse struct {
	ID                   string          `json:"id"`
	EventID              string          `json:"event_id"`
	EventType            string          `json:"event_type"`
	AgentID              string          `json:"agent_id"`
	BridgeConversationID string          `json:"bridge_conversation_id"`
	Timestamp            string          `json:"timestamp"`
	SequenceNumber       int64           `json:"sequence_number"`
	Data                 json.RawMessage `json:"data"`
	CreatedAt            string          `json:"created_at"`
}

// conversationHistoryResponse is the payload shape for GET /conversations/{id}/history.
// It returns events in chronological order so the frontend can render them
// directly before opening the SSE stream.
type conversationHistoryResponse struct {
	ConversationID string                      `json:"conversation_id"`
	Events         []conversationEventResponse `json:"events"`
	// LastEventID is the event_id of the last event in this page. Clients
	// can pass it as the `Last-Event-ID` header when opening the SSE stream
	// to resume from exactly where history ended.
	LastEventID string `json:"last_event_id,omitempty"`
	HasMore     bool   `json:"has_more"`
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

	// Enforce free plan run limit (100 runs/month)
	if org.BillingPlan == "free" {
		startOfMonth := time.Date(time.Now().Year(), time.Now().Month(), 1, 0, 0, 0, 0, time.UTC)
		var runCount int64
		h.db.Model(&model.AgentConversation{}).
			Where("org_id = ? AND created_at >= ?", org.ID, startOfMonth).
			Count(&runCount)
		if runCount >= 100 {
			writeJSON(w, http.StatusPaymentRequired, map[string]string{
				"error": "free plan limit reached (100 runs/month). Upgrade to Pro for more.",
			})
			return
		}
	}

	agentID := chi.URLParam(r, "agentID")

	// Load agent with associations
	var agent model.Agent
	if err := h.db.Preload("Credential").
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

	// Enqueue billing usage event (best-effort, don't block the response)
	if h.enqueuer != nil {
		usageTask, taskErr := tasks.NewBillingUsageEventTask(org.ID, agent.ID, conv.ID, agent.SandboxType)
		if taskErr == nil {
			if _, enqErr := h.enqueuer.Enqueue(usageTask); enqErr != nil {
				slog.Warn("failed to enqueue billing usage event", "run_id", conv.ID, "error", enqErr)
			}
		}
	}

	slog.Info("conversation created",
		"conversation_id", conv.ID,
		"agent_id", agent.ID,
		"sandbox_id", sb.ID,
		"bridge_conversation_id", bridgeResp.ConversationId,
	)

	writeJSON(w, http.StatusCreated, conversationResponse{
		ID:        conv.ID.String(),
		AgentID:   agent.ID.String(),
		Name:      conv.Name,
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
			Name:      c.Name,
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
		Name:      conv.Name,
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
// @Description Opens a Server-Sent Events stream for real-time agent responses. Defaults to live-only (cursor "$"); clients that want history should hydrate via GET /v1/conversations/{convID}/history first. Resumes from Last-Event-ID when provided.
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

// Stream handler tuning constants.
const (
	// sseWriteDeadline bounds each individual SSE frame write. Slow clients
	// that can't accept a frame within this window are disconnected to
	// prevent them from holding a tap subscriber slot indefinitely.
	sseWriteDeadline = 10 * time.Second

	// sseMaxAge caps the lifetime of a single SSE connection. Browsers'
	// EventSource reconnects automatically with Last-Event-ID, so this is
	// transparent to users but makes deploys and long-running connection
	// tracking much cleaner.
	sseMaxAge = 1 * time.Hour

	// sseAuthRecheckInterval controls how often an active SSE stream
	// re-verifies that the caller still has access to the conversation.
	// The re-check reuses the same DB lookup as the initial auth, so the
	// auth cache's TTL gates the worst-case revocation latency.
	sseAuthRecheckInterval = 60 * time.Second

	// ssePingInterval is the interval at which we emit an SSE keep-alive
	// comment to keep intermediaries from idling the connection out.
	ssePingInterval = 15 * time.Second
)

// streamFromRedis streams events from Redis Streams (multi-subscriber, resumable).
func (h *ConversationHandler) streamFromRedis(w http.ResponseWriter, r *http.Request, conv *model.AgentConversation) {
	// Parse Last-Event-ID for resume support. Default is live-only ("$"):
	// clients should hydrate history via GET /history first, then open the
	// stream. Replaying the entire retained window on every connect is
	// wasteful and inconsistent with the DB (which keeps everything).
	cursor := r.Header.Get("Last-Event-ID")
	if cursor == "" {
		cursor = "$"
	}

	// Set SSE headers.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	rc := http.NewResponseController(w)

	// Tell EventSource to wait 5s before reconnecting on disconnect (default
	// is 3s; a bit of backoff smooths deploy rollouts).
	if err := writeSSEFrame(w, rc, "retry: 5000\n\n"); err != nil {
		return
	}
	// Synthetic "ready" so the frontend can distinguish "connected but no
	// events yet" from "still connecting".
	if err := writeSSEFrame(w, rc, "event: ready\ndata: {}\n\n"); err != nil {
		return
	}

	// Subscribe to the conversation's Redis Stream. The EventBus is shared
	// across all SSE subscribers on this pod via a single per-conversation
	// tap goroutine — see internal/streaming/bus.go.
	streamCtx, cancelStream := context.WithCancel(r.Context())
	defer cancelStream()
	events := h.eventBus.Subscribe(streamCtx, conv.ID.String(), cursor)

	// Keep-alive, auth recheck, and max age timers.
	pingTicker := time.NewTicker(ssePingInterval)
	defer pingTicker.Stop()
	authTicker := time.NewTicker(sseAuthRecheckInterval)
	defer authTicker.Stop()
	maxAge := time.NewTimer(sseMaxAge)
	defer maxAge.Stop()

	convID := conv.ID
	orgID := conv.OrgID

	for {
		select {
		case event, ok := <-events:
			if !ok {
				return // upstream closed (ctx cancelled, tap stopped, or evicted)
			}

			// Strip redundant envelope fields for over-the-wire efficiency.
			// The browser already knows conversation_id (from URL) and
			// agent_id (from conversation metadata); they stay in Redis/DB
			// for the flusher and history endpoint.
			trimmed := trimSSEEnvelope(event.Data)

			frame := fmt.Sprintf("id: %s\nevent: %s\ndata: %s\n\n",
				event.ID, event.EventType, string(trimmed))
			if err := writeSSEFrame(w, rc, frame); err != nil {
				slog.Debug("SSE client disconnected", "conversation_id", convID)
				return
			}

		case <-pingTicker.C:
			if err := writeSSEFrame(w, rc, ": ping\n\n"); err != nil {
				return
			}

		case <-authTicker.C:
			// Re-verify that the caller still owns this conversation.
			// Drops silently if membership/key was revoked since connect.
			if !h.stillAuthorized(r.Context(), convID, orgID) {
				slog.Info("SSE auth recheck failed, closing stream",
					"conversation_id", convID)
				return
			}

		case <-maxAge.C:
			slog.Debug("SSE max age reached, closing for reconnect",
				"conversation_id", convID)
			return

		case <-r.Context().Done():
			return
		}
	}
}

// writeSSEFrame writes a single SSE frame with a bounded write deadline and
// flushes. Any error causes the caller to tear the stream down.
func writeSSEFrame(w http.ResponseWriter, rc *http.ResponseController, frame string) error {
	if err := rc.SetWriteDeadline(time.Now().Add(sseWriteDeadline)); err != nil && err != http.ErrNotSupported {
		return err
	}
	if _, err := w.Write([]byte(frame)); err != nil {
		return err
	}
	return rc.Flush()
}

// trimSSEEnvelope removes fields from the event envelope that the browser
// already knows from context (conversation_id from URL, agent_id from
// conversation metadata). The original envelope is preserved in Redis and
// Postgres for history / debugging. On parse error, returns the original
// bytes unchanged.
func trimSSEEnvelope(data json.RawMessage) json.RawMessage {
	if len(data) == 0 {
		return data
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil {
		return data
	}
	delete(obj, "conversation_id")
	delete(obj, "agent_id")
	out, err := json.Marshal(obj)
	if err != nil {
		return data
	}
	return out
}

// stillAuthorized re-checks that the conversation exists, belongs to the
// given org, and is still active. Returns false on any lookup failure so
// we err on the side of dropping the stream when auth state is uncertain.
func (h *ConversationHandler) stillAuthorized(ctx context.Context, convID uuid.UUID, orgID uuid.UUID) bool {
	var count int64
	if err := h.db.WithContext(ctx).
		Model(&model.AgentConversation{}).
		Where("id = ? AND org_id = ? AND status = ?", convID, orgID, "active").
		Count(&count).Error; err != nil {
		return false
	}
	return count == 1
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
			ID:                   e.ID.String(),
			EventID:              e.EventID,
			EventType:            e.EventType,
			AgentID:              e.AgentID,
			BridgeConversationID: e.BridgeConversationID,
			Timestamp:            e.Timestamp.Format(time.RFC3339),
			SequenceNumber:       e.SequenceNumber,
			Data:                 json.RawMessage(e.Data),
			CreatedAt:            e.CreatedAt.Format(time.RFC3339),
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

// History handles GET /v1/conversations/{convID}/history.
// @Summary Hydrate conversation history
// @Description Returns persisted conversation events in chronological order, intended for hydrating a UI before opening the SSE stream. Paginated via since=<event_id>. Unlike GET /events, this endpoint sorts events ASC by sequence_number so the caller can render in order.
// @Tags conversations
// @Produce json
// @Param convID path string true "Conversation ID"
// @Param since query string false "Return events with sequence_number greater than this event_id"
// @Param limit query int false "Page size (default 200, max 1000)"
// @Success 200 {object} conversationHistoryResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/conversations/{convID}/history [get]
func (h *ConversationHandler) History(w http.ResponseWriter, r *http.Request) {
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

	limit := 200
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconvAtoiPositive(l); err == nil {
			if n > 1000 {
				n = 1000
			}
			limit = n
		}
	}

	q := h.db.Where("conversation_id = ?", conv.ID).Order("sequence_number ASC, created_at ASC").Limit(limit + 1)
	if since := r.URL.Query().Get("since"); since != "" {
		// Find the anchor event's sequence number, then return everything after it.
		var anchor model.ConversationEvent
		if err := h.db.Where("conversation_id = ? AND event_id = ?", conv.ID, since).
			First(&anchor).Error; err == nil {
			q = q.Where("sequence_number > ?", anchor.SequenceNumber)
		}
	}

	var events []model.ConversationEvent
	if err := q.Find(&events).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load history"})
		return
	}

	hasMore := len(events) > limit
	if hasMore {
		events = events[:limit]
	}

	resp := conversationHistoryResponse{
		ConversationID: conv.ID.String(),
		HasMore:        hasMore,
		Events:         make([]conversationEventResponse, len(events)),
	}
	for i, e := range events {
		resp.Events[i] = conversationEventResponse{
			ID:                   e.ID.String(),
			EventID:              e.EventID,
			EventType:            e.EventType,
			AgentID:              e.AgentID,
			BridgeConversationID: e.BridgeConversationID,
			Timestamp:            e.Timestamp.Format(time.RFC3339),
			SequenceNumber:       e.SequenceNumber,
			Data:                 json.RawMessage(e.Data),
			CreatedAt:            e.CreatedAt.Format(time.RFC3339),
		}
	}
	if len(events) > 0 {
		resp.LastEventID = events[len(events)-1].EventID
	}
	writeJSON(w, http.StatusOK, resp)
}

// strconvAtoiPositive parses a positive integer. Returns an error on non-positive values.
func strconvAtoiPositive(s string) (int, error) {
	n := 0
	if len(s) == 0 {
		return 0, fmt.Errorf("empty")
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("not a number")
		}
		n = n*10 + int(c-'0')
	}
	if n <= 0 {
		return 0, fmt.Errorf("not positive")
	}
	return n, nil
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
