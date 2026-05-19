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
// @Description Requires the org to have an active Slack connection.
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

	if upgrade, ok, err := activeEmployeeSandboxUpgrade(ctx, h.db, org.ID, agentID); err != nil {
		log.ErrorContext(ctx, "load active employee sandbox upgrade", "error", err, "agent_id", agentID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load active upgrade"})
		return
	} else if ok {
		writeEmployeeUpgradeConflict(w, upgrade)
		return
	}

	hasProfile, err := h.orgHasActiveSlackConnection(ctx, org.ID)
	if err != nil {
		log.ErrorContext(ctx, "count Slack org connections", "error", err, "agent_id", agentID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load Slack connection"})
		return
	}
	if !hasProfile {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "organization must have an active Slack connection"})
		return
	}

	if _, err := h.ensureEmployeeAgentTemplates(ctx, &agent); err != nil {
		log.ErrorContext(ctx, "ensure employee agent templates", "error", err, "agent_id", agentID, "org_id", org.ID)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "failed to ensure employee agent templates"})
		return
	}

	sb, err := h.ensureEmployeeSandbox(ctx, &agent)
	if err != nil {
		log.ErrorContext(ctx, "provision employee sandbox during sync", "error", err, "agent_id", agentID)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to provision employee sandbox"})
		return
	}

	resp, err := h.runEmployeeSync(ctx, &agent, sb)
	if err != nil {
		log.ErrorContext(ctx, "sync employee config", "error", err,
			"agent_id", agentID, "sandbox_id", sb.ID)
		logging.Capture(ctx, err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "sandbox rejected sync"})
		return
	}
	writeJSON(w, http.StatusOK, toSyncResponseDTO(resp))
}
