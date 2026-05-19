package handler

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehiveloop/hiveloop/internal/employeeprompts"
	"github.com/usehiveloop/hiveloop/internal/employeeruntime"
	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/model"
)

func ensureHivyEmployee(ctx context.Context, db *gorm.DB, orgID uuid.UUID) (*model.Agent, error) {
	var existing model.Agent
	err := db.WithContext(ctx).
		Where("org_id = ? AND is_employee = true AND is_system = false", orgID).
		Order("created_at ASC").
		First(&existing).Error
	if err == nil {
		return &existing, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("lookup Hivy employee: %w", err)
	}

	var out *model.Agent
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		agent, err := createHivyEmployeeWithDefaultsTx(ctx, tx, orgID)
		if err != nil {
			return err
		}
		out = agent
		return nil
	})
	if err != nil {
		if isDuplicateKeyError(err) || isUniqueViolation(err) {
			if refetch := db.WithContext(ctx).
				Where("org_id = ? AND is_employee = true AND is_system = false", orgID).
				Order("created_at ASC").
				First(&existing).Error; refetch == nil {
				return &existing, nil
			}
		}
		return nil, err
	}
	return out, nil
}

func createHivyEmployeeWithDefaultsTx(ctx context.Context, tx *gorm.DB, orgID uuid.UUID) (*model.Agent, error) {
	agent, subagents, err := createHivyEmployeeTx(ctx, tx, orgID)
	if err != nil {
		return nil, err
	}
	if err := attachPublishedGlobalSkillsTx(ctx, tx, agent.ID, defaultEmployeeSkills); err != nil {
		return nil, err
	}
	for _, subagent := range subagents {
		template := employeeAgentTemplateForSubagent(subagent)
		if template == nil {
			continue
		}
		if err := attachPublishedGlobalSkillsTx(ctx, tx, subagent.ID, template.DefaultSkillNames); err != nil {
			return nil, err
		}
	}
	return agent, nil
}

func createHivyEmployeeTx(ctx context.Context, tx *gorm.DB, orgID uuid.UUID) (*model.Agent, []*model.Agent, error) {
	team, err := ensureEngineeringTeam(tx, orgID)
	if err != nil {
		return nil, nil, err
	}

	choice, err := pickEmployeeCredential(tx)
	if err != nil {
		logging.FromContext(ctx).WarnContext(ctx, "no provider credential available for Hivy employee", "error", err, "org_id", orgID)
		choice = &employeeProviderChoice{model: employeeruntime.DefaultEmployeeModel}
	}

	desc := hivyEmployeeDescription
	agent := model.Agent{
		OrgID:          &orgID,
		Name:           hivyEmployeeName,
		Description:    &desc,
		SystemPrompt:   "",
		IdentityPrompt: employeeprompts.EngineeringIdentityPrompt,
		Model:          choice.model,
		TeamID:         &team.ID,
		Team:           team.Name,
		Harness:        employeeHarness,
		IsEmployee:     true,
		Status:         "draft",
		Tools:          model.JSON{},
		McpServers:     model.JSON{},
		Skills:         model.JSON{},
		Integrations:   model.JSON{},
		Resources:      model.JSON{},
		AgentConfig:    model.JSON{},
		Permissions:    model.JSON{},
	}
	if choice.cred != nil {
		agent.CredentialID = &choice.cred.ID
	}
	if err := tx.WithContext(ctx).Create(&agent).Error; err != nil {
		return nil, nil, fmt.Errorf("create Hivy employee: %w", err)
	}

	h := &EmployeeHandler{db: tx}
	subagents, err := h.ensureEmployeeAgentTemplatesTx(ctx, tx, &agent, team)
	if err != nil {
		return nil, nil, err
	}
	return &agent, subagents, nil
}

func attachPublishedGlobalSkillsTx(ctx context.Context, tx *gorm.DB, agentID uuid.UUID, names []string) error {
	required := make(map[string]bool, len(names))
	for _, name := range names {
		required[name] = true
	}
	skills, err := loadPublishedGlobalSkillsByName(ctx, tx, required)
	if err != nil {
		return err
	}
	for _, name := range names {
		skill, ok := skills[name]
		if !ok {
			logging.FromContext(ctx).WarnContext(ctx, "global skill not attached to Hivy", "skill_name", name, "agent_id", agentID)
			continue
		}
		link := model.AgentSkill{AgentID: agentID, SkillID: skill.ID}
		if err := tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&link).Error; err != nil {
			return fmt.Errorf("attach global skill %q: %w", name, err)
		}
	}
	return nil
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
	attachPublishedGlobalSkills(ctx, h.db, agentID, names)
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

const subagentSlugMaxAttempts = 32

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

func createWithUniqueNameSlug(tx *gorm.DB, agent *model.Agent, baseSlug string) error {
	for i := 0; i < subagentSlugMaxAttempts; i++ {
		candidate := baseSlug
		if i > 0 {
			candidate = fmt.Sprintf("%s-%d", baseSlug, i+1)
		}
		agent.Name = candidate
		agent.ID = uuid.Nil

		exists, err := agentNameExists(tx, agent.OrgID, candidate)
		if err != nil {
			return err
		}
		if exists {
			continue
		}

		sp := fmt.Sprintf("sp_subagent_attempt_%d", i)
		if err := tx.SavePoint(sp).Error; err != nil {
			return fmt.Errorf("savepoint: %w", err)
		}
		err = tx.Create(agent).Error
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

func agentNameExists(tx *gorm.DB, orgID *uuid.UUID, name string) (bool, error) {
	var count int64
	query := tx.Model(&model.Agent{}).Where("name = ?", name)
	if orgID == nil {
		query = query.Where("org_id IS NULL")
	} else {
		query = query.Where("org_id = ?", *orgID)
	}
	if err := query.Count(&count).Error; err != nil {
		return false, fmt.Errorf("check agent name: %w", err)
	}
	return count > 0, nil
}
