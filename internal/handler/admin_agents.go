package handler

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
)

// ListAgents handles GET /admin/v1/agents.
// @Summary List all agents
// @Description Returns agents across all organizations.
// @Tags admin
// @Produce json
// @Param org_id query string false "Filter by org ID"
// @Param status query string false "Filter by status (active, archived)"
// @Param limit query int false "Page size"
// @Param cursor query string false "Pagination cursor"
// @Success 200 {object} paginatedResponse[adminAgentResponse]
// @Security BearerAuth
// @Router /admin/v1/agents [get]
func (h *AdminHandler) ListAgents(w http.ResponseWriter, r *http.Request) {
	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	q := h.db.Model(&model.Agent{})
	if orgID := r.URL.Query().Get("org_id"); orgID != "" {
		q = q.Where("org_id = ?", orgID)
	}
	if status := r.URL.Query().Get("status"); status != "" {
		q = q.Where("status = ?", status)
	}

	q = applyPagination(q, cursor, limit)

	var agents []model.Agent
	if err := q.Find(&agents).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list agents"})
		return
	}

	hasMore := len(agents) > limit
	if hasMore {
		agents = agents[:limit]
	}

	resp := make([]adminAgentResponse, len(agents))
	for i, a := range agents {
		resp[i] = toAdminAgentResponse(a)
	}

	result := paginatedResponse[adminAgentResponse]{Data: resp, HasMore: hasMore}
	if hasMore {
		last := agents[len(agents)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		result.NextCursor = &c
	}
	writeJSON(w, http.StatusOK, result)
}

// GetAgent handles GET /admin/v1/agents/{id}.
// @Summary Get agent details
// @Description Returns agent details.
// @Tags admin
// @Produce json
// @Param id path string true "Agent ID"
// @Success 200 {object} adminAgentResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/agents/{id} [get]
func (h *AdminHandler) GetAgent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var agent model.Agent
	if err := h.db.Where("id = ?", id).First(&agent).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get agent"})
		return
	}

	writeJSON(w, http.StatusOK, toAdminAgentResponse(agent))
}

// ArchiveAgent handles POST /admin/v1/agents/{id}/archive.
// @Summary Archive an agent
// @Description Force-archives an active agent.
// @Tags admin
// @Produce json
// @Param id path string true "Agent ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/agents/{id}/archive [post]
func (h *AdminHandler) ArchiveAgent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	result := h.db.Model(&model.Agent{}).Where("id = ? AND status = ?", id, "active").Update("status", "archived")
	if result.Error != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to archive agent"})
		return
	}
	if result.RowsAffected == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found or already archived"})
		return
	}

	slog.Info("admin: agent archived", "agent_id", id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "archived"})
}

// DeleteAgent handles DELETE /admin/v1/agents/{id}.
// @Summary Delete an agent
// @Description Permanently deletes an agent.
// @Tags admin
// @Produce json
// @Param id path string true "Agent ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/agents/{id} [delete]
func (h *AdminHandler) DeleteAgent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var agent model.Agent
	if err := h.db.Where("id = ?", id).First(&agent).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get agent"})
		return
	}

	if err := h.db.Delete(&agent).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete agent"})
		return
	}

	slog.Info("admin: agent deleted", "agent_id", id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}