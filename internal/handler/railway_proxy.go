package handler

import (
	"bytes"
	"crypto/subtle"
	"fmt"
	"io"
	"log/slog"
	"net/http"
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
	railwayGraphQLURL     = "https://backboard.railway.app/graphql/v2"
	railwayTokenCacheSize = 100000
	railwayTokenCacheTTL  = 30 * time.Minute
	railwayProvider       = "railway"
)

type railwayTokenEntry struct {
	token    string
	cachedAt time.Time
}

// RailwayProxyHandler proxies GraphQL requests to Railway's API with
// credentials fetched from Nango. Agents call this endpoint without auth
// headers — the proxy injects the Bearer token.
type RailwayProxyHandler struct {
	db          *gorm.DB
	encKey      *crypto.SymmetricKey
	nango       *nango.Client
	cache       *expirable.LRU[uuid.UUID, *railwayTokenEntry]
	client      *http.Client
	upstreamURL string
}

// SetRailwayUpstreamURL overrides the Railway GraphQL endpoint (for testing).
func SetRailwayUpstreamURL(handler *RailwayProxyHandler, url string) {
	handler.upstreamURL = url
}

// NewRailwayProxyHandler creates a Railway proxy handler with an in-memory
// token cache (30-minute TTL).
func NewRailwayProxyHandler(db *gorm.DB, encKey *crypto.SymmetricKey, nangoClient *nango.Client) *RailwayProxyHandler {
	return &RailwayProxyHandler{
		db:          db,
		encKey:      encKey,
		nango:       nangoClient,
		cache:       expirable.NewLRU[uuid.UUID, *railwayTokenEntry](railwayTokenCacheSize, nil, railwayTokenCacheTTL),
		client:      &http.Client{Timeout: 30 * time.Second},
		upstreamURL: railwayGraphQLURL,
	}
}

// Handle processes POST /internal/railway-proxy/{agentID}.
// Authenticates via the sandbox's Bridge API key, fetches Railway credentials
// from Nango, and forwards the request body to Railway's GraphQL API.
func (h *RailwayProxyHandler) Handle(w http.ResponseWriter, r *http.Request) {
	agentIDStr := chi.URLParam(r, "agentID")
	agentID, err := uuid.Parse(agentIDStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent_id"})
		return
	}

	bearerToken := extractBearerToken(r)
	if bearerToken == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing authorization"})
		return
	}

	// Load the agent
	var agent model.Agent
	if err := h.db.Where("id = ? AND deleted_at IS NULL", agentID).First(&agent).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to look up agent"})
		return
	}

	// Verify the bearer token matches any sandbox's Bridge API key for this agent
	var sandboxes []model.Sandbox
	if err := h.db.Where("agent_id = ?", agentID).Find(&sandboxes).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to look up sandboxes"})
		return
	}
	authenticated := false
	for _, sb := range sandboxes {
		decryptedKey, err := h.encKey.DecryptString(sb.EncryptedBridgeAPIKey)
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

	// Get Railway token (cached or fresh from Nango)
	railwayToken, err := h.getRailwayToken(w, r, &agent, agentID)
	if err != nil {
		return // error already written to w
	}

	// Forward the request body to Railway's GraphQL API
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read request body"})
		return
	}

	h.proxyToRailway(w, r, body, railwayToken)
}

// getRailwayToken returns a Railway API token for the agent's org, using
// the in-memory cache or fetching fresh from Nango. Cached by org ID so
// all agents in the same org share one token.
func (h *RailwayProxyHandler) getRailwayToken(w http.ResponseWriter, r *http.Request, agent *model.Agent, agentID uuid.UUID) (string, error) {
	if agent.OrgID == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent has no org"})
		return "", fmt.Errorf("no org")
	}
	orgID := *agent.OrgID

	if entry, ok := h.cache.Get(orgID); ok {
		return entry.token, nil
	}

	var conn model.InConnection
	err := h.db.
		Joins("JOIN in_integrations ON in_integrations.id = in_connections.in_integration_id AND in_integrations.deleted_at IS NULL").
		Where("in_connections.org_id = ? AND in_connections.revoked_at IS NULL AND in_integrations.provider = ?", orgID, railwayProvider).
		Order("in_connections.created_at ASC").
		First(&conn).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "no railway connection for org"})
			return "", fmt.Errorf("no connection")
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to look up connection"})
		return "", fmt.Errorf("db error")
	}

	var integration model.InIntegration
	if err := h.db.Where("id = ?", conn.InIntegrationID).First(&integration).Error; err != nil {
		slog.Error("railway-proxy: failed to load integration", "integration_id", conn.InIntegrationID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load integration"})
		return "", fmt.Errorf("integration error")
	}

	providerConfigKey := fmt.Sprintf("%s_%s", orgID.String(), integration.UniqueKey)
	nangoConn, err := h.nango.GetConnection(r.Context(), conn.NangoConnectionID, providerConfigKey)
	if err != nil {
		slog.Error("railway-proxy: failed to fetch from nango",
			"agent_id", agentID,
			"connection_id", conn.ID,
			"error", err,
		)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to fetch railway credentials"})
		return "", fmt.Errorf("nango error")
	}

	creds, ok := nangoConn["credentials"].(map[string]any)
	if !ok {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "no credentials in railway response"})
		return "", fmt.Errorf("no credentials")
	}
	accessToken, ok := creds["access_token"].(string)
	if !ok || accessToken == "" {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "no access_token in railway credentials"})
		return "", fmt.Errorf("no token")
	}

	h.cache.Add(orgID, &railwayTokenEntry{
		token:    accessToken,
		cachedAt: time.Now(),
	})

	return accessToken, nil
}

// proxyToRailway forwards a GraphQL request body to Railway's API with the
// given auth token and streams the response back.
func (h *RailwayProxyHandler) proxyToRailway(w http.ResponseWriter, r *http.Request, body []byte, token string) {
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, h.upstreamURL, bytes.NewReader(body))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to build upstream request"})
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := h.client.Do(req)
	if err != nil {
		slog.Error("railway-proxy: upstream request failed", "error", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "railway api request failed"})
		return
	}
	defer resp.Body.Close()

	for key, vals := range resp.Header {
		for _, val := range vals {
			w.Header().Add(key, val)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}
