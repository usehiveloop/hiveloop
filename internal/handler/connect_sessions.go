package handler

import (
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/useportal/llmvault/internal/middleware"
	"github.com/useportal/llmvault/internal/model"
	"github.com/useportal/llmvault/internal/registry"
)

// ConnectSessionHandler manages connect session creation.
type ConnectSessionHandler struct {
	db  *gorm.DB
	reg *registry.Registry
}

// NewConnectSessionHandler creates a new connect session handler.
func NewConnectSessionHandler(db *gorm.DB, reg *registry.Registry) *ConnectSessionHandler {
	return &ConnectSessionHandler{db: db, reg: reg}
}

const maxSessionTTL = 30 * time.Minute

type createConnectSessionRequest struct {
	IdentityID       *string    `json:"identity_id,omitempty"`
	ExternalID       *string    `json:"external_id,omitempty"`
	AllowedProviders []string   `json:"allowed_providers,omitempty"`
	Permissions      []string   `json:"permissions,omitempty"`
	AllowedOrigins   []string   `json:"allowed_origins,omitempty"`
	Metadata         model.JSON `json:"metadata,omitempty"`
	TTL              string     `json:"ttl,omitempty"`
}

type connectSessionResponse struct {
	ID               string   `json:"id"`
	SessionToken     string   `json:"session_token"`
	IdentityID       *string  `json:"identity_id,omitempty"`
	ExternalID       string   `json:"external_id,omitempty"`
	AllowedProviders []string `json:"allowed_providers,omitempty"`
	AllowedOrigins   []string `json:"allowed_origins,omitempty"`
	ExpiresAt        string   `json:"expires_at"`
	CreatedAt        string   `json:"created_at"`
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

	// Validate allowed_providers against registry
	for _, pid := range req.AllowedProviders {
		if _, ok := h.reg.GetProvider(pid); !ok {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown provider: " + pid})
			return
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
		AllowedProviders: pq.StringArray(req.AllowedProviders),
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
		AllowedProviders: req.AllowedProviders,
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
