package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// @Summary Create an AI employee
// @Description Persists an Agent (is_employee=true). The employee sandbox is
// @Description provisioned after an active channel profile exists, during onboarding
// @Description completion or explicit sync.
// @Tags employees
// @Accept json
// @Produce json
// @Param body body createEmployeeRequest true "Employee definition"
// @Success 201 {object} createEmployeeResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Failure 503 {object} errorResponse
// @Security BearerAuth
// @Router /v1/employees [post]
func (h *EmployeeHandler) Create(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	var req createEmployeeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Category = strings.TrimSpace(req.Category)
	req.AvatarURL = strings.TrimSpace(req.AvatarURL)
	req.Description = strings.TrimSpace(req.Description)

	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}
	if req.Description == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "description is required"})
		return
	}
	if req.Category == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "category is required"})
		return
	}
	if !isValidAgentCategory(req.Category) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("invalid category %q", req.Category)})
		return
	}
	if req.Category != employeeCategoryEngineering {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("category %q is not yet supported for employees", req.Category)})
		return
	}
	if req.AvatarURL != "" {
		u, err := url.Parse(req.AvatarURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "avatar_url must be an absolute http(s) URL"})
			return
		}
	}

	choice, err := pickEmployeeCredential(h.db)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "pick employee credential", "error", err, "org_id", org.ID)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "no provider credential available for new employees"})
		return
	}

	subChoice, err := pickEmployeeSubagentCredential(h.db)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "pick employee subagent credential", "error", err, "org_id", org.ID)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "no provider credential available for new employee subagent"})
		return
	}

	team, err := ensureEngineeringTeam(h.db, org.ID)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "ensure engineering team", "error", err, "org_id", org.ID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to set up team for employee"})
		return
	}

	desc := req.Description
	cat := req.Category
	agent := model.Agent{
		OrgID:        &org.ID,
		Name:         req.Name,
		Description:  &desc,
		Category:     &cat,
		SystemPrompt: engineeringSystemPrompt,
		Model:        choice.model,
		CredentialID: &choice.cred.ID,
		TeamID:       &team.ID,
		Team:         team.Name,
		Harness:      employeeHarness,
		IsEmployee:   true,
		Status:       "active",
		Tools:        model.JSON{},
		McpServers:   model.JSON{},
		Skills:       model.JSON{},
		Integrations: model.JSON{},
		Resources:    model.JSON{},
		AgentConfig:  model.JSON{},
		Permissions:  model.JSON{},
	}
	if req.AvatarURL != "" {
		avatar := req.AvatarURL
		agent.AvatarURL = &avatar
	}

	subDesc := fmt.Sprintf("Subagent for %s.", req.Name)
	subagent := model.Agent{
		OrgID:        &org.ID,
		Description:  &subDesc,
		Category:     &cat,
		SystemPrompt: engineeringSubagentSystemPrompt,
		Model:        subChoice.model,
		CredentialID: &subChoice.cred.ID,
		TeamID:       &team.ID,
		Team:         team.Name,
		Harness:      employeeHarness,
		IsEmployee:   false,
		Status:       "active",
		Tools:        model.JSON{},
		McpServers:   model.JSON{},
		Skills:       model.JSON{},
		Integrations: model.JSON{},
		Resources:    model.JSON{},
		AgentConfig:  model.JSON{},
		Permissions:  model.JSON{},
	}

	subBaseSlug := employeeSubagentBaseSlug(req.Name, req.Category)

	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&agent).Error; err != nil {
			return err
		}
		if err := createWithUniqueNameSlug(tx, &subagent, subBaseSlug); err != nil {
			return err
		}
		return tx.Create(&model.AgentSubagent{
			AgentID:    agent.ID,
			SubagentID: subagent.ID,
		}).Error
	})
	if err != nil {
		if isDuplicateKeyError(err) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": fmt.Sprintf("employee with name %q already exists", req.Name)})
			return
		}
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "create employee + subagent", "error", err, "org_id", org.ID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create employee"})
		return
	}

	h.attachGlobalSkills(r.Context(), agent.ID, defaultEmployeeSkills[req.Category])
	h.attachGlobalSkills(r.Context(), subagent.ID, defaultEmployeeSubagentSkills[req.Category])

	writeJSON(w, http.StatusCreated, createEmployeeResponse{
		AgentID:   agent.ID.String(),
		SandboxID: "",
		Status:    "pending_profile",
	})
}

