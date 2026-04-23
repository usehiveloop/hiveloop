package handler

import (
	"time"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
)

// ListOrgs handles GET /admin/v1/orgs.
// @Summary List all organizations
// @Description Returns all organizations with optional filters.
// @Tags admin
// @Produce json
// @Param search query string false "Search by name"
// @Param active query string false "Filter by active status (true/false)"
// @Param limit query int false "Page size"
// @Param cursor query string false "Pagination cursor"
// @Success 200 {object} paginatedResponse[adminOrgResponse]
// @Security BearerAuth
// @Router /admin/v1/orgs [get]
func (h *AdminHandler) ListOrgs(w http.ResponseWriter, r *http.Request) {
	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	q := h.db.Model(&model.Org{})

	if search := r.URL.Query().Get("search"); search != "" {
		q = q.Where("name ILIKE ?", "%"+search+"%")
	}
	if r.URL.Query().Get("active") == "true" {
		q = q.Where("active = true")
	} else if r.URL.Query().Get("active") == "false" {
		q = q.Where("active = false")
	}

	q = applyPagination(q, cursor, limit)

	var orgs []model.Org
	if err := q.Find(&orgs).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list orgs"})
		return
	}

	hasMore := len(orgs) > limit
	if hasMore {
		orgs = orgs[:limit]
	}

	resp := make([]adminOrgResponse, len(orgs))
	for i, o := range orgs {
		resp[i] = toAdminOrgResponse(o)
	}

	result := paginatedResponse[adminOrgResponse]{Data: resp, HasMore: hasMore}
	if hasMore {
		last := orgs[len(orgs)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		result.NextCursor = &c
	}
	writeJSON(w, http.StatusOK, result)
}

// GetOrg handles GET /admin/v1/orgs/{id}.
// @Summary Get organization details
// @Description Returns org details with member, credential, agent, and sandbox counts.
// @Tags admin
// @Produce json
// @Param id path string true "Org ID"
// @Success 200 {object} adminOrgDetailResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/orgs/{id} [get]
func (h *AdminHandler) GetOrg(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var org model.Org
	if err := h.db.Where("id = ?", id).First(&org).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "org not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get org"})
		return
	}

	detail := adminOrgDetailResponse{adminOrgResponse: toAdminOrgResponse(org)}
	h.db.Model(&model.OrgMembership{}).Where("org_id = ?", org.ID).Count(&detail.MemberCount)
	h.db.Model(&model.Credential{}).Where("org_id = ? AND revoked_at IS NULL", org.ID).Count(&detail.CredentialCount)
	h.db.Model(&model.Agent{}).Where("org_id = ? AND status = ?", org.ID, "active").Count(&detail.AgentCount)
	h.db.Model(&model.Sandbox{}).Where("org_id = ?", org.ID).Count(&detail.SandboxCount)

	writeJSON(w, http.StatusOK, detail)
}

// UpdateOrg handles PUT /admin/v1/orgs/{id}.
// @Summary Update organization
// @Description Updates org settings (rate_limit, active, allowed_origins).
// @Tags admin
// @Accept json
// @Produce json
// @Param id path string true "Org ID"
// @Success 200 {object} adminOrgResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/orgs/{id} [put]
func (h *AdminHandler) UpdateOrg(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var org model.Org
	if err := h.db.Where("id = ?", id).First(&org).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "org not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get org"})
		return
	}

	var req struct {
		RateLimit      *int      `json:"rate_limit,omitempty"`
		Active         *bool     `json:"active,omitempty"`
		AllowedOrigins *[]string `json:"allowed_origins,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	updates := map[string]any{}
	if req.RateLimit != nil {
		updates["rate_limit"] = *req.RateLimit
	}
	if req.Active != nil {
		updates["active"] = *req.Active
	}
	if req.AllowedOrigins != nil {
		updates["allowed_origins"] = *req.AllowedOrigins
	}

	if len(updates) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no fields to update"})
		return
	}

	oldOrg := map[string]any{"rate_limit": org.RateLimit, "active": org.Active, "allowed_origins": org.AllowedOrigins}
	setAuditDiff(r, oldOrg, updates)

	if err := h.db.Model(&org).Updates(updates).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update org"})
		return
	}

	h.db.Where("id = ?", id).First(&org)
	slog.Info("admin: org updated", "org_id", id)
	writeJSON(w, http.StatusOK, toAdminOrgResponse(org))
}

// DeactivateOrg handles POST /admin/v1/orgs/{id}/deactivate.
// @Summary Deactivate organization
// @Description Deactivates an organization, blocking all API access.
// @Tags admin
// @Produce json
// @Param id path string true "Org ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/orgs/{id}/deactivate [post]
func (h *AdminHandler) DeactivateOrg(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	result := h.db.Model(&model.Org{}).Where("id = ? AND active = true", id).Update("active", false)
	if result.Error != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to deactivate org"})
		return
	}
	if result.RowsAffected == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "org not found or already inactive"})
		return
	}

	slog.Info("admin: org deactivated", "org_id", id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deactivated"})
}

// ActivateOrg handles POST /admin/v1/orgs/{id}/activate.
// @Summary Activate organization
// @Description Reactivates a previously deactivated organization.
// @Tags admin
// @Produce json
// @Param id path string true "Org ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/orgs/{id}/activate [post]
func (h *AdminHandler) ActivateOrg(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	result := h.db.Model(&model.Org{}).Where("id = ? AND active = false", id).Update("active", true)
	if result.Error != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to activate org"})
		return
	}
	if result.RowsAffected == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "org not found or already active"})
		return
	}

	slog.Info("admin: org activated", "org_id", id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "activated"})
}

// ListOrgMembers handles GET /admin/v1/orgs/{id}/members.
// @Summary List organization members
// @Description Returns all members of an organization.
// @Tags admin
// @Produce json
// @Param id path string true "Org ID"
// @Success 200 {object} map[string]any
// @Security BearerAuth
// @Router /admin/v1/orgs/{id}/members [get]
func (h *AdminHandler) ListOrgMembers(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "id")

	var memberships []model.OrgMembership
	if err := h.db.Preload("User").Where("org_id = ?", orgID).Find(&memberships).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list members"})
		return
	}

	resp := make([]adminMembershipResponse, len(memberships))
	for i, m := range memberships {
		resp[i] = adminMembershipResponse{
			ID:        m.ID.String(),
			UserID:    m.UserID.String(),
			OrgID:     m.OrgID.String(),
			Role:      m.Role,
			UserEmail: m.User.Email,
			UserName:  m.User.Name,
			CreatedAt: m.CreatedAt.Format(time.RFC3339),
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": resp})
}

// DeleteOrg handles DELETE /admin/v1/orgs/{id}.
// @Summary Delete organization
// @Description Permanently deletes an organization and cascades to related data.
// @Tags admin
// @Produce json
// @Param id path string true "Org ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/orgs/{id} [delete]
func (h *AdminHandler) DeleteOrg(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	uid, err := uuid.Parse(id)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid org ID"})
		return
	}

	var org model.Org
	if err := h.db.Where("id = ?", uid).First(&org).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "org not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get org"})
		return
	}

	if err := h.db.Delete(&org).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete org"})
		return
	}

	slog.Info("admin: org deleted", "org_id", id, "name", org.Name)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}