package handler

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/hermes"
	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
	slackprov "github.com/usehiveloop/hiveloop/internal/profiles/slack"
)

const employeeProfileWhatsapp = "whatsapp"

type syncEmployeeResponse struct {
	Applied          int      `json:"applied"`
	Deleted          int      `json:"deleted"`
	ReposCloned      int      `json:"repos_cloned"`
	RestartTriggered bool     `json:"restart_triggered"`
	Errors           []string `json:"errors,omitempty"`
}

// @Summary Push compiled config to a Hermes employee's sandbox
// @Description Compiles the agent into a SyncRequest and pushes it to the
// @Description sandbox sidecar. Requires the agent be an employee with at
// @Description least one active slack or whatsapp profile.
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
		Where("agent_id = ? AND status = ? AND deleted_at IS NULL AND provider IN ?",
			agentID, "active", []string{slackprov.Provider, employeeProfileWhatsapp}).
		Count(&profileCount).Error
	if err != nil {
		log.ErrorContext(ctx, "count employee profiles", "error", err, "agent_id", agentID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load employee profiles"})
		return
	}
	if profileCount == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "employee must have an active slack or whatsapp profile"})
		return
	}

	var sb model.Sandbox
	err = h.db.WithContext(ctx).
		Where("agent_id = ? AND org_id = ?", agentID, org.ID).
		Order("created_at DESC").Limit(1).First(&sb).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "no sandbox provisioned for employee"})
			return
		}
		log.ErrorContext(ctx, "load employee sandbox", "error", err, "agent_id", agentID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load employee sandbox"})
		return
	}

	apiKey, err := h.compileDeps.EncKey.DecryptString(sb.EncryptedBridgeAPIKey)
	if err != nil {
		log.ErrorContext(ctx, "decrypt sidecar api key", "error", err, "sandbox_id", sb.ID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to read sandbox credentials"})
		return
	}

	syncReq, err := hermes.Compile(ctx, h.compileDeps, &agent)
	if err != nil {
		log.ErrorContext(ctx, "compile employee config", "error", err, "agent_id", agentID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to compile employee config"})
		return
	}

	client, err := hermes.New(sb.BridgeURL, apiKey)
	if err != nil {
		log.ErrorContext(ctx, "init hermes client", "error", err, "sandbox_id", sb.ID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to reach sandbox"})
		return
	}

	resp, err := client.SyncConfig(ctx, *syncReq)
	if err != nil {
		log.ErrorContext(ctx, "sync employee config", "error", err,
			"agent_id", agentID, "sandbox_id", sb.ID)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "sandbox rejected sync"})
		return
	}

	out := syncEmployeeResponse{}
	if resp.Applied != nil {
		out.Applied = *resp.Applied
	}
	if resp.Deleted != nil {
		out.Deleted = *resp.Deleted
	}
	if resp.ReposCloned != nil {
		out.ReposCloned = *resp.ReposCloned
	}
	if resp.RestartTriggered != nil {
		out.RestartTriggered = *resp.RestartTriggered
	}
	if resp.Errors != nil {
		out.Errors = *resp.Errors
	}
	writeJSON(w, http.StatusOK, out)
}
