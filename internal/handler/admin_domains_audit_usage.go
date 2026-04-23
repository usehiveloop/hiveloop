package handler

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
)

// ListCustomDomains handles GET /admin/v1/custom-domains.
// @Summary List all custom domains
// @Description Returns custom domains across all organizations.
// @Tags admin
// @Produce json
// @Param org_id query string false "Filter by org ID"
// @Param verified query string false "Filter by verified status (true/false)"
// @Param limit query int false "Page size"
// @Param cursor query string false "Pagination cursor"
// @Success 200 {object} paginatedResponse[adminCustomDomainResponse]
// @Security BearerAuth
// @Router /admin/v1/custom-domains [get]
func (h *AdminHandler) ListCustomDomains(w http.ResponseWriter, r *http.Request) {
	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	q := h.db.Model(&model.CustomDomain{})
	if orgID := r.URL.Query().Get("org_id"); orgID != "" {
		q = q.Where("org_id = ?", orgID)
	}
	if r.URL.Query().Get("verified") == "true" {
		q = q.Where("verified = true")
	} else if r.URL.Query().Get("verified") == "false" {
		q = q.Where("verified = false")
	}

	q = applyPagination(q, cursor, limit)

	var domains []model.CustomDomain
	if err := q.Find(&domains).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list custom domains"})
		return
	}

	hasMore := len(domains) > limit
	if hasMore {
		domains = domains[:limit]
	}

	resp := make([]adminCustomDomainResponse, len(domains))
	for i, d := range domains {
		resp[i] = toAdminCustomDomainResponse(d)
	}

	result := paginatedResponse[adminCustomDomainResponse]{Data: resp, HasMore: hasMore}
	if hasMore {
		last := domains[len(domains)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		result.NextCursor = &c
	}
	writeJSON(w, http.StatusOK, result)
}

// DeleteCustomDomain handles DELETE /admin/v1/custom-domains/{id}.
// @Summary Delete a custom domain
// @Description Force-deletes a custom domain.
// @Tags admin
// @Produce json
// @Param id path string true "Domain ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/custom-domains/{id} [delete]
func (h *AdminHandler) DeleteCustomDomain(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	result := h.db.Where("id = ?", id).Delete(&model.CustomDomain{})
	if result.Error != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete custom domain"})
		return
	}
	if result.RowsAffected == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "custom domain not found"})
		return
	}

	slog.Info("admin: custom domain deleted", "domain_id", id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
// ListAudit handles GET /admin/v1/audit.
// @Summary List all audit entries
// @Description Returns audit entries across all organizations.
// @Tags admin
// @Produce json
// @Param org_id query string false "Filter by org ID"
// @Param action query string false "Filter by action"
// @Param limit query int false "Page size"
// @Success 200 {object} map[string]any
// @Security BearerAuth
// @Router /admin/v1/audit [get]
func (h *AdminHandler) ListAudit(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := parseInt(l); err == nil && n > 0 {
			if n > 100 {
				n = 100
			}
			limit = n
		}
	}

	q := h.db.Model(&model.AuditEntry{})
	if orgID := r.URL.Query().Get("org_id"); orgID != "" {
		q = q.Where("org_id = ?", orgID)
	}
	if action := r.URL.Query().Get("action"); action != "" {
		q = q.Where("action = ?", action)
	}

	q = q.Order("created_at DESC").Limit(limit + 1)

	var entries []model.AuditEntry
	if err := q.Find(&entries).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list audit entries"})
		return
	}

	hasMore := len(entries) > limit
	if hasMore {
		entries = entries[:limit]
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": entries, "has_more": hasMore})
}
// ListUsage handles GET /admin/v1/usage.
// @Summary Aggregate usage by org
// @Description Returns aggregate request counts grouped by organization.
// @Tags admin
// @Produce json
// @Success 200 {object} map[string]any
// @Security BearerAuth
// @Router /admin/v1/usage [get]
func (h *AdminHandler) ListUsage(w http.ResponseWriter, r *http.Request) {
	var results []struct {
		OrgID        uuid.UUID `json:"org_id"`
		RequestCount int64     `json:"request_count"`
	}

	q := h.db.Model(&model.Usage{}).
		Select("org_id, SUM(request_count) as request_count").
		Group("org_id").Order("request_count DESC").Limit(100)

	if err := q.Scan(&results).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list usage"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": results})
}
// ListWorkspaceStorage handles GET /admin/v1/workspace-storage.
// @Summary List all workspace storage
// @Description Returns all provisioned workspace databases.
// @Tags admin
// @Produce json
// @Success 200 {object} map[string]any
// @Security BearerAuth
// @Router /admin/v1/workspace-storage [get]
func (h *AdminHandler) ListWorkspaceStorage(w http.ResponseWriter, r *http.Request) {
	var storages []model.WorkspaceStorage
	if err := h.db.Order("created_at DESC").Find(&storages).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list workspace storage"})
		return
	}

	resp := make([]adminWorkspaceStorageResponse, len(storages))
	for i, ws := range storages {
		resp[i] = toAdminWorkspaceStorageResponse(ws)
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": resp})
}

// DeleteWorkspaceStorage handles DELETE /admin/v1/workspace-storage/{id}.
// @Summary Delete workspace storage
// @Description Deletes a workspace storage record.
// @Tags admin
// @Produce json
// @Param id path string true "Storage ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/workspace-storage/{id} [delete]
func (h *AdminHandler) DeleteWorkspaceStorage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	result := h.db.Where("id = ?", id).Delete(&model.WorkspaceStorage{})
	if result.Error != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete workspace storage"})
		return
	}
	if result.RowsAffected == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "workspace storage not found"})
		return
	}

	slog.Info("admin: workspace storage deleted", "storage_id", id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}