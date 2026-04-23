package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/cache"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

type APIKeyHandler struct {
	db           *gorm.DB
	keyCache     *cache.APIKeyCache
	cacheManager *cache.Manager
}

// memberAllowedAPIKeyScopes is the narrow set of scopes a non-admin
// organization member (role != "owner"/"admin") may request when minting an
// API key. Admins and owners may request any scope in model.ValidAPIKeyScopes.
//
// "connect" covers the public-facing connect flows and is considered safe for
// non-admin members to delegate to an API key. Sensitive scopes such as
// "credentials", "tokens", "integrations", "agents", and "all" require an
// admin/owner role so a lower-privileged member cannot mint a key that
// exceeds their own permissions.
var memberAllowedAPIKeyScopes = map[string]bool{
	"connect": true,
}

func NewAPIKeyHandler(db *gorm.DB, keyCache *cache.APIKeyCache, cm *cache.Manager) *APIKeyHandler {
	return &APIKeyHandler{db: db, keyCache: keyCache, cacheManager: cm}
}

type createAPIKeyRequest struct {
	Name      string   `json:"name"`
	Scopes    []string `json:"scopes"`
	ExpiresIn *string  `json:"expires_in,omitempty"`
}

type createAPIKeyResponse struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Key       string   `json:"key"`
	KeyPrefix string   `json:"key_prefix"`
	Scopes    []string `json:"scopes"`
	ExpiresAt *string  `json:"expires_at,omitempty"`
	CreatedAt string   `json:"created_at"`
}

type apiKeyResponse struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	KeyPrefix  string   `json:"key_prefix"`
	Scopes     []string `json:"scopes"`
	ExpiresAt  *string  `json:"expires_at,omitempty"`
	LastUsedAt *string  `json:"last_used_at,omitempty"`
	RevokedAt  *string  `json:"revoked_at,omitempty"`
	CreatedAt  string   `json:"created_at"`
}

// Create handles POST /v1/api-keys.
// @Summary Create an API key
// @Description Creates a new API key for the current organization. The plaintext key is returned once.
// @Tags api-keys
// @Accept json
// @Produce json
// @Param body body createAPIKeyRequest true "API key parameters"
// @Success 201 {object} createAPIKeyResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/api-keys [post]
func (h *APIKeyHandler) Create(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	var req createAPIKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}

	if len(req.Scopes) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "at least one scope is required"})
		return
	}

	for _, s := range req.Scopes {
		if !model.ValidAPIKeyScopes[s] {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid scope: " + s})
			return
		}
	}

	if !requesterCanGrantScopes(r, h.db, org.ID, req.Scopes) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "role does not permit requested scopes; admin/owner required"})
		return
	}

	plaintext, hash, prefix, err := model.GenerateAPIKey()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate api key"})
		return
	}

	var expiresAt *time.Time
	if req.ExpiresIn != nil {
		dur, err := time.ParseDuration(*req.ExpiresIn)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid expires_in: must be a valid Go duration (e.g. 720h)"})
			return
		}
		t := time.Now().Add(dur)
		expiresAt = &t
	}

	apiKey := model.APIKey{
		ID:        uuid.New(),
		OrgID:     org.ID,
		Name:      req.Name,
		KeyHash:   hash,
		KeyPrefix: prefix,
		Scopes:    req.Scopes,
		ExpiresAt: expiresAt,
	}

	if err := h.db.Create(&apiKey).Error; err != nil {
		slog.Error("failed to create api key", "error", err, "org_id", org.ID, "name", req.Name)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create api key"})
		return
	}

	slog.Info("api key created", "org_id", org.ID, "key_id", apiKey.ID, "name", req.Name, "scopes", req.Scopes)

	resp := createAPIKeyResponse{
		ID:        apiKey.ID.String(),
		Name:      apiKey.Name,
		Key:       plaintext,
		KeyPrefix: apiKey.KeyPrefix,
		Scopes:    apiKey.Scopes,
		CreatedAt: apiKey.CreatedAt.Format(time.RFC3339),
	}
	if apiKey.ExpiresAt != nil {
		s := apiKey.ExpiresAt.Format(time.RFC3339)
		resp.ExpiresAt = &s
	}

	writeJSON(w, http.StatusCreated, resp)
}

