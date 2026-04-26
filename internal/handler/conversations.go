package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/billing"
	"github.com/usehiveloop/hiveloop/internal/enqueue"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/sandbox"
	"github.com/usehiveloop/hiveloop/internal/streaming"
)

// ConversationHandler proxies conversation operations to Bridge.
type ConversationHandler struct {
	db           *gorm.DB
	orchestrator *sandbox.Orchestrator
	pusher       *sandbox.Pusher
	eventBus     *streaming.EventBus // nil = use legacy Bridge SSE proxy
	enqueuer     enqueue.TaskEnqueuer
	credits      *billing.CreditsService // nil disables credit gating (useful in tests)
}

func NewConversationHandler(db *gorm.DB, orchestrator *sandbox.Orchestrator, pusher *sandbox.Pusher, eventBus *streaming.EventBus) *ConversationHandler {
	return &ConversationHandler{db: db, orchestrator: orchestrator, pusher: pusher, eventBus: eventBus}
}

func (h *ConversationHandler) SetEnqueuer(enqueuer enqueue.TaskEnqueuer) {
	h.enqueuer = enqueuer
}

// SetCredits wires the credit ledger. When set, Create deducts one credit per
// conversation and rejects with 402 when the balance is exhausted.
func (h *ConversationHandler) SetCredits(credits *billing.CreditsService) {
	h.credits = credits
}

type createConversationRequest struct {
	ToolNames      []string `json:"tool_names,omitempty"`
	McpServerNames []string `json:"mcp_server_names,omitempty"`
}

type conversationResponse struct {
	ID        string `json:"id"`
	AgentID   string `json:"agent_id"`
	Name      string `json:"name,omitempty"`
	Status    string `json:"status"`
	StreamURL string `json:"stream_url"`
	CreatedAt string `json:"created_at"`
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
// @Description Creates a new conversation for an agent by spinning up a dedicated sandbox.
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

	// Credit gate. One conversation = one credit. Token-based accounting
	// happens downstream at the LLM proxy layer — out of scope here.
	if h.credits != nil {
		if err := h.credits.Spend(org.ID, 1, billing.ReasonAgentRun, "conversation", ""); err != nil {
			if errors.Is(err, billing.ErrInsufficientCredits) {
				writeJSON(w, http.StatusPaymentRequired, map[string]string{
					"error": "insufficient credits — add credits or upgrade your plan",
				})
				return
			}
			slog.Error("credits: spend failed", "org_id", org.ID, "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to check credits"})
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

	sb, err := h.orchestrator.CreateDedicatedSandbox(ctx, &agent)
	if err != nil {
		slog.Error("failed to create dedicated sandbox", "agent_id", agent.ID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to provision sandbox"})
		return
	}
	if err := h.pusher.PushAgentToSandbox(ctx, &agent, sb); err != nil {
		slog.Error("failed to push agent to dedicated sandbox", "agent_id", agent.ID, "sandbox_id", sb.ID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to initialize agent in sandbox"})
		return
	}

	client, err := h.orchestrator.GetBridgeClient(ctx, sb)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to connect to sandbox"})
		return
	}

	bridgeResp, err := client.CreateConversation(ctx, agent.ID.String())
	if err != nil {
		slog.Error("failed to create conversation in bridge", "agent_id", agent.ID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create conversation"})
		return
	}

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
		Name:      conv.Name,
		Status:    "active",
		StreamURL: fmt.Sprintf("/v1/conversations/%s/stream", conv.ID),
		CreatedAt: conv.CreatedAt.Format(time.RFC3339),
	})
}
