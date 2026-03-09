package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/zitadel/zitadel-go/v3/pkg/authorization"
	"gorm.io/gorm"

	"github.com/useportal/llmvault/internal/middleware"
	"github.com/useportal/llmvault/internal/model"
	"github.com/useportal/llmvault/internal/zitadel"
)

// OrgHandler manages organization lifecycle operations.
type OrgHandler struct {
	db        *gorm.DB
	zClient   *zitadel.Client
	projectID string
}

// NewOrgHandler creates a new org handler.
func NewOrgHandler(db *gorm.DB, zClient *zitadel.Client, projectID string) *OrgHandler {
	return &OrgHandler{db: db, zClient: zClient, projectID: projectID}
}

type createOrgRequest struct {
	Name string `json:"name"`
}

type orgResponse struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ZitadelOrgID string `json:"zitadel_org_id"`
	RateLimit    int    `json:"rate_limit"`
	Active       bool   `json:"active"`
	CreatedAt    string `json:"created_at"`
}

// Create handles POST /v1/orgs.
func (h *OrgHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID := authorization.UserID(r.Context())
	if userID == "" {
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

	// 1. Create org in ZITADEL
	zOrgID, err := h.zClient.CreateOrganization(req.Name)
	if err != nil {
		slog.Error("failed to create org in ZITADEL", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create organization"})
		return
	}

	// 2. Grant llmvault project to the new org so its users can receive roles
	if h.projectID != "" {
		if _, err := h.zClient.GrantProjectToOrg(h.projectID, zOrgID, []string{"admin", "viewer"}); err != nil {
			slog.Error("failed to grant project to org", "error", err, "org_id", zOrgID)
		}
	}

	// 3. Add the requesting user as org owner
	if err := h.zClient.AddOrgMember(zOrgID, userID, []string{"ORG_OWNER"}); err != nil {
		slog.Error("failed to add user as org member", "error", err, "org_id", zOrgID, "user_id", userID)
	}

	// 4. Grant user admin role on the project within this org
	if h.projectID != "" {
		if err := h.zClient.GrantProjectRoles(zOrgID, userID, h.projectID, []string{"admin"}); err != nil {
			slog.Error("failed to grant project roles", "error", err, "org_id", zOrgID, "user_id", userID)
		}
	}

	// 5. Create local org record
	org := model.Org{
		Name:         req.Name,
		ZitadelOrgID: zOrgID,
	}
	if err := h.db.Create(&org).Error; err != nil {
		slog.Error("failed to create local org", "error", err, "zitadel_org_id", zOrgID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create organization record"})
		return
	}

	writeJSON(w, http.StatusCreated, orgResponse{
		ID:           org.ID.String(),
		Name:         org.Name,
		ZitadelOrgID: org.ZitadelOrgID,
		RateLimit:    org.RateLimit,
		Active:       org.Active,
		CreatedAt:    org.CreatedAt.Format(time.RFC3339),
	})
}

// Current handles GET /v1/orgs/current.
func (h *OrgHandler) Current(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "no organization context"})
		return
	}

	writeJSON(w, http.StatusOK, orgResponse{
		ID:           org.ID.String(),
		Name:         org.Name,
		ZitadelOrgID: org.ZitadelOrgID,
		RateLimit:    org.RateLimit,
		Active:       org.Active,
		CreatedAt:    org.CreatedAt.Format(time.RFC3339),
	})
}
