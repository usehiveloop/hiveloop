package handler

import (
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/llmvault/llmvault/internal/middleware"
	"github.com/llmvault/llmvault/internal/model"
)

// ConnectSessionHandler manages connect session creation.
type ConnectSessionHandler struct {
	db *gorm.DB
}

// NewConnectSessionHandler creates a new connect session handler.
func NewConnectSessionHandler(db *gorm.DB) *ConnectSessionHandler {
	return &ConnectSessionHandler{db: db}
}

const maxSessionTTL = 30 * time.Minute

type createConnectSessionRequest struct {
	IdentityID          *string    `json:"identity_id,omitempty"`
	ExternalID          *string    `json:"external_id,omitempty"`
	AllowedIntegrations []string   `json:"allowed_integrations,omitempty"`
	Permissions         []string   `json:"permissions,omitempty"`
	AllowedOrigins      []string   `json:"allowed_origins,omitempty"`
	Metadata            model.JSON `json:"metadata,omitempty"`
	TTL                 string     `json:"ttl,omitempty"`
}

type connectSessionResponse struct {
	ID                  string   `json:"id"`
	SessionToken        string   `json:"session_token"`
	IdentityID          *string  `json:"identity_id,omitempty"`
	ExternalID          string   `json:"external_id,omitempty"`
	AllowedIntegrations []string `json:"allowed_integrations,omitempty"`
	AllowedOrigins      []string `json:"allowed_origins,omitempty"`
	ExpiresAt           string   `json:"expires_at"`
	CreatedAt           string   `json:"created_at"`
}

type connectSessionListItem struct {
	ID                  string     `json:"id"`
	SessionToken        string     `json:"session_token"`
	IdentityID          *string    `json:"identity_id,omitempty"`
	ExternalID          string     `json:"external_id,omitempty"`
	AllowedIntegrations []string   `json:"allowed_integrations,omitempty"`
	Permissions         []string   `json:"permissions,omitempty"`
	AllowedOrigins      []string   `json:"allowed_origins,omitempty"`
	Metadata            model.JSON `json:"metadata,omitempty"`
	Status              string     `json:"status"`
	ActivatedAt         *string    `json:"activated_at,omitempty"`
	ExpiresAt           string     `json:"expires_at"`
	CreatedAt           string     `json:"created_at"`
}

func sessionStatus(sess model.ConnectSession) string {
	if time.Now().After(sess.ExpiresAt) {
		return "expired"
	}
	if sess.ActivatedAt != nil {
		return "activated"
	}
	return "active"
}

func maskToken(token string) string {
	if len(token) <= 12 {
		return token
	}
	return token[:10] + "..." + token[len(token)-4:]
}

func toSessionListItem(sess model.ConnectSession) connectSessionListItem {
	item := connectSessionListItem{
		ID:                  sess.ID.String(),
		SessionToken:        maskToken(sess.SessionToken),
		ExternalID:          sess.ExternalID,
		AllowedIntegrations: []string(sess.AllowedIntegrations),
		Permissions:         []string(sess.Permissions),
		AllowedOrigins:      []string(sess.AllowedOrigins),
		Metadata:            sess.Metadata,
		Status:              sessionStatus(sess),
		ExpiresAt:           sess.ExpiresAt.Format(time.RFC3339),
		CreatedAt:           sess.CreatedAt.Format(time.RFC3339),
	}
	if sess.IdentityID != nil {
		s := sess.IdentityID.String()
		item.IdentityID = &s
	}
	if sess.ActivatedAt != nil {
		s := sess.ActivatedAt.Format(time.RFC3339)
		item.ActivatedAt = &s
	}
	return item
}

