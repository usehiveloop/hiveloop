package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/config"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/sandbox"
	"github.com/usehiveloop/hiveloop/internal/streaming"
	subagents "github.com/usehiveloop/hiveloop/internal/sub-agents"
	"github.com/usehiveloop/hiveloop/internal/token"
)

// SystemConversationHandler handles conversation creation with system agents.
type SystemConversationHandler struct {
	db           *gorm.DB
	orchestrator *sandbox.Orchestrator
	pusher       *sandbox.Pusher
	eventBus     *streaming.EventBus
	signingKey   []byte
	cfg          *config.Config
}

// NewSystemConversationHandler creates a system conversation handler.
func NewSystemConversationHandler(
	db *gorm.DB,
	orchestrator *sandbox.Orchestrator,
	pusher *sandbox.Pusher,
	eventBus *streaming.EventBus,
	signingKey []byte,
	cfg *config.Config,
) *SystemConversationHandler {
	return &SystemConversationHandler{
		db:           db,
		orchestrator: orchestrator,
		pusher:       pusher,
		eventBus:     eventBus,
		signingKey:   signingKey,
		cfg:          cfg,
	}
}

type createSystemConversationRequest struct {
	CredentialID string `json:"credential_id"`
}

// validSystemAgentTypes is an allowlist of recognized system agent type names.
// Keep in sync with the directories under internal/sub-agents/. Unrecognized
// types must be rejected before any DB lookup so attackers cannot enumerate
// internal agent names via 404 vs. 201 response differences.
var validSystemAgentTypes = map[string]bool{
	"browser-tester-expert":   true,
	"code-review-bugs":        true,
	"code-review-duplication": true,
	"code-review-security":    true,
	"code-review-semantics":   true,
	"code-review-verifier":    true,
	"codebase-explorer":       true,
	"codebase-summarizer":     true,
	"critic":                  true,
	"data-analyst":            true,
	"deep-researcher":         true,
	"planner":                 true,
}

// Create handles POST /v1/system-agents/{type}/conversations.
func (h *SystemConversationHandler) Create(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	agentType := chi.URLParam(r, "type")
	if agentType == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "agent type is required"})
		return
	}
	if !validSystemAgentTypes[agentType] {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "system agent not found"})
		return
	}

	var req createSystemConversationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.CredentialID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "credential_id is required"})
		return
	}

	// Load and validate the user's credential
	var cred model.Credential
	if err := h.db.Where("id = ? AND org_id = ? AND revoked_at IS NULL", req.CredentialID, org.ID).First(&cred).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "credential not found or revoked"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to validate credential"})
		return
	}

	// Map credential provider to system agent name
	providerGroup := subagents.MapProviderToGroup(cred.ProviderID, "")
	systemAgentName := fmt.Sprintf("%s-%s", agentType, providerGroup)

	// Load the system agent
	var agent model.Agent
	if err := h.db.Where("name = ? AND is_system = true AND status = 'active'", systemAgentName).First(&agent).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "system agent not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load system agent"})
		return
	}

	ctx := r.Context()

	// Assign a pool sandbox if not already assigned
	if agent.SandboxID == nil {
		if err := h.pusher.PushAgent(ctx, &agent); err != nil {
			slog.Error("failed to assign pool sandbox to system agent", "agent_name", agent.Name, "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to provision sandbox"})
			return
		}
		// Reload to get updated SandboxID
		h.db.Where("id = ?", agent.ID).First(&agent)
	}

	if agent.SandboxID == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to assign sandbox to system agent"})
		return
	}

	// Load the sandbox
	var sb model.Sandbox
	if err := h.db.Where("id = ?", *agent.SandboxID).First(&sb).Error; err != nil {
		slog.Error("failed to load sandbox for system agent", "agent_name", agent.Name, "sandbox_id", *agent.SandboxID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load sandbox"})
		return
	}

	// Wake if stopped
	if sb.Status == "stopped" {
		woken, err := h.orchestrator.WakeSandbox(ctx, &sb)
		if err != nil {
			slog.Error("failed to wake sandbox", "sandbox_id", sb.ID, "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to wake sandbox"})
			return
		}
		sb = *woken
	}

	// Ensure system agent is pushed to Bridge (idempotent)
	if err := h.pusher.PushAgentToSandbox(ctx, &agent, &sb); err != nil {
		slog.Error("failed to push system agent to bridge", "agent_name", agent.Name, "sandbox_id", sb.ID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to initialize agent in sandbox"})
		return
	}

	// Mint proxy token using the user's org and credential
	tokenStr, jti, err := token.Mint(h.signingKey, org.ID.String(), cred.ID.String(), 24*time.Hour)
	if err != nil {
		slog.Error("failed to mint proxy token", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create auth token"})
		return
	}
	proxyToken := "ptok_" + tokenStr

	// Record the proxy token prefix in Meta for audit/observability; the full
	// token is never stored in plaintext (only the JTI, for revocation). See #43
	// for wiring this token through to Bridge as a per-conversation auth override.
	now := time.Now()
	dbToken := model.Token{
		OrgID:        org.ID,
		CredentialID: cred.ID,
		JTI:          jti,
		ExpiresAt:    now.Add(24 * time.Hour),
		Meta: model.JSON{
			"agent_id":     agent.ID.String(),
			"type":         "system_agent_proxy",
			"token_prefix": proxyToken[:min(len(proxyToken), 12)],
		},
	}
	if err := h.db.Create(&dbToken).Error; err != nil {
		slog.Error("failed to store proxy token", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to store auth token"})
		return
	}

	// Get Bridge client
	client, err := h.orchestrator.GetBridgeClient(ctx, &sb)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to connect to sandbox"})
		return
	}

	// TODO(#43): Pass proxyToken as per-conversation auth override once Bridge
	// supports it. Until then the token is persisted via dbToken (JTI linked to
	// the conversation via TokenID) so we retain a revocable audit trail.
	bridgeResp, err := client.CreateConversation(ctx, agent.ID.String())
	if err != nil {
		slog.Error("failed to create conversation in bridge", "agent_name", agent.Name, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create conversation"})
		return
	}

	// Save conversation record
	conv := model.AgentConversation{
		OrgID:                org.ID,
		AgentID:              agent.ID,
		SandboxID:            sb.ID,
		BridgeConversationID: bridgeResp.ConversationId,
		CredentialID:         &cred.ID,
		TokenID:              &dbToken.ID,
		Status:               "active",
	}
	if err := h.db.Create(&conv).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save conversation"})
		return
	}

	// Update sandbox last active
	h.db.Model(&sb).Update("last_active_at", time.Now())

	slog.Info("system agent conversation created",
		"conversation_id", conv.ID,
		"agent_name", agent.Name,
		"org_id", org.ID,
		"sandbox_id", sb.ID,
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
