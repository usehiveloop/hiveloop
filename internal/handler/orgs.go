package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

type OrgHandler struct {
	db *gorm.DB
}

func NewOrgHandler(db *gorm.DB) *OrgHandler {
	return &OrgHandler{db: db}
}

// planFor looks up the full plan by slug. Returns nil if the slug has no
// row in the plans table — caller surfaces that as `plan: null`.
func (h *OrgHandler) planFor(slug string) *planDTO {
	if slug == "" {
		return nil
	}
	var plan model.Plan
	if err := h.db.Where("slug = ?", slug).First(&plan).Error; err != nil {
		return nil
	}
	return planFromModel(plan)
}

func (h *OrgHandler) buildOrgResponse(org model.Org) orgResponse {
	return orgResponse{
		ID:        org.ID.String(),
		Name:      org.Name,
		RateLimit: org.RateLimit,
		Active:    org.Active,
		LogoURL:   org.LogoURL,
		Plan:      h.planFor(org.PlanSlug),
		CreatedAt: org.CreatedAt.Format(time.RFC3339),
	}
}

type createOrgRequest struct {
	Name string `json:"name"`
}

type updateOrgRequest struct {
	Name    *string `json:"name,omitempty"`
	LogoURL *string `json:"logo_url,omitempty"`
}

type orgResponse struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	RateLimit int      `json:"rate_limit"`
	Active    bool     `json:"active"`
	LogoURL   string   `json:"logo_url,omitempty"`
	Plan      *planDTO `json:"plan,omitempty"`
	CreatedAt string   `json:"created_at"`
}

// Create handles POST /v1/orgs.
// @Summary Create an organization
// @Description Creates a new organization and adds the requesting user as an admin member.
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
	claims, ok := middleware.AuthClaimsFromContext(r.Context())
	if !ok || claims.UserID == "" {
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

	var org model.Org
	var membership model.OrgMembership

	err := h.db.Transaction(func(tx *gorm.DB) error {
		org = model.Org{
			Name: req.Name,
		}
		if err := tx.Create(&org).Error; err != nil {
			return fmt.Errorf("creating org: %w", err)
		}

		membership = model.OrgMembership{
			UserID: uuid.MustParse(claims.UserID),
			OrgID:  org.ID,
			Role:   "owner",
		}
		if err := tx.Create(&membership).Error; err != nil {
			return fmt.Errorf("creating membership: %w", err)
		}

		return nil
	})
	if err != nil {
		slog.Error("failed to create org", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create organization"})
		return
	}

	writeJSON(w, http.StatusCreated, h.buildOrgResponse(org))
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

	writeJSON(w, http.StatusOK, h.buildOrgResponse(*org))
}

// Update handles PATCH /v1/orgs/current.
// @Summary Update current organization
// @Description Updates name and/or logo_url on the current organization. Admins
// @Description and owners only. Pass an empty string for logo_url to clear it.
// @Tags orgs
// @Accept json
// @Produce json
// @Param body body updateOrgRequest true "Fields to patch"
// @Success 200 {object} orgResponse
// @Failure 400 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/orgs/current [patch]
func (h *OrgHandler) Update(w http.ResponseWriter, r *http.Request) {
	ctxOrg, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "no organization context"})
		return
	}

	var req updateOrgRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Name == nil && req.LogoURL == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no fields to update"})
		return
	}

	updates := map[string]any{}
	if req.Name != nil {
		trimmed := strings.TrimSpace(*req.Name)
		if trimmed == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name cannot be empty"})
			return
		}
		updates["name"] = trimmed
	}
	if req.LogoURL != nil {
		updates["logo_url"] = strings.TrimSpace(*req.LogoURL)
	}

	if err := h.db.Model(&model.Org{}).Where("id = ?", ctxOrg.ID).Updates(updates).Error; err != nil {
		slog.Error("failed to update org", "org_id", ctxOrg.ID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update organization"})
		return
	}

	var org model.Org
	if err := h.db.First(&org, "id = ?", ctxOrg.ID).Error; err != nil {
		slog.Error("failed to reload org after update", "org_id", ctxOrg.ID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to reload organization"})
		return
	}

	writeJSON(w, http.StatusOK, h.buildOrgResponse(org))
}
