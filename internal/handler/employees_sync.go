package handler

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
	slackprov "github.com/usehiveloop/hiveloop/internal/profiles/slack"
)

type syncEmployeeResponse struct {
	Applied          int      `json:"applied"`
	Deleted          int      `json:"deleted"`
	ReposCloned      int      `json:"repos_cloned"`
	RestartTriggered bool     `json:"restart_triggered"`
	Errors           []string `json:"errors,omitempty"`
}

// @Summary Push compiled config to an employee sandbox
// @Description Compiles the employee config, provisions an employee sandbox if
// @Description needed, pushes it to the runtime, and verifies readiness.
// @Description Requires the agent be an employee with an active Slack profile.
// @Tags employees
// @Produce json
// @Param id path string true "Agent UUID"
// @Success 200 {object} syncEmployeeResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Failure 502 {object} errorResponse
// @Security BearerAuth
// @Router /v1/employees/{id}/sync [post]
func (h *EmployeeHandler) Sync(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logging.FromContext(ctx)

	org, ok := middleware.OrgFromContext(ctx)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	agentID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent id"})
		return
	}

	var agent model.Agent
	if err := h.db.WithContext(ctx).Where("id = ? AND org_id = ?", agentID, org.ID).First(&agent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "employee not found"})
			return
		}
		log.ErrorContext(ctx, "load employee", "error", err, "agent_id", agentID, "org_id", org.ID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load employee"})
		return
	}

	if !agent.IsEmployee {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "agent is not an employee"})
		return
	}

	var profileCount int64
	err = h.db.WithContext(ctx).Model(&model.AgentProfile{}).
		Where("agent_id = ? AND status = ? AND deleted_at IS NULL AND provider = ?",
			agentID, "active", slackprov.Provider).
		Count(&profileCount).Error
	if err != nil {
		log.ErrorContext(ctx, "count employee profiles", "error", err, "agent_id", agentID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load employee profiles"})
		return
	}
	if profileCount == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "employee must have an active slack profile"})
		return
	}

	var sb model.Sandbox
	err = h.db.WithContext(ctx).
		Where("agent_id = ? AND org_id = ?", agentID, org.ID).
		Order("created_at DESC").Limit(1).First(&sb).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			created, createErr := h.ensureEmployeeSandbox(ctx, &agent)
			if createErr != nil {
				log.ErrorContext(ctx, "provision employee sandbox during sync", "error", createErr, "agent_id", agentID)
				writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to provision employee sandbox"})
				return
			}
			sb = *created
		} else {
			log.ErrorContext(ctx, "load employee sandbox", "error", err, "agent_id", agentID)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load employee sandbox"})
			return
		}
	}

	resp, err := h.runEmployeeSync(ctx, &agent, &sb)
	if err != nil {
		log.ErrorContext(ctx, "sync employee config", "error", err,
			"agent_id", agentID, "sandbox_id", sb.ID)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "sandbox rejected sync"})
		return
	}
	writeJSON(w, http.StatusOK, toSyncResponseDTO(resp))
}