// Create handles POST /v1/connect/sessions.
// @Summary Create a connect session
// @Description Creates a short-lived session for the Connect widget. Requires an identity_id or external_id to link the session to an end-user.
// @Tags connect-sessions
// @Accept json
// @Produce json
// @Param body body createConnectSessionRequest true "Session parameters"
// @Success 201 {object} connectSessionResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/connect/sessions [post]
func (h *ConnectSessionHandler) Create(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	var req createConnectSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	// Parse TTL (default 15m, max 30m)
	ttl := 15 * time.Minute
	if req.TTL != "" {
		parsed, err := time.ParseDuration(req.TTL)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid ttl: must be a valid Go duration (e.g. 15m, 30m)"})
			return
		}
		if parsed <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ttl must be positive"})
			return
		}
		if parsed > maxSessionTTL {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ttl exceeds maximum of 30m"})
			return
		}
		ttl = parsed
	}

	if len(req.AllowedIntegrations) > 0 {
		var integrations []model.Integration
		if err := h.db.Where("org_id = ? AND deleted_at IS NULL", org.ID).Select("unique_key").Find(&integrations).Error; err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to check integrations"})
			return
		}
		integKeys := make(map[string]bool, len(integrations))
		for _, integ := range integrations {
			integKeys[integ.UniqueKey] = true
		}

		for _, key := range req.AllowedIntegrations {
			if !integKeys[key] {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown integration: " + key})
				return
			}
		}
	}

	// Validate allowed_origins format
	for _, origin := range req.AllowedOrigins {
		u, err := url.Parse(origin)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid origin: " + origin + " (must be http(s)://host)"})
			return
		}
	}

	// If org has AllowedOrigins configured, session origins must be a subset
	if len(org.AllowedOrigins) > 0 && len(req.AllowedOrigins) > 0 {
		orgSet := make(map[string]bool, len(org.AllowedOrigins))
		for _, o := range org.AllowedOrigins {
			orgSet[o] = true
		}
		for _, o := range req.AllowedOrigins {
			if !orgSet[o] {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "origin not in org's allowed_origins: " + o})
				return
			}
		}
	}

	// Validate permissions
	validPerms := map[string]bool{"create": true, "list": true, "delete": true, "verify": true}
	for _, p := range req.Permissions {
		if !validPerms[p] {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid permission: " + p})
			return
		}
	}

	// Resolve identity: explicit identity_id or auto-upsert via external_id
	if req.IdentityID == nil && (req.ExternalID == nil || *req.ExternalID == "") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "identity_id or external_id is required"})
		return
	}

	var identityID *uuid.UUID
	var externalID string

	if req.IdentityID != nil {
		id, err := uuid.Parse(*req.IdentityID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid identity_id"})
			return
		}
		var ident model.Identity
		if err := h.db.Where("id = ? AND org_id = ?", id, org.ID).First(&ident).Error; err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "identity not found"})
			return
		}
		identityID = &id
		externalID = ident.ExternalID
	} else if req.ExternalID != nil && *req.ExternalID != "" {
		var ident model.Identity
		err := h.db.Where("external_id = ? AND org_id = ?", *req.ExternalID, org.ID).First(&ident).Error
		if err == gorm.ErrRecordNotFound {
			ident = model.Identity{
				ID:         uuid.New(),
				OrgID:      org.ID,
				ExternalID: *req.ExternalID,
			}
			if err := h.db.Create(&ident).Error; err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create identity"})
				return
			}
		} else if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to resolve identity"})
			return
		}
		identityID = &ident.ID
		externalID = ident.ExternalID
	}

	// Generate session token
	sessionToken, err := model.GenerateSessionToken()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate session token"})
		return
	}

	sess := model.ConnectSession{
		ID:               uuid.New(),
		OrgID:            org.ID,
		IdentityID:       identityID,
		ExternalID:       externalID,
		SessionToken:     sessionToken,
		AllowedIntegrations: pq.StringArray(req.AllowedIntegrations),
		Permissions:      pq.StringArray(req.Permissions),
		AllowedOrigins:   pq.StringArray(req.AllowedOrigins),
		Metadata:         req.Metadata,
		ExpiresAt:        time.Now().Add(ttl),
	}

	if err := h.db.Create(&sess).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create session"})
		return
	}

	resp := connectSessionResponse{
		ID:               sess.ID.String(),
		SessionToken:     sess.SessionToken,
		ExternalID:       sess.ExternalID,
		AllowedIntegrations: req.AllowedIntegrations,
		AllowedOrigins:   req.AllowedOrigins,
		ExpiresAt:        sess.ExpiresAt.Format(time.RFC3339),
		CreatedAt:        sess.CreatedAt.Format(time.RFC3339),
	}
	if identityID != nil {
		s := identityID.String()
		resp.IdentityID = &s
	}

	writeJSON(w, http.StatusCreated, resp)
}

// List handles GET /v1/connect/sessions.
// @Summary List connect sessions
// @Description Returns connect sessions for the current organization with cursor-based pagination.
// @Tags connect-sessions
// @Produce json
// @Param status query string false "Filter by status: active, activated, expired"
// @Param identity_id query string false "Filter by identity ID"
// @Param external_id query string false "Filter by external ID"
// @Param limit query int false "Page size (default 50, max 100)"
// @Param cursor query string false "Pagination cursor from previous response"
// @Success 200 {object} paginatedResponse[connectSessionListItem]
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/connect/sessions [get]
func (h *ConnectSessionHandler) List(w http.ResponseWriter, r *http.Request) {
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

	q := h.db.Where("connect_sessions.org_id = ?", org.ID)

	if status := r.URL.Query().Get("status"); status != "" {
		now := time.Now()
		switch status {
		case "expired":
			q = q.Where("expires_at <= ?", now)
		case "activated":
			q = q.Where("activated_at IS NOT NULL AND expires_at > ?", now)
		case "active":
			q = q.Where("activated_at IS NULL AND expires_at > ?", now)
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid status: must be active, activated, or expired"})
			return
		}
	}

	if identityID := r.URL.Query().Get("identity_id"); identityID != "" {
		q = q.Where("identity_id = ?", identityID)
	}

	if externalID := r.URL.Query().Get("external_id"); externalID != "" {
		q = q.Where("external_id = ?", externalID)
	}

	q = applyPagination(q, cursor, limit)

	var sessions []model.ConnectSession
	if err := q.Find(&sessions).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list sessions"})
		return
	}

	hasMore := len(sessions) > limit
	if hasMore {
		sessions = sessions[:limit]
	}

	resp := make([]connectSessionListItem, len(sessions))
	for i, s := range sessions {
		resp[i] = toSessionListItem(s)
	}

	result := paginatedResponse[connectSessionListItem]{
		Data:    resp,
		HasMore: hasMore,
	}
	if hasMore {
		last := sessions[len(sessions)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		result.NextCursor = &c
	}

	writeJSON(w, http.StatusOK, result)
}

// Get handles GET /v1/connect/sessions/{id}.
// @Summary Get a connect session
// @Description Returns a single connect session by ID.
// @Tags connect-sessions
// @Produce json
// @Param id path string true "Session ID"
// @Success 200 {object} connectSessionListItem
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/connect/sessions/{id} [get]
func (h *ConnectSessionHandler) Get(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	sessID := chi.URLParam(r, "id")
	if sessID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "session id required"})
		return
	}

	var sess model.ConnectSession
	if err := h.db.Where("id = ? AND org_id = ?", sessID, org.ID).First(&sess).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get session"})
		return
	}

	writeJSON(w, http.StatusOK, toSessionListItem(sess))
}

// Delete handles DELETE /v1/connect/sessions/{id}.
// @Summary Delete a connect session
// @Description Deletes a connect session, immediately invalidating it.
// @Tags connect-sessions
// @Produce json
// @Param id path string true "Session ID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/connect/sessions/{id} [delete]
func (h *ConnectSessionHandler) Delete(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	sessID := chi.URLParam(r, "id")
	if sessID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "session id required"})
		return
	}

	result := h.db.Where("id = ? AND org_id = ?", sessID, org.ID).Delete(&model.ConnectSession{})
	if result.Error != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete session"})
		return
	}
	if result.RowsAffected == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
