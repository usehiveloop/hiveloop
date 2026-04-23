package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
)

// UpdateCredential handles PUT /admin/v1/credentials/{id}.
// @Summary Update a credential
// @Description Updates credential label.
// @Tags admin
// @Accept json
// @Produce json
// @Param id path string true "Credential ID"
// @Param body body adminUpdateCredentialRequest true "Fields to update"
// @Success 200 {object} adminCredentialResponse
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/credentials/{id} [put]
func (h *AdminHandler) UpdateCredential(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var cred model.Credential
	if err := h.db.Where("id = ?", id).First(&cred).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "credential not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get credential"})
		return
	}

	if cred.RevokedAt != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot update a revoked credential"})
		return
	}

	var req struct {
		Label      *string `json:"label,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	updates := map[string]any{}

	if req.Label != nil {
		label := strings.TrimSpace(*req.Label)
		if label == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "label cannot be empty"})
			return
		}
		updates["label"] = label
	}


	if len(updates) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no fields to update"})
		return
	}

	old := map[string]any{"label": cred.Label}
	setAuditDiff(r, old, updates)

	if err := h.db.Model(&cred).Updates(updates).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update credential"})
		return
	}

	h.db.Where("id = ?", id).First(&cred)
	slog.Info("admin: credential updated", "credential_id", id)
	writeJSON(w, http.StatusOK, toAdminCredentialResponse(cred))
}

// UpdateAgent handles PUT /admin/v1/agents/{id}.
// @Summary Update an agent
// @Description Updates agent configuration with full validation.
// @Tags admin
// @Accept json
// @Produce json
// @Param id path string true "Agent ID"
// @Param body body adminUpdateAgentRequest true "Fields to update"
// @Success 200 {object} adminAgentResponse
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/agents/{id} [put]
func (h *AdminHandler) UpdateAgent(w http.ResponseWriter, r *http.Request) {
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

	var req struct {
		Name              *string    `json:"name,omitempty"`
		Description       *string    `json:"description,omitempty"`
		CredentialID      *string    `json:"credential_id,omitempty"`
		SandboxType       *string    `json:"sandbox_type,omitempty"`
		SandboxTemplateID *string    `json:"sandbox_template_id,omitempty"`
		SystemPrompt      *string    `json:"system_prompt,omitempty"`
		Model             *string    `json:"model,omitempty"`
		Tools             model.JSON `json:"tools,omitempty"`
		McpServers        model.JSON `json:"mcp_servers,omitempty"`
		Skills            model.JSON `json:"skills,omitempty"`
		Integrations      model.JSON `json:"integrations,omitempty"`
		AgentConfig       model.JSON `json:"agent_config,omitempty"`
		Permissions       model.JSON `json:"permissions,omitempty"`
		Team              *string    `json:"team,omitempty"`
		SharedMemory      *bool      `json:"shared_memory,omitempty"`
		Status            *string    `json:"status,omitempty"`
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
		updates["name"] = name
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.SystemPrompt != nil {
		updates["system_prompt"] = *req.SystemPrompt
	}
	if req.Model != nil {
		updates["model"] = *req.Model
	}
	if req.Status != nil {
		if *req.Status != "active" && *req.Status != "archived" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "status must be 'active' or 'archived'"})
			return
		}
		updates["status"] = *req.Status
	}

	if req.SandboxType != nil {
		if *req.SandboxType != "dedicated" && *req.SandboxType != "shared" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "sandbox_type must be 'dedicated' or 'shared'"})
			return
		}
		updates["sandbox_type"] = *req.SandboxType
	}

	// Validate credential if changing
	if req.CredentialID != nil {
		credID, err := uuid.Parse(*req.CredentialID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid credential_id format"})
			return
		}
		var cred model.Credential
		if err := h.db.Where("id = ? AND org_id = ? AND revoked_at IS NULL", credID, agent.OrgID).First(&cred).Error; err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "credential not found, not in same org, or revoked"})
			return
		}
		updates["credential_id"] = credID
	}

	// Validate sandbox template if changing
	if req.SandboxTemplateID != nil {
		if *req.SandboxTemplateID == "" {
			updates["sandbox_template_id"] = nil
		} else {
			tmplID, err := uuid.Parse(*req.SandboxTemplateID)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid sandbox_template_id format"})
				return
			}
			var tmpl model.SandboxTemplate
			if err := h.db.Where("id = ? AND org_id = ?", tmplID, agent.OrgID).First(&tmpl).Error; err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "sandbox template not found in the same org"})
				return
			}
			updates["sandbox_template_id"] = tmplID
		}
	}

	if req.Tools != nil {
		updates["tools"] = req.Tools
	}
	if req.McpServers != nil {
		updates["mcp_servers"] = req.McpServers
	}
	if req.Skills != nil {
		updates["skills"] = req.Skills
	}
	if req.Integrations != nil {
		updates["integrations"] = req.Integrations
	}
	if req.AgentConfig != nil {
		if errMsg := validateJSONSchema(req.AgentConfig); errMsg != "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": errMsg})
			return
		}
		updates["agent_config"] = req.AgentConfig
	}
	if req.Permissions != nil {
		updates["permissions"] = req.Permissions
	}
	if req.Team != nil {
		updates["team"] = *req.Team
	}
	if req.SharedMemory != nil {
		updates["shared_memory"] = *req.SharedMemory
	}

	if len(updates) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no fields to update"})
		return
	}

	old := map[string]any{
		"name": agent.Name, "description": agent.Description, "model": agent.Model,
		"system_prompt": agent.SystemPrompt, "sandbox_type": agent.SandboxType, "status": agent.Status,
		"credential_id": agent.CredentialID.String(), "team": agent.Team, "shared_memory": agent.SharedMemory,
	}
	setAuditDiff(r, old, updates)

	if err := h.db.Model(&agent).Updates(updates).Error; err != nil {
		if isDuplicateKeyError(err) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "agent with that name already exists in this workspace"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update agent"})
		return
	}

	h.db.Where("id = ?", id).First(&agent)
	slog.Info("admin: agent updated", "agent_id", id)
	writeJSON(w, http.StatusOK, toAdminAgentResponse(agent))
}