package hindsight

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/mcpserver"
	"github.com/ziraloop/ziraloop/internal/middleware"
	"github.com/ziraloop/ziraloop/internal/model"
)

// MemoryMCPHandler serves MCP memory tools (recall, retain, reflect) for agents.
// Route: /memory/{agentID}/*
type MemoryMCPHandler struct {
	db     *gorm.DB
	client *Client
	cache  *mcpserver.ServerCache
}

// NewMemoryMCPHandler creates a new memory MCP handler.
func NewMemoryMCPHandler(db *gorm.DB, client *Client) *MemoryMCPHandler {
	return &MemoryMCPHandler{
		db:     db,
		client: client,
		cache:  mcpserver.NewServerCache(),
	}
}

// StreamableHTTPHandler returns an HTTP handler for the MCP Streamable HTTP transport.
func (h *MemoryMCPHandler) StreamableHTTPHandler() http.Handler {
	return mcpsdk.NewStreamableHTTPHandler(h.serverFactory, &mcpsdk.StreamableHTTPOptions{
		Stateless: true,
		Logger:    slog.Default(),
	})
}

// StartCleanup starts the background cache cleanup goroutine.
func (h *MemoryMCPHandler) StartCleanup(ctx context.Context, interval time.Duration) {
	h.cache.StartCleanup(ctx, interval)
}

// serverFactory returns or builds an MCP server for the agent in the request URL.
func (h *MemoryMCPHandler) serverFactory(r *http.Request) *mcpsdk.Server {
	agentID := chi.URLParam(r, "agentID")
	if agentID == "" {
		slog.Error("memory mcp: no agentID in URL")
		return nil
	}

	srv, err := h.cache.GetOrBuild(agentID, func() (*mcpsdk.Server, time.Time, error) {
		// Load agent with identity
		var agent model.Agent
		if err := h.db.Preload("Identity").Where("id = ?", agentID).First(&agent).Error; err != nil {
			return nil, time.Time{}, err
		}

		server := BuildMemoryServer(&agent, agent.Identity, h.client)

		// Cache for 1 hour (agent config rarely changes mid-conversation)
		return server, time.Now().Add(1 * time.Hour), nil
	})
	if err != nil {
		slog.Error("memory mcp: failed to build server", "agent_id", agentID, "error", err)
		return nil
	}

	return srv
}

// ValidateAgentToken is middleware that ensures the proxy token's agent_id
// matches the {agentID} in the URL. Prevents agents from accessing other agents' memory tools.
func (h *MemoryMCPHandler) ValidateAgentToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		urlAgentID := chi.URLParam(r, "agentID")

		claims, ok := middleware.ClaimsFromContext(r.Context())
		if !ok {
			http.Error(w, `{"error":"missing auth claims"}`, http.StatusUnauthorized)
			return
		}

		// Load token to check agent_id in metadata
		var token model.Token
		if err := h.db.Where("jti = ? AND revoked_at IS NULL", claims.JTI).First(&token).Error; err != nil {
			http.Error(w, `{"error":"token not found"}`, http.StatusUnauthorized)
			return
		}

		tokenAgentID, _ := token.Meta["agent_id"].(string)
		if tokenAgentID != urlAgentID {
			http.Error(w, `{"error":"token agent_id does not match URL"}`, http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}
