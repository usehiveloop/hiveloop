package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"gorm.io/gorm"

	"github.com/llmvault/llmvault/internal/logto"
	"github.com/llmvault/llmvault/internal/middleware"
	"github.com/llmvault/llmvault/internal/model"
)

// OrgHandler manages organization lifecycle operations.
type OrgHandler struct {
	db      *gorm.DB
	logto   *logto.Client
}

// NewOrgHandler creates a new org handler.
func NewOrgHandler(db *gorm.DB, logtoClient *logto.Client) *OrgHandler {
	return &OrgHandler{db: db, logto: logtoClient}
}

type createOrgRequest struct {
	Name string `json:"name"`
}

type orgResponse struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	LogtoOrgID string `json:"logto_org_id"`
	RateLimit  int    `json:"rate_limit"`
	Active     bool   `json:"active"`
	CreatedAt  string `json:"created_at"`
}

// Create handles POST /v1/orgs.
// @Summary Create an organization
// @Description Creates a new organization in Logto and stores a local record.
// @Tags orgs
// @Accept json
// @Produce json
// @Param body body createOrgRequest true "Organization name"
// @Success 201 {object} orgResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/orgs [post]
func (h *OrgHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.LogtoClaimsFromContext(r.Context())
	if !ok || claims.Sub == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	var req createOrgRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}

	// 1. Create org in Logto
	logtoOrgID, err := h.logto.CreateOrganization(req.Name)
	if err != nil {
		slog.Error("failed to create org in Logto", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create organization"})
		return
	}

	// 2. Add the requesting actor as an org member.
	// For user tokens, sub is the user ID and differs from client_id.
	// For M2M tokens, sub == client_id, so we only add as M2M member.
	isUserToken := claims.Sub != "" && claims.Sub != claims.ClientID
	if isUserToken {
		if err := h.logto.AddOrgMember(logtoOrgID, claims.Sub); err != nil {
			slog.Error("failed to add user as org member", "error", err, "org_id", logtoOrgID, "user_id", claims.Sub)
		}
		adminRoleID, err := h.logto.GetOrgRoleByName("admin")
		if err == nil {
			if err := h.logto.AssignOrgRoleToUser(logtoOrgID, claims.Sub, []string{adminRoleID}); err != nil {
				slog.Error("failed to assign admin role to user", "error", err, "org_id", logtoOrgID)
			}
		}
	}
	if claims.ClientID != "" {
		if err := h.logto.AddOrgMemberM2M(logtoOrgID, claims.ClientID); err != nil {
			slog.Error("failed to add M2M app as org member", "error", err, "org_id", logtoOrgID, "client_id", claims.ClientID)
		}
		adminRoleID, err := h.logto.GetOrgRoleByName("m2m:admin")
		if err == nil {
			if err := h.logto.AssignOrgRoleToM2M(logtoOrgID, claims.ClientID, []string{adminRoleID}); err != nil {
				slog.Error("failed to assign admin role to M2M", "error", err, "org_id", logtoOrgID)
			}
		}
	}

	// 3. Create local org record
	org := model.Org{
		Name:       req.Name,
		LogtoOrgID: logtoOrgID,
	}
	if err := h.db.Create(&org).Error; err != nil {
		slog.Error("failed to create local org", "error", err, "logto_org_id", logtoOrgID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create organization record"})
		return
	}

	writeJSON(w, http.StatusCreated, orgResponse{
		ID:         org.ID.String(),
		Name:       org.Name,
		LogtoOrgID: org.LogtoOrgID,
		RateLimit:  org.RateLimit,
		Active:     org.Active,
		CreatedAt:  org.CreatedAt.Format(time.RFC3339),
	})
}

// Current handles GET /v1/orgs/current.
// @Summary Get current organization
// @Description Returns the organization resolved from the request's auth context.
// @Tags orgs
// @Produce json
// @Success 200 {object} orgResponse
// @Failure 403 {object} errorResponse
// @Security BearerAuth
// @Router /v1/orgs/current [get]
func (h *OrgHandler) Current(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "no organization context"})
		return
	}

	writeJSON(w, http.StatusOK, orgResponse{
		ID:         org.ID.String(),
		Name:       org.Name,
		LogtoOrgID: org.LogtoOrgID,
		RateLimit:  org.RateLimit,
		Active:     org.Active,
		CreatedAt:  org.CreatedAt.Format(time.RFC3339),
	})
}