func ensureEngineeringTeam(db *gorm.DB, orgID uuid.UUID) (*model.Team, error) {
	var team model.Team
	err := db.Where("org_id = ? AND name = ? AND deleted_at IS NULL", orgID, engineeringTeamName).
		First(&team).Error
	if err == nil {
		return &team, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("lookup engineering team: %w", err)
	}

	team = model.Team{
		OrgID:       orgID,
		Name:        engineeringTeamName,
		Description: "AI engineering team.",
	}
	if err := db.Create(&team).Error; err != nil {
		if isUniqueViolation(err) {
			if err := db.Where("org_id = ? AND name = ? AND deleted_at IS NULL", orgID, engineeringTeamName).
				First(&team).Error; err != nil {
				return nil, fmt.Errorf("refetch engineering team after race: %w", err)
			}
			return &team, nil
		}
		return nil, fmt.Errorf("create engineering team: %w", err)
	}
	return &team, nil
}

func (h *EmployeeHandler) attachGlobalSkills(ctx context.Context, agentID uuid.UUID, names []string) {
	if len(names) == 0 {
		return
	}
	log := logging.FromContext(ctx)
	for _, name := range names {
		var skill model.Skill
		err := h.db.
			Where("org_id IS NULL AND status = ? AND name = ?", model.SkillStatusPublished, name).
			First(&skill).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				log.WarnContext(ctx, "default global skill not found, skipping",
					"agent_id", agentID, "skill_name", name)
			} else {
				log.ErrorContext(ctx, "lookup default global skill",
					"error", err, "agent_id", agentID, "skill_name", name)
			}
			continue
		}
		link := model.AgentSkill{AgentID: agentID, SkillID: skill.ID}
		if err := h.db.Create(&link).Error; err != nil {
			log.ErrorContext(ctx, "attach default global skill",
				"error", err, "agent_id", agentID, "skill_id", skill.ID, "skill_name", name)
			continue
		}
		if err := h.db.Model(&model.Skill{}).
			Where("id = ?", skill.ID).
			UpdateColumn("install_count", gorm.Expr("install_count + 1")).Error; err != nil {
			log.WarnContext(ctx, "bump install_count for default global skill",
				"error", err, "skill_id", skill.ID)
		}
	}
}

func (h *EmployeeHandler) rollbackEmployee(ctx context.Context, orgID, agentID, subagentID uuid.UUID) {
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("org_id = ? AND agent_id = ?", orgID, agentID).Delete(&model.Sandbox{}).Error; err != nil {
			return fmt.Errorf("delete sandbox: %w", err)
		}
		if err := tx.Where("org_id = ? AND id = ?", orgID, agentID).Delete(&model.Agent{}).Error; err != nil {
			return fmt.Errorf("delete employee agent: %w", err)
		}
		if subagentID != uuid.Nil {
			if err := tx.Where("org_id = ? AND id = ?", orgID, subagentID).Delete(&model.Agent{}).Error; err != nil {
				return fmt.Errorf("delete subagent: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "rollback employee", "error", err,
			"agent_id", agentID, "subagent_id", subagentID, "org_id", orgID)
	}
}

func slugifyAgentName(s string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevDash = false
		} else if !prevDash && b.Len() > 0 {
			b.WriteRune('-')
			prevDash = true
		}
	}
	return strings.TrimRight(b.String(), "-")
}

func employeeSubagentBaseSlug(employeeName, category string) string {
	s := slugifyAgentName(employeeName)
	if s == "" {
		s = category
	}
	return s + "-subagent"
}

const subagentSlugMaxAttempts = 32

func createWithUniqueNameSlug(tx *gorm.DB, agent *model.Agent, baseSlug string) error {
	for i := 0; i < subagentSlugMaxAttempts; i++ {
		candidate := baseSlug
		if i > 0 {
			candidate = fmt.Sprintf("%s-%d", baseSlug, i+1)
		}
		agent.Name = candidate
		agent.ID = uuid.Nil

		sp := fmt.Sprintf("sp_subagent_attempt_%d", i)
		if err := tx.SavePoint(sp).Error; err != nil {
			return fmt.Errorf("savepoint: %w", err)
		}
		err := tx.Create(agent).Error
		if err == nil {
			return nil
		}
		if !isDuplicateKeyError(err) {
			return err
		}
		if rbErr := tx.RollbackTo(sp).Error; rbErr != nil {
			return fmt.Errorf("rollback to savepoint: %w", rbErr)
		}
	}
	return fmt.Errorf("could not allocate unique subagent name after %d attempts (base=%s)", subagentSlugMaxAttempts, baseSlug)
}
