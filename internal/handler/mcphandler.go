package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/llmvault/llmvault/internal/counter"
	mcppkg "github.com/llmvault/llmvault/internal/mcp"
	"github.com/llmvault/llmvault/internal/mcp/catalog"
	"github.com/llmvault/llmvault/internal/mcpserver"
	"github.com/llmvault/llmvault/internal/middleware"
	"github.com/llmvault/llmvault/internal/model"
	"github.com/llmvault/llmvault/internal/nango"
)

// MCPHandler handles MCP protocol requests for scoped proxy tokens.
type MCPHandler struct {
	db          *gorm.DB
	signingKey  []byte
	catalog     *catalog.Catalog
	nango       *nango.Client
	counter     *counter.Counter
	ServerCache *mcpserver.ServerCache
}

// NewMCPHandler creates a new MCP handler.
func NewMCPHandler(db *gorm.DB, signingKey []byte, cat *catalog.Catalog, nangoClient *nango.Client, ctr *counter.Counter) *MCPHandler {
	return &MCPHandler{
		db:          db,
		signingKey:  signingKey,
		catalog:     cat,
		nango:       nangoClient,
		counter:     ctr,
		ServerCache: mcpserver.NewServerCache(),
	}
}

// StreamableHTTPHandler returns an HTTP handler for the MCP Streamable HTTP transport.
func (h *MCPHandler) StreamableHTTPHandler() http.Handler {
	return mcp.NewStreamableHTTPHandler(h.serverFactory, &mcp.StreamableHTTPOptions{
		Stateless: true,
		Logger:    slog.Default(),
	})
}

// SSEHandler returns an HTTP handler for the legacy MCP SSE transport.
func (h *MCPHandler) SSEHandler() http.Handler {
	return mcp.NewSSEHandler(h.serverFactory, nil)
}

// serverFactory returns or builds an MCP server for the given request's token.
func (h *MCPHandler) serverFactory(r *http.Request) *mcp.Server {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		slog.Error("mcp: no claims in context")
		return nil
	}

	srv, err := h.ServerCache.GetOrBuild(claims.JTI, func() (*mcp.Server, time.Time, error) {
		// Load token record with scopes
		var token model.Token
		if err := h.db.Where("jti = ?", claims.JTI).First(&token).Error; err != nil {
			return nil, time.Time{}, err
		}

		// Parse scopes from JSONB
		scopes, err := parseTokenScopes(token.Scopes)
		if err != nil {
			return nil, time.Time{}, err
		}

		// Build MCP server from scopes
		srv, err := mcpserver.BuildServer(&token, scopes, h.catalog, h.nango, h.db, h.counter)
		if err != nil {
			return nil, time.Time{}, err
		}

		return srv, token.ExpiresAt, nil
	})
	if err != nil {
		slog.Error("mcp: failed to build server", "error", err, "jti", claims.JTI)
		return nil
	}

	return srv
}

// ValidateJTIMatch is middleware ensuring the URL {jti} matches the JWT's JTI claim.
func (h *MCPHandler) ValidateJTIMatch(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		urlJTI := chi.URLParam(r, "jti")
		claims, ok := middleware.ClaimsFromContext(r.Context())
		if !ok || urlJTI != claims.JTI {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "token JTI does not match URL"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ValidateHasScopes is middleware ensuring the token has scopes (returns 403 if no scopes).
func (h *MCPHandler) ValidateHasScopes(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := middleware.ClaimsFromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing claims"})
			return
		}

		var token model.Token
		if err := h.db.Where("jti = ?", claims.JTI).First(&token).Error; err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "token not found"})
			return
		}

		scopes, err := parseTokenScopes(token.Scopes)
		if err != nil || len(scopes) == 0 {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "token has no MCP scopes"})
			return
		}

		next.ServeHTTP(w, r)
	})
}

// parseTokenScopes extracts TokenScope slice from the JSONB column.
func parseTokenScopes(scopesJSON model.JSON) ([]mcppkg.TokenScope, error) {
	if scopesJSON == nil {
		return nil, nil
	}

	// The scopes column may be stored as {"scopes": [...]} or directly as [...]
	// Try the wrapper format first
	if scopeArr, ok := scopesJSON["scopes"]; ok {
		raw, err := json.Marshal(scopeArr)
		if err != nil {
			return nil, err
		}
		var scopes []mcppkg.TokenScope
		if err := json.Unmarshal(raw, &scopes); err != nil {
			return nil, err
		}
		return scopes, nil
	}

	return nil, nil
}