// List handles GET /v1/api-keys.
// @Summary List API keys
// @Description Returns API keys for the current organization with cursor pagination.
// @Tags api-keys
// @Produce json
// @Param limit query int false "Max items per page (1-100, default 50)"
// @Param cursor query string false "Pagination cursor from previous response"
// @Success 200 {object} paginatedResponse[apiKeyResponse]
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/api-keys [get]
func (h *APIKeyHandler) List(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	q := h.db.Where("org_id = ?", org.ID)
	q = applyPagination(q, cursor, limit)

	var keys []model.APIKey
	if err := q.Find(&keys).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list api keys"})
		return
	}

	hasMore := len(keys) > limit
	if hasMore {
		keys = keys[:limit]
	}

	items := make([]apiKeyResponse, len(keys))
	for i, k := range keys {
		items[i] = apiKeyResponse{
			ID:        k.ID.String(),
			Name:      k.Name,
			KeyPrefix: k.KeyPrefix,
			Scopes:    k.Scopes,
			CreatedAt: k.CreatedAt.Format(time.RFC3339),
		}
		if k.ExpiresAt != nil {
			s := k.ExpiresAt.Format(time.RFC3339)
			items[i].ExpiresAt = &s
		}
		if k.LastUsedAt != nil {
			s := k.LastUsedAt.Format(time.RFC3339)
			items[i].LastUsedAt = &s
		}
		if k.RevokedAt != nil {
			s := k.RevokedAt.Format(time.RFC3339)
			items[i].RevokedAt = &s
		}
	}

	resp := paginatedResponse[apiKeyResponse]{
		Data:    items,
		HasMore: hasMore,
	}
	if hasMore {
		last := keys[len(keys)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		resp.NextCursor = &c
	}

	writeJSON(w, http.StatusOK, resp)
}

// Revoke handles DELETE /v1/api-keys/{id}.
// @Summary Revoke an API key
// @Description Soft-deletes an API key by setting its revoked_at timestamp.
// @Tags api-keys
// @Produce json
// @Param id path string true "API Key ID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/api-keys/{id} [delete]
func (h *APIKeyHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	keyID := chi.URLParam(r, "id")
	if keyID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "api key id required"})
		return
	}

	var apiKey model.APIKey
	if err := h.db.Where("id = ? AND org_id = ? AND revoked_at IS NULL", keyID, org.ID).First(&apiKey).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "api key not found or already revoked"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to find api key"})
		return
	}

	now := time.Now()
	if err := h.db.Model(&apiKey).Update("revoked_at", &now).Error; err != nil {
		slog.Error("failed to revoke api key", "error", err, "org_id", org.ID, "key_id", keyID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to revoke api key"})
		return
	}

	h.keyCache.Invalidate(apiKey.KeyHash)

	_ = h.cacheManager.InvalidateAPIKey(r.Context(), apiKey.KeyHash)

	slog.Info("api key revoked", "org_id", org.ID, "key_id", keyID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

// requesterCanGrantScopes enforces role-based restrictions on which scopes a
// caller may assign to a new API key. Owners and admins may assign any valid
// scope. Other members are restricted to memberAllowedAPIKeyScopes.
//
// For API-key-authenticated requests (no JWT claims on the context), the
// minting API key must itself carry the "all" scope to grant elevated scopes.
func requesterCanGrantScopes(r *http.Request, db *gorm.DB, orgID uuid.UUID, requested []string) bool {
	allMemberAllowed := true
	for _, s := range requested {
		if !memberAllowedAPIKeyScopes[s] {
			allMemberAllowed = false
			break
		}
	}
	if allMemberAllowed {
		return true
	}

	if claims, ok := middleware.AuthClaimsFromContext(r.Context()); ok && claims != nil && claims.UserID != "" {
		var m model.OrgMembership
		userUUID, err := uuid.Parse(claims.UserID)
		if err != nil {
			return false
		}
		if err := db.Where("user_id = ? AND org_id = ?", userUUID, orgID).First(&m).Error; err != nil {
			return false
		}
		return m.Role == "owner" || m.Role == "admin"
	}

	if apiClaims, ok := middleware.APIKeyClaimsFromContext(r.Context()); ok && apiClaims != nil {
		for _, s := range apiClaims.Scopes {
			if s == "all" {
				return true
			}
		}
	}
	return false
}
