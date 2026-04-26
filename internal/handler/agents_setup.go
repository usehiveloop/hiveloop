package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/lib/pq"

	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// GetSetup handles GET /v1/agents/{id}/setup.
// @Summary Get agent sandbox setup config
// @Description Returns setup commands and env var key names for dedicated agents.
// @Tags agents
// @Produce json
// @Param id path string true "Agent ID"
// @Success 200 {object} setupResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/{id}/setup [get]
func (h *AgentHandler) GetSetup(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	var agent model.Agent
	if err := h.db.Where("id = ? AND org_id = ? AND is_system = false AND deleted_at IS NULL", chi.URLParam(r, "id"), org.ID).First(&agent).Error; err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
		return
	}

	resp := setupResponse{
		SetupCommands: []string(agent.SetupCommands),
		EnvVarKeys:    []string{},
	}
	if resp.SetupCommands == nil {
		resp.SetupCommands = []string{}
	}

	if h.encKey != nil && len(agent.EncryptedEnvVars) > 0 {
		if decrypted, err := h.encKey.DecryptString(agent.EncryptedEnvVars); err == nil {
			var envMap map[string]string
			if json.Unmarshal([]byte(decrypted), &envMap) == nil {
				for k := range envMap {
					resp.EnvVarKeys = append(resp.EnvVarKeys, k)
				}
			}
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// UpdateSetup handles PUT /v1/agents/{id}/setup.
// @Summary Update agent sandbox setup config
// @Description Sets setup commands and encrypted environment variables. Only available for dedicated sandbox agents.
// @Tags agents
// @Accept json
// @Produce json
// @Param id path string true "Agent ID"
// @Param body body setupRequest true "Setup configuration"
// @Success 200 {object} setupResponse
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/{id}/setup [put]
func (h *AgentHandler) UpdateSetup(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	var agent model.Agent
	if err := h.db.Where("id = ? AND org_id = ? AND is_system = false AND deleted_at IS NULL", chi.URLParam(r, "id"), org.ID).First(&agent).Error; err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
		return
	}

	var req setupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	for k := range req.EnvVars {
		if strings.HasPrefix(strings.ToUpper(k), "BRIDGE_") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "environment variable names starting with BRIDGE_ are reserved"})
			return
		}
	}

	updates := map[string]any{}

	if req.SetupCommands != nil {
		updates["setup_commands"] = pq.StringArray(req.SetupCommands)
	}

	if req.EnvVars != nil && h.encKey != nil {
		envJSON, err := json.Marshal(req.EnvVars)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid env_vars"})
			return
		}
		encrypted, err := h.encKey.EncryptString(string(envJSON))
		if err != nil {
			slog.Error("failed to encrypt env vars", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to encrypt environment variables"})
			return
		}
		updates["encrypted_env_vars"] = encrypted
	}

	if len(updates) > 0 {
		if err := h.db.Model(&agent).Updates(updates).Error; err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update setup"})
			return
		}
	}

	resp := setupResponse{
		SetupCommands: req.SetupCommands,
		EnvVarKeys:    []string{},
	}
	if resp.SetupCommands == nil {
		resp.SetupCommands = []string{}
	}
	for k := range req.EnvVars {
		resp.EnvVarKeys = append(resp.EnvVarKeys, k)
	}

	writeJSON(w, http.StatusOK, resp)
}