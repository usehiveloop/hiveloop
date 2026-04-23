package handler

import (
	"gorm.io/gorm"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/tasks"
)

// Delete handles DELETE /v1/agents/{id}.
// @Summary Delete an agent
// @Description Deletes an agent and removes it from Bridge.
// @Tags agents
// @Produce json
// @Param id path string true "Agent ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/{id} [delete]
func (h *AgentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	id := chi.URLParam(r, "id")

	var agent model.Agent
	if err := h.db.Where("id = ? AND org_id = ? AND is_system = false AND deleted_at IS NULL", id, org.ID).First(&agent).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get agent"})
		return
	}

	// Soft-delete: set deleted_at timestamp
	now := time.Now()
	if err := h.db.Model(&agent).Update("deleted_at", &now).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete agent"})
		return
	}

	if err := deleteAgentTriggers(h.db, agent.ID); err != nil {
		slog.Error("failed to clean up agent triggers on delete", "agent_id", agent.ID, "error", err)
	}

	if h.enqueuer != nil {
		task, err := tasks.NewAgentCleanupTask(agent.ID)
		if err != nil {
			slog.Error("failed to create agent cleanup task", "agent_id", agent.ID, "error", err)
		} else if _, err := h.enqueuer.Enqueue(task); err != nil {
			slog.Error("failed to enqueue agent cleanup", "agent_id", agent.ID, "error", err)
		}
	}

	slog.Info("agent soft-deleted", "agent_id", agent.ID, "org_id", org.ID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}