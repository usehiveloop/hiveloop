package handler

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/employeeprompts"
	"github.com/usehivy/hivy/internal/employeeruntime"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

func ensureHivyEmployee(ctx context.Context, db *gorm.DB, orgID uuid.UUID) (*model.Employee, error) {
	var existing model.Employee
	err := db.WithContext(ctx).
		Where("org_id = ? AND status <> ?", orgID, "archived").
		Order("created_at ASC").
		First(&existing).Error
	if err == nil {
		if err := ensureAutoAttachedSpecialists(ctx, db, &existing, specialistCatalogFromArgs().AutoAttachSlugs()); err != nil {
			return nil, err
		}
		return &existing, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("lookup Hivy employee: %w", err)
	}

	var out *model.Employee
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		agent, err := createHivyEmployeeWithDefaultsTx(ctx, tx, orgID)
		if err != nil {
			return err
		}
		out = agent
		return nil
	})
	if err != nil {
		if isDuplicateKeyError(err) {
			if refetch := db.WithContext(ctx).
				Where("org_id = ? AND status <> ?", orgID, "archived").
				Order("created_at ASC").
				First(&existing).Error; refetch == nil {
				return &existing, nil
			}
		}
		return nil, err
	}
	return out, nil
}

func ensureAutoAttachedSpecialists(ctx context.Context, db *gorm.DB, agent *model.Employee, slugs []string) error {
	if agent == nil || len(slugs) == 0 {
		return nil
	}
	next := append([]string(nil), agent.AttachedSpecialists...)
	changed := false
	for _, slug := range slugs {
		before := len(next)
		next = setAttachedSpecialist(next, slug, true)
		if len(next) != before {
			changed = true
		}
	}
	if !changed {
		return nil
	}
	if err := db.WithContext(ctx).
		Model(&model.Employee{}).
		Where("id = ?", agent.ID).
		Update("attached_specialists", pq.StringArray(next)).Error; err != nil {
		return fmt.Errorf("auto-attach specialists: %w", err)
	}
	agent.AttachedSpecialists = next
	return nil
}

func createHivyEmployeeWithDefaultsTx(ctx context.Context, tx *gorm.DB, orgID uuid.UUID) (*model.Employee, error) {
	agent, err := createHivyEmployeeTx(ctx, tx, orgID, specialistCatalogFromArgs().AutoAttachSlugs())
	if err != nil {
		return nil, err
	}
	if err := attachPublishedGlobalSkillsTx(ctx, tx, agent.ID, defaultEmployeeSkills); err != nil {
		return nil, err
	}
	return agent, nil
}

func createHivyEmployeeTx(ctx context.Context, tx *gorm.DB, orgID uuid.UUID, attachedSpecialists []string) (*model.Employee, error) {
	choice, err := pickEmployeeCredential(tx)
	if err != nil {
		logging.FromContext(ctx).WarnContext(ctx, "no provider credential available for Hivy employee", "error", err, "org_id", orgID)
		choice = &employeeProviderChoice{model: employeeruntime.DefaultEmployeeModel}
	}

	desc := hivyEmployeeDescription
	agent := model.Employee{
		OrgID:               &orgID,
		Name:                hivyEmployeeName,
		Description:         &desc,
		SystemPrompt:        "",
		IdentityPrompt:      employeeprompts.EngineeringIdentityPrompt,
		Model:               choice.model,
		Harness:             employeeHarness,
		Status:              "draft",
		Tools:               model.JSON{},
		McpServers:          model.JSON{},
		Skills:              model.JSON{},
		Integrations:        model.JSON{},
		Resources:           model.JSON{},
		RuntimeConfig:       model.JSON{},
		Permissions:         model.JSON{},
		AttachedSpecialists: attachedSpecialists,
	}
	if choice.cred != nil {
		agent.CredentialID = &choice.cred.ID
	}
	if err := tx.WithContext(ctx).Create(&agent).Error; err != nil {
		return nil, fmt.Errorf("create Hivy employee: %w", err)
	}

	return &agent, nil
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
			logging.FromContext(ctx).WarnContext(ctx, "global skill not attached to Hivy", "skill_name", name, "employee_id", agentID)
			continue
		}
		link := model.EmployeeSkill{EmployeeID: agentID, SkillID: skill.ID}
		if err := tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&link).Error; err != nil {
			return fmt.Errorf("attach global skill %q: %w", name, err)
		}
	}
	return nil
}

func (h *EmployeeHandler) attachGlobalSkills(ctx context.Context, agentID uuid.UUID, names []string) {
	attachPublishedGlobalSkills(ctx, h.db, agentID, names)
}

func (h *EmployeeHandler) rollbackEmployee(ctx context.Context, orgID, agentID, subagentID uuid.UUID) {
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("org_id = ? AND employee_id = ?", orgID, agentID).Delete(&model.Sandbox{}).Error; err != nil {
			return fmt.Errorf("delete sandbox: %w", err)
		}
		if err := tx.Where("org_id = ? AND id = ?", orgID, agentID).Delete(&model.Employee{}).Error; err != nil {
			return fmt.Errorf("delete employee agent: %w", err)
		}
		if subagentID != uuid.Nil {
			if err := tx.Where("org_id = ? AND id = ?", orgID, subagentID).Delete(&model.Employee{}).Error; err != nil {
				return fmt.Errorf("delete subagent: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "rollback employee", "error", err,
			"employee_id", agentID, "subemployee_id", subagentID, "org_id", orgID)
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

func createWithUniqueNameSlug(tx *gorm.DB, agent *model.Employee, baseSlug string) error {
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
	query := tx.Model(&model.Employee{}).Where("name = ?", name)
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
