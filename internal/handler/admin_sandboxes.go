package handler

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
)

// ListSandboxes handles GET /admin/v1/sandboxes.
// @Summary List all sandboxes
// @Description Returns sandboxes across all organizations with resource metrics.
// @Tags admin
// @Produce json
// @Param org_id query string false "Filter by org ID"
// @Param status query string false "Filter by status (running, stopped, error)"
// @Param sandbox_type query string false "Filter by type (shared, dedicated)"
// @Param limit query int false "Page size"
// @Param cursor query string false "Pagination cursor"
// @Success 200 {object} paginatedResponse[adminSandboxResponse]
// @Security BearerAuth
// @Router /admin/v1/sandboxes [get]
func (h *AdminHandler) ListSandboxes(w http.ResponseWriter, r *http.Request) {
	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	q := h.db.Model(&model.Sandbox{})
	if orgID := r.URL.Query().Get("org_id"); orgID != "" {
		q = q.Where("org_id = ?", orgID)
	}
	if status := r.URL.Query().Get("status"); status != "" {
		q = q.Where("status = ?", status)
	}
	if stype := r.URL.Query().Get("sandbox_type"); stype != "" {
		q = q.Where("sandbox_type = ?", stype)
	}

	q = applyPagination(q, cursor, limit)

	var sandboxes []model.Sandbox
	if err := q.Find(&sandboxes).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list sandboxes"})
		return
	}

	hasMore := len(sandboxes) > limit
	if hasMore {
		sandboxes = sandboxes[:limit]
	}

	resp := make([]adminSandboxResponse, len(sandboxes))
	for i, s := range sandboxes {
		resp[i] = toAdminSandboxResponse(s)
	}

	result := paginatedResponse[adminSandboxResponse]{Data: resp, HasMore: hasMore}
	if hasMore {
		last := sandboxes[len(sandboxes)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		result.NextCursor = &c
	}
	writeJSON(w, http.StatusOK, result)
}

// GetSandbox handles GET /admin/v1/sandboxes/{id}.
// @Summary Get sandbox details
// @Description Returns sandbox details with resource metrics.
// @Tags admin
// @Produce json
// @Param id path string true "Sandbox ID"
// @Success 200 {object} adminSandboxResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/sandboxes/{id} [get]
func (h *AdminHandler) GetSandbox(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var sb model.Sandbox
	if err := h.db.Where("id = ?", id).First(&sb).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "sandbox not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get sandbox"})
		return
	}

	writeJSON(w, http.StatusOK, toAdminSandboxResponse(sb))
}

// StopSandbox handles POST /admin/v1/sandboxes/{id}/stop.
// @Summary Stop a sandbox
// @Description Force-stops a running sandbox.
// @Tags admin
// @Produce json
// @Param id path string true "Sandbox ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} errorResponse
// @Failure 503 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/sandboxes/{id}/stop [post]
func (h *AdminHandler) StopSandbox(w http.ResponseWriter, r *http.Request) {
	if h.orchestrator == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sandbox orchestrator not configured"})
		return
	}

	id := chi.URLParam(r, "id")
	var sb model.Sandbox
	if err := h.db.Where("id = ?", id).First(&sb).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "sandbox not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get sandbox"})
		return
	}

	if err := h.orchestrator.StopSandbox(r.Context(), &sb); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to stop sandbox"})
		return
	}

	slog.Info("admin: sandbox stopped", "sandbox_id", id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

// DeleteSandbox handles DELETE /admin/v1/sandboxes/{id}.
// @Summary Delete a sandbox
// @Description Force-deletes a sandbox from the provider and DB.
// @Tags admin
// @Produce json
// @Param id path string true "Sandbox ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} errorResponse
// @Failure 503 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/sandboxes/{id} [delete]
func (h *AdminHandler) DeleteSandbox(w http.ResponseWriter, r *http.Request) {
	if h.orchestrator == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sandbox orchestrator not configured"})
		return
	}

	id := chi.URLParam(r, "id")
	var sb model.Sandbox
	if err := h.db.Where("id = ?", id).First(&sb).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "sandbox not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get sandbox"})
		return
	}

	if err := h.orchestrator.DeleteSandbox(r.Context(), &sb); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete sandbox"})
		return
	}

	slog.Info("admin: sandbox deleted", "sandbox_id", id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// CleanupSandboxes handles POST /admin/v1/sandboxes/cleanup.
// @Summary Bulk cleanup sandboxes
// @Description Deletes all errored and stale stopped sandboxes (stopped > 24h).
// @Tags admin
// @Produce json
// @Success 200 {object} map[string]any
// @Failure 503 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/sandboxes/cleanup [post]
func (h *AdminHandler) CleanupSandboxes(w http.ResponseWriter, r *http.Request) {
	if h.orchestrator == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sandbox orchestrator not configured"})
		return
	}

	// Find all sandboxes in error state or stopped for > 24h
	var sandboxes []model.Sandbox
	cutoff := time.Now().Add(-24 * time.Hour)
	h.db.Where("status = 'error' OR (status = 'stopped' AND updated_at < ?)", cutoff).Find(&sandboxes)

	deleted := 0
	for _, sb := range sandboxes {
		if err := h.orchestrator.DeleteSandbox(r.Context(), &sb); err != nil {
			slog.Warn("admin: cleanup failed for sandbox", "sandbox_id", sb.ID, "error", err)
			continue
		}
		deleted++
	}

	slog.Info("admin: sandbox cleanup", "found", len(sandboxes), "deleted", deleted)
	writeJSON(w, http.StatusOK, map[string]any{
		"found":   len(sandboxes),
		"deleted": deleted,
	})
}