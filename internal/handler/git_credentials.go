package handler

import (
	"crypto/subtle"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/hashicorp/golang-lru/v2/expirable"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/crypto"
	"github.com/ziraloop/ziraloop/internal/model"
	"github.com/ziraloop/ziraloop/internal/nango"
)

const (
	gitTokenCacheSize = 100000
	gitTokenCacheTTL  = 30 * time.Minute
)

type gitTokenEntry struct {
	token    string
	cachedAt time.Time
}

// GitCredentialsHandler serves git credential helper requests from sandboxes.
// Sandboxes call this endpoint to get a fresh GitHub token rather than storing
// tokens on disk. Responses follow the git credential helper protocol.
type GitCredentialsHandler struct {
	db     *gorm.DB
	encKey *crypto.SymmetricKey
	nango  *nango.Client
	cache  *expirable.LRU[uuid.UUID, *gitTokenEntry]
}

// NewGitCredentialsHandler creates a git credentials handler with an in-memory
// token cache (30-minute TTL, max 1000 entries).
func NewGitCredentialsHandler(db *gorm.DB, encKey *crypto.SymmetricKey, nangoClient *nango.Client) *GitCredentialsHandler {
	return &GitCredentialsHandler{
		db:     db,
		encKey: encKey,
		nango:  nangoClient,
		cache:  expirable.NewLRU[uuid.UUID, *gitTokenEntry](gitTokenCacheSize, nil, gitTokenCacheTTL),
	}
}

// Handle processes POST /internal/git-credentials/{agentID}.
// Authenticates via the sandbox's Bridge API key, then returns a fresh
// GitHub installation token from Nango in git credential protocol format.
func (h *GitCredentialsHandler) Handle(w http.ResponseWriter, r *http.Request) {
	agentIDStr := chi.URLParam(r, "agentID")
	agentID, err := uuid.Parse(agentIDStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent_id"})
		return
	}

	// Extract bearer token from Authorization header
	bearerToken := extractBearerToken(r)
	if bearerToken == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing authorization"})
		return
	}

	// Find the latest dedicated sandbox for this agent
	var sandbox model.Sandbox
	if err := h.db.
		Where("agent_id = ? AND sandbox_type = 'dedicated'", agentID).
		Order("created_at DESC").
		First(&sandbox).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "no running sandbox for agent"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to look up sandbox"})
		return
	}

	// Verify the bearer token matches the sandbox's Bridge API key
	decryptedKey, err := h.encKey.DecryptString(sandbox.EncryptedBridgeAPIKey)
	if err != nil {
		slog.Error("git-credentials: failed to decrypt bridge api key", "agent_id", agentID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "auth verification failed"})
		return
	}
	if subtle.ConstantTimeCompare([]byte(bearerToken), []byte(decryptedKey)) != 1 {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}

	// Check cache
	if entry, ok := h.cache.Get(agentID); ok {
		writeGitCredentials(w, entry.token)
		return
	}

	// Look up org's github-app connection
	if sandbox.OrgID == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "sandbox has no org"})
		return
	}
	orgID := *sandbox.OrgID

	var conn model.Connection
	err = h.db.
		Joins("JOIN integrations ON integrations.id = connections.integration_id AND integrations.deleted_at IS NULL").
		Where("connections.org_id = ? AND connections.revoked_at IS NULL AND integrations.provider = ?", orgID, "github-app").
		Order("connections.created_at ASC").
		First(&conn).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "no github-app connection for org"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to look up connection"})
		return
	}

	var integration model.Integration
	if err := h.db.Where("id = ?", conn.IntegrationID).First(&integration).Error; err != nil {
		slog.Error("git-credentials: failed to load integration", "integration_id", conn.IntegrationID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load integration"})
		return
	}

	// Fetch fresh token from Nango
	providerConfigKey := fmt.Sprintf("%s_%s", orgID.String(), integration.UniqueKey)
	nangoConn, err := h.nango.GetConnection(r.Context(), conn.NangoConnectionID, providerConfigKey)
	if err != nil {
		slog.Error("git-credentials: failed to fetch from nango",
			"agent_id", agentID,
			"connection_id", conn.ID,
			"error", err,
		)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to fetch github token"})
		return
	}

	creds, ok := nangoConn["credentials"].(map[string]any)
	if !ok {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "no credentials in github-app response"})
		return
	}
	accessToken, ok := creds["access_token"].(string)
	if !ok || accessToken == "" {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "no access_token in github-app credentials"})
		return
	}

	// Cache and respond
	h.cache.Add(agentID, &gitTokenEntry{
		token:    accessToken,
		cachedAt: time.Now(),
	})

	writeGitCredentials(w, accessToken)
}

// writeGitCredentials writes a response in git credential helper protocol format.
func writeGitCredentials(w http.ResponseWriter, token string) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "username=x-access-token\npassword=%s\n", token)
}

// extractBearerToken extracts the token from an "Authorization: Bearer {token}" header.
func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(auth, "Bearer ")
}
