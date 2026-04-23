package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// UpdateOrgFull handles PUT /admin/v1/orgs/{id}.
// @Summary Update organization
// @Description Updates org name, rate_limit, active status, and allowed_origins.
// @Tags admin
// @Accept json
// @Produce json
// @Param id path string true "Org ID"
// @Param body body adminUpdateOrgRequest true "Fields to update"
// @Success 200 {object} adminOrgResponse
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/orgs/{id} [put]
func (h *AdminHandler) UpdateOrgFull(w http.ResponseWriter, r *http.Request) {
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
		Name           *string   `json:"name,omitempty"`
		RateLimit      *int      `json:"rate_limit,omitempty"`
		Active         *bool     `json:"active,omitempty"`
		AllowedOrigins *[]string `json:"allowed_origins,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	updates := map[string]any{}

	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name cannot be empty"})
			return
		}
		// Check uniqueness
		var existing model.Org
		if err := h.db.Where("name = ? AND id != ?", name, org.ID).First(&existing).Error; err == nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "org name already in use"})
			return
		}
		updates["name"] = name
	}
	if req.RateLimit != nil {
		if *req.RateLimit < 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "rate_limit cannot be negative"})
			return
		}
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

	old := map[string]any{"name": org.Name, "rate_limit": org.RateLimit, "active": org.Active, "allowed_origins": org.AllowedOrigins}
	setAuditDiff(r, old, updates)

	if err := h.db.Model(&org).Updates(updates).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update org"})
		return
	}

	h.db.Where("id = ?", id).First(&org)
	slog.Info("admin: org updated", "org_id", id)
	writeJSON(w, http.StatusOK, toAdminOrgResponse(org))
}