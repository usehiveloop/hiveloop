package handler

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/hashicorp/golang-lru/v2/expirable"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/nango"
)

const (
	gitTokenCacheSize = 100000
	gitTokenCacheTTL  = 30 * time.Minute
)

type gitTokenCacheKey struct {
	agentID           uuid.UUID
	providerConfigKey string
	nangoConnectionID string
}

type gitTokenEntry struct {
	token    string
	cachedAt time.Time
}

type gitHubTokenConnection struct {
	conn              model.Connection
	providerConfigKey string
	cacheKey          gitTokenCacheKey
}

// GitCredentialsHandler serves git credential helper requests from sandboxes.
// Sandboxes call this endpoint to get a fresh GitHub token rather than storing
// tokens on disk. Responses follow the git credential helper protocol.
type GitCredentialsHandler struct {
	db     *gorm.DB
	encKey *crypto.SymmetricKey
	nango  *nango.Client
	cache  *expirable.LRU[gitTokenCacheKey, *gitTokenEntry]
}

// NewGitCredentialsHandler creates a git credentials handler with an in-memory
// token cache (30-minute TTL, max 1000 entries).
func NewGitCredentialsHandler(db *gorm.DB, encKey *crypto.SymmetricKey, nangoClient *nango.Client) *GitCredentialsHandler {
	return &GitCredentialsHandler{
		db:     db,
		encKey: encKey,
		nango:  nangoClient,
		cache:  expirable.NewLRU[gitTokenCacheKey, *gitTokenEntry](gitTokenCacheSize, nil, gitTokenCacheTTL),
	}
}

// Handle processes POST /internal/git-credentials/{employeeID}.
// Authenticates via the sandbox's runtime secret, then returns a fresh
// GitHub installation token from Nango in git credential protocol format.
func (h *GitCredentialsHandler) Handle(w http.ResponseWriter, r *http.Request) {
	agentIDStr := chi.URLParam(r, "employeeID")
	agentID, err := uuid.Parse(agentIDStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid employee_id"})
		return
	}

	bearerToken := extractBearerToken(r)
	if bearerToken == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing authorization"})
		return
	}

	var agent model.Employee
	if err := h.db.Where("id = ?", agentID).First(&agent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to look up agent"})
		return
	}

	var sandboxes []model.Sandbox
	if err := h.db.Where("employee_id = ?", agentID).Find(&sandboxes).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to look up sandboxes"})
		return
	}
	authenticated := false
	for _, sb := range sandboxes {
		decryptedKey, err := h.encKey.DecryptString(sb.EncryptedRuntimeSecret)
		if err != nil {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(bearerToken), []byte(decryptedKey)) == 1 {
			authenticated = true
			break
		}
	}
	if !authenticated {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}

	if agent.OrgID == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent has no org"})
		return
	}
	orgID := *agent.OrgID

	tokenConn, err := h.resolveGitHubTokenConnection(r.Context(), orgID, agent)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "no github connection for org"})
			return
		}
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "git-credentials: failed to resolve github connection",
			"employee_id", agentID,
			"error", err,
		)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to look up connection"})
		return
	}

	if entry, ok := h.cache.Get(tokenConn.cacheKey); ok {
		writeGitCredentials(w, entry.token)
		return
	}

	nangoConn, err := h.nango.GetConnection(r.Context(), tokenConn.conn.NangoConnectionID, tokenConn.providerConfigKey)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "git-credentials: failed to fetch from nango",
			"employee_id", agentID,
			"connection_id", tokenConn.conn.ID,
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

	h.cache.Add(tokenConn.cacheKey, &gitTokenEntry{
		token:    accessToken,
		cachedAt: time.Now(),
	})

	writeGitCredentials(w, accessToken)
}

func (h *GitCredentialsHandler) resolveGitHubTokenConnection(ctx context.Context, orgID uuid.UUID, agent model.Employee) (gitHubTokenConnection, error) {
	var conn model.Connection
	if err := h.db.WithContext(ctx).
		Preload("Integration").
		Joins("JOIN integrations ON integrations.id = connections.integration_id AND integrations.deleted_at IS NULL").
		Where("connections.org_id = ? AND connections.revoked_at IS NULL AND integrations.provider LIKE ?", orgID, "github%").
		Order("connections.created_at ASC").
		First(&conn).Error; err != nil {
		return gitHubTokenConnection{}, err
	}
	providerConfigKey := nangoProviderConfigKey(conn.Integration.UniqueKey)
	return gitHubTokenConnection{
		conn:              conn,
		providerConfigKey: providerConfigKey,
		cacheKey:          gitTokenCacheKey{agentID: agent.ID, providerConfigKey: providerConfigKey, nangoConnectionID: conn.NangoConnectionID},
	}, nil
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
