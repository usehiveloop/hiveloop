package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// Update handles PUT /v1/agents/{id}.
// @Summary Update an agent
// @Description Updates an agent. Re-validates credential/model compatibility.
// @Tags agents
// @Accept json
// @Produce json
// @Param id path string true "Agent ID"
// @Param body body updateAgentRequest true "Fields to update"
// @Success 200 {object} agentResponse
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/{id} [put]
func (h *AgentHandler) Update(w http.ResponseWriter, r *http.Request) {
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

	var req updateAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	updates := map[string]any{}

	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.AvatarURL != nil {
		if *req.AvatarURL == "" {
			updates["avatar_url"] = nil
		} else {
			updates["avatar_url"] = *req.AvatarURL
		}
	}
	if req.Category != nil {
		if *req.Category == "" {
			updates["category"] = nil
		} else {
			if !isValidAgentCategory(*req.Category) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("invalid category %q", *req.Category)})
				return
			}
			updates["category"] = *req.Category
		}
	}
	if req.SystemPrompt != nil {
		updates["system_prompt"] = *req.SystemPrompt
	}
	if len(req.ProviderPrompts) > 0 {
		if errMsg := req.ProviderPrompts.Validate(); errMsg != "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": errMsg})
			return
		}
		updates["provider_prompts"] = req.ProviderPrompts
	}
	if req.Instructions != nil {
		updates["instructions"] = *req.Instructions
	}

	// If credential or model changes, re-validate compatibility
	credID := agent.CredentialID
	modelName := agent.Model
	if req.CredentialID != nil {
		var cred model.Credential
		if err := h.db.Where("id = ? AND org_id = ? AND revoked_at IS NULL", *req.CredentialID, org.ID).First(&cred).Error; err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "credential not found or revoked"})
			return
		}
		credID = &cred.ID
		updates["credential_id"] = cred.ID
	}
	if req.Model != nil {
		modelName = *req.Model
		updates["model"] = *req.Model
	}

	if (req.CredentialID != nil || req.Model != nil) && credID != nil {
		var cred model.Credential
		h.db.Where("id = ?", *credID).First(&cred)
		if cred.ProviderID != "" {
			provider, ok := h.registry.GetProvider(cred.ProviderID)
			if ok {
				if _, exists := provider.Models[modelName]; !exists {
					writeJSON(w, http.StatusBadRequest, map[string]string{
						"error": fmt.Sprintf("model %q is not supported by provider %q", modelName, cred.ProviderID),
					})
					return
				}
			}
		}
	}

	if req.SandboxTemplateID != nil {
		if *req.SandboxTemplateID == "" {
			updates["sandbox_template_id"] = nil
		} else {
			var tmpl model.SandboxTemplate
			if err := h.db.Where("id = ? AND org_id = ?", *req.SandboxTemplateID, org.ID).First(&tmpl).Error; err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "sandbox template not found"})
				return
			}
			updates["sandbox_template_id"] = tmpl.ID
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
		filtered := make(model.JSON, len(req.Permissions))
		for key, val := range req.Permissions {
			if !model.IsValidPermissionKey(key) {
				continue
			}
			filtered[key] = val
		}
		updates["permissions"] = filtered
	}
	if req.Resources != nil {
		updates["resources"] = req.Resources
	}
	if req.Team != nil {
		updates["team"] = *req.Team
	}
	if req.SharedMemory != nil {
		updates["shared_memory"] = *req.SharedMemory
	}
	if req.SandboxTools != nil {
		if invalid := model.ValidateSandboxTools(req.SandboxTools); invalid != "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("invalid sandbox tool: %q", invalid)})
			return
		}
		updates["sandbox_tools"] = pq.StringArray(req.SandboxTools)
	}

	// Validate trigger inputs per trigger_type (webhook | http | cron).
	if req.Triggers != nil {
		if errMsg := validateAgentTriggers(h.db, org.ID, *req.Triggers); errMsg != "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": errMsg})
			return
		}
	}

	if len(updates) > 0 {
		if err := h.db.Model(&agent).Updates(updates).Error; err != nil {
			if isDuplicateKeyError(err) {
				writeJSON(w, http.StatusConflict, map[string]string{"error": "agent with that name already exists in this workspace"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update agent"})
			return
		}
	}

	h.db.Preload("Credential").Where("id = ?", agent.ID).First(&agent)

	if req.Triggers != nil {
		if err := deleteAgentTriggers(h.db, agent.ID); err != nil {
			slog.Error("failed to delete old triggers during update", "agent_id", agent.ID, "error", err)
		}
		if err := createAgentTriggers(h.db, org.ID, agent.ID, *req.Triggers); err != nil {
			slog.Error("failed to create new triggers during update", "agent_id", agent.ID, "error", err)
		}
	}

	if req.SkillIDs != nil {
		if err := h.db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Where("agent_id = ?", agent.ID).Delete(&model.AgentSkill{}).Error; err != nil {
				return err
			}
			for _, rawID := range *req.SkillIDs {
				skillUUID, parseErr := uuid.Parse(rawID)
				if parseErr != nil {
					continue
				}
				if err := tx.Create(&model.AgentSkill{
					AgentID: agent.ID,
					SkillID: skillUUID,
				}).Error; err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			slog.Error("failed to sync skills during update", "agent_id", agent.ID, "error", err)
		}
	}

	resp := toAgentResponse(agent)
	resp.Triggers = h.loadAgentTriggers(agent.ID)[agent.ID]
	resp.AttachedSkills = h.loadAgentSkills(agent.ID)[agent.ID]
	writeJSON(w, http.StatusOK, resp)
}