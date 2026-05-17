package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

type updateEmployeeRequest struct {
	Name          *string              `json:"name,omitempty"`
	AvatarURL     *string              `json:"avatar_url,omitempty"`
	Description   *string              `json:"description,omitempty"`
	ConnectionIDs *[]string            `json:"connection_ids,omitempty"`
	SkillIDs      *[]string            `json:"skill_ids,omitempty"`
	Triggers      *[]agentTriggerInput `json:"triggers,omitempty"`
}

type updateEmployeeResponse struct {
	Employee   employeeListItem      `json:"employee"`
	SyncStatus string                `json:"sync_status"`
	Sync       *syncEmployeeResponse `json:"sync,omitempty"`
	Warnings   []string              `json:"warnings,omitempty"`
}

// Update handles PUT /v1/employees/{id}.
// @Summary Update an AI employee
// @Description Updates employee creation fields, assigned org connections, and optional skills.
// @Description Category is read-only. Required backend-managed employee skills are preserved.
// @Tags employees
// @Accept json
// @Produce json
// @Param id path string true "Employee agent ID"
// @Param body body updateEmployeeRequest true "Fields to update"
// @Success 200 {object} updateEmployeeResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Failure 502 {object} errorResponse
// @Security BearerAuth
// @Router /v1/employees/{id} [put]
func (h *EmployeeHandler) Update(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logging.FromContext(ctx)

	org, ok := middleware.OrgFromContext(ctx)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	agentID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid employee id"})
		return
	}

	var agent model.Agent
	if err := h.db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND is_employee = true AND is_system = false", agentID, org.ID).
		First(&agent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "employee not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load employee"})
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

	var req updateEmployeeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	updates := map[string]any{}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
			return
		}
		updates["name"] = name
	}
	if req.Description != nil {
		description := strings.TrimSpace(*req.Description)
		if description == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "description is required"})
			return
		}
		updates["description"] = description
	}
	if req.AvatarURL != nil {
		avatarURL := strings.TrimSpace(*req.AvatarURL)
		if avatarURL == "" {
			updates["avatar_url"] = nil
		} else {
			u, err := url.Parse(avatarURL)
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "avatar_url must be an absolute http(s) URL"})
				return
			}
			updates["avatar_url"] = avatarURL
		}
	}

	nextIntegrations := agent.Integrations
	if req.ConnectionIDs != nil {
		connectionIDs, _, err := validateEmployeeConnectionIDs(ctx, h.db, org.ID, *req.ConnectionIDs)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		nextIntegrations = employeeIntegrationsFromConnectionIDs(connectionIDs)
		updates["integrations"] = nextIntegrations
	}

	var requestedSkillIDs map[uuid.UUID]bool
	if req.SkillIDs != nil {
		skillIDs, err := parseUniqueUUIDStrings(*req.SkillIDs, "skill_id")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if _, err := loadVisibleSkillsForOrg(ctx, h.db, org.ID, skillIDs); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		requestedSkillIDs = map[uuid.UUID]bool{}
		for _, skillID := range skillIDs {
			requestedSkillIDs[skillID] = true
		}
	}
	if req.Triggers != nil {
		if errMsg := validateAgentTriggers(h.db, org.ID, *req.Triggers); errMsg != "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": errMsg})
			return
		}
	}

	warnings := make([]string, 0)
	err = h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if len(updates) > 0 {
			if err := tx.Model(&model.Agent{}).
				Where("id = ? AND org_id = ? AND is_employee = true", agent.ID, org.ID).
				Updates(updates).Error; err != nil {
				return err
			}
		}

		requiredNames, connectionWarnings, err := employeeRequiredSkillNames(ctx, tx, org.ID, agent.ID, agent.Category, nextIntegrations)
		if err != nil {
			return err
		}
		warnings = append(warnings, connectionWarnings...)

		if requestedSkillIDs != nil {
			if err := tx.Where("agent_id = ?", agent.ID).Delete(&model.AgentSkill{}).Error; err != nil {
				return err
			}
			for skillID := range requestedSkillIDs {
				if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.AgentSkill{
					AgentID: agent.ID,
					SkillID: skillID,
				}).Error; err != nil {
					return err
				}
			}
		}

		skillWarnings, err := attachEmployeeRequiredSkills(ctx, tx, agent.ID, requiredNames)
		if err != nil {
			return err
		}
		warnings = append(warnings, skillWarnings...)

		if req.Triggers != nil {
			if err := replaceAgentTriggers(tx, org.ID, agent.ID, *req.Triggers); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		if isDuplicateKeyError(err) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "employee with that name already exists in this workspace"})
			return
		}
		log.ErrorContext(ctx, "update employee", "error", err, "agent_id", agent.ID, "org_id", org.ID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update employee"})
		return
	}

	if err := h.db.WithContext(ctx).Preload("Credential").Preload("TeamRef").Where("id = ?", agent.ID).First(&agent).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to reload employee"})
		return
	}

	syncStatus := "pending_profile"
	var syncResp *syncEmployeeResponse
	hasProfile, err := h.employeeHasActiveSlackProfile(ctx, org.ID, agent.ID)
	if err != nil {
		log.ErrorContext(ctx, "count employee profiles", "error", err, "agent_id", agent.ID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load employee profiles"})
		return
	}
	if hasProfile {
		if _, err := h.ensureEmployeeAgentTemplates(ctx, &agent); err != nil {
			log.ErrorContext(ctx, "ensure employee agent templates during update", "error", err, "agent_id", agent.ID, "org_id", org.ID)
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "failed to ensure employee agent templates"})
			return
		}
		sb, err := h.ensureEmployeeSandbox(ctx, &agent)
		if err != nil {
			log.ErrorContext(ctx, "provision employee sandbox during update", "error", err, "agent_id", agent.ID)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to provision employee sandbox"})
			return
		}
		resp, err := h.runEmployeeSync(ctx, &agent, sb)
		if err != nil {
			log.ErrorContext(ctx, "sync employee config after update", "error", err, "agent_id", agent.ID, "sandbox_id", sb.ID)
			logging.Capture(ctx, err)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "employee updated but sandbox rejected sync"})
			return
		}
		dto := toSyncResponseDTO(resp)
		syncResp = &dto
		syncStatus = "synced"
	}

	writeJSON(w, http.StatusOK, updateEmployeeResponse{
		Employee:   h.employeeListItem(ctx, org.ID, agent),
		SyncStatus: syncStatus,
		Sync:       syncResp,
		Warnings:   warnings,
	})
}
