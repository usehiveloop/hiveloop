package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// Create handles POST /v1/agents.
// @Summary Create an agent
// @Description Creates a new agent tied to a credential. A dedicated sandbox is provisioned lazily on conversation create.
// @Tags agents
// @Accept json
// @Produce json
// @Param body body createAgentRequest true "Agent definition"
// @Success 201 {object} agentResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents [post]
func (h *AgentHandler) Create(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	var req createAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}

	if !org.BYOK && req.CredentialID != "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "credential_id is only allowed when BYOK is enabled for this workspace"})
		return
	}

	if err := validateAgentModel(h.registry, req.Model); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if len(req.ProviderPrompts) > 0 {
		if errMsg := req.ProviderPrompts.Validate(); errMsg != "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": errMsg})
			return
		}
	}

	if req.Category != nil && *req.Category != "" && !isValidAgentCategory(*req.Category) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("invalid category %q", *req.Category)})
		return
	}

	if len(req.SandboxTools) > 0 {
		if invalid := model.ValidateSandboxTools(req.SandboxTools); invalid != "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("invalid sandbox tool: %q", invalid)})
			return
		}
	}

	if len(req.Permissions) > 0 {
		permKeys := make(map[string]string, len(req.Permissions))
		for key, val := range req.Permissions {
			str, _ := val.(string)
			permKeys[key] = str
		}
		if invalid := model.ValidatePermissionKeys(permKeys); invalid != "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("invalid permission tool: %q", invalid)})
			return
		}
	}

	var cred model.Credential
	var hasCred bool
	if req.CredentialID != "" {
		if err := h.db.Where("id = ? AND org_id = ? AND revoked_at IS NULL", req.CredentialID, org.ID).First(&cred).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "credential not found or revoked"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to validate credential"})
			return
		}
		hasCred = true
	}

	if errMsg := validateJSONSchema(req.AgentConfig); errMsg != "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": errMsg})
		return
	}

	if err := validateAgentIntegrationsExclusivity(h.db, org.ID, req.Integrations); err != nil {
		if errors.Is(err, errGitHubAppExclusive) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to validate integrations"})
		return
	}

	var sandboxTemplateID *interface{ String() string }
	_ = sandboxTemplateID
	agent := model.Agent{
		OrgID:           &org.ID,
		Name:            req.Name,
		Description:     req.Description,
		AvatarURL:       req.AvatarURL,
		Category:        req.Category,
		SystemPrompt:    req.SystemPrompt,
		ProviderPrompts: req.ProviderPrompts,
		Instructions:    req.Instructions,
		Model:           req.Model,
		Tools:           defaultJSON(req.Tools),
		McpServers:      defaultJSON(req.McpServers),
		Skills:          defaultJSON(req.Skills),
		Integrations:    defaultJSON(req.Integrations),
		AgentConfig:     defaultJSON(req.AgentConfig),
		Permissions:     defaultJSON(req.Permissions),
		Resources:       defaultJSON(req.Resources),
		Team:            req.Team,
		SharedMemory:    req.SharedMemory,
		SandboxTools:    pq.StringArray(req.SandboxTools),
		Status:          "active",
	}
	if hasCred {
		agent.CredentialID = &cred.ID
	}
	if len(agent.Permissions) == 0 {
		agent.Permissions = defaultToolPermissions()
	}

	if req.SandboxTemplateID != nil && *req.SandboxTemplateID != "" {
		var tmpl model.SandboxTemplate
		if err := h.db.Where("id = ? AND (org_id = ? OR org_id IS NULL)", *req.SandboxTemplateID, org.ID).First(&tmpl).Error; err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "sandbox template not found"})
			return
		}
		if tmpl.BuildStatus != "ready" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("sandbox template is not ready (status: %s)", tmpl.BuildStatus)})
			return
		}
		agent.SandboxTemplateID = &tmpl.ID
	}

	if errMsg := validateAgentTriggers(h.db, org.ID, req.Triggers); errMsg != "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": errMsg})
		return
	}

	var skillUUIDs []uuid.UUID
	if len(req.SkillIDs) > 0 {
		skillUUIDs = make([]uuid.UUID, 0, len(req.SkillIDs))
		seen := make(map[uuid.UUID]struct{}, len(req.SkillIDs))
		for _, raw := range req.SkillIDs {
			parsed, parseErr := uuid.Parse(raw)
			if parseErr != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("invalid skill_id %q", raw)})
				return
			}
			if _, dup := seen[parsed]; dup {
				continue
			}
			seen[parsed] = struct{}{}
			skillUUIDs = append(skillUUIDs, parsed)
		}
	}

	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&agent).Error; err != nil {
			return err
		}

		if len(skillUUIDs) > 0 {
			var visibleSkills []model.Skill
			if err := tx.
				Select("id").
				Where("id IN ? AND (org_id = ? OR (org_id IS NULL AND status = ?))",
					skillUUIDs, org.ID, model.SkillStatusPublished).
				Find(&visibleSkills).Error; err != nil {
				return fmt.Errorf("validate skill_ids: %w", err)
			}
			if len(visibleSkills) != len(skillUUIDs) {
				return fmt.Errorf("one or more skill_ids are not visible to this org")
			}
			links := make([]model.AgentSkill, len(visibleSkills))
			for i, skill := range visibleSkills {
				links[i] = model.AgentSkill{AgentID: agent.ID, SkillID: skill.ID}
			}
			if err := tx.Create(&links).Error; err != nil {
				return fmt.Errorf("attach skills: %w", err)
			}
			if err := tx.Model(&model.Skill{}).
				Where("id IN ?", skillUUIDs).
				UpdateColumn("install_count", gorm.Expr("install_count + 1")).Error; err != nil {
				return fmt.Errorf("bump install_count: %w", err)
			}
		}

		if err := createAgentTriggers(tx, org.ID, agent.ID, req.Triggers); err != nil {
			return fmt.Errorf("create triggers: %w", err)
		}

		return nil
	})
	if err != nil {
		if isDuplicateKeyError(err) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": fmt.Sprintf("agent with name %q already exists in this workspace", req.Name)})
			return
		}
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to create agent",
			"error", err,
			"org_id", org.ID,
			"agent_name", req.Name,
			"trigger_count", len(req.Triggers),
		)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create agent"})
		return
	}

	h.db.Preload("Credential").Where("id = ?", agent.ID).First(&agent)

	resp := toAgentResponse(agent)
	resp.Triggers = h.loadAgentTriggers(agent.ID)[agent.ID]

	writeJSON(w, http.StatusCreated, resp)
}
