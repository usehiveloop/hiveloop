package handler

import (
	"context"
	"fmt"
	"sort"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

func employeeRequiredSkillNames(ctx context.Context, db *gorm.DB, orgID uuid.UUID) (map[string]bool, []string, error) {
	names := map[string]bool{}
	warnings := make([]string, 0)
	var connections []model.InConnection
	if err := db.WithContext(ctx).
		Preload("InIntegration").
		Joins("JOIN in_integrations ON in_integrations.id = in_connections.in_integration_id AND in_integrations.deleted_at IS NULL").
		Where("in_connections.org_id = ? AND in_connections.revoked_at IS NULL", orgID).
		Find(&connections).Error; err != nil {
		return nil, nil, fmt.Errorf("load employee connection providers: %w", err)
	}

	for _, conn := range connections {
		provider := conn.InIntegration.Provider
		if provider == "" {
			provider = conn.InIntegration.UniqueKey
		}
		mapped := employeeConnectionSkillNames[provider]
		if len(mapped) == 0 {
			display := conn.InIntegration.DisplayName
			if display == "" {
				display = provider
			}
			warnings = append(warnings, fmt.Sprintf("%s connections do not have an employee skill mapping yet", display))
			continue
		}
		for _, name := range mapped {
			names[name] = true
		}
	}
	return names, warnings, nil
}

func attachEmployeeRequiredSkills(ctx context.Context, db *gorm.DB, agentID uuid.UUID, names map[string]bool) ([]string, error) {
	if len(names) == 0 {
		return nil, nil
	}
	skills, err := loadPublishedGlobalSkillsByName(ctx, db, names)
	if err != nil {
		return nil, err
	}

	warnings := make([]string, 0)
	for name := range names {
		skill, ok := skills[name]
		if !ok {
			warnings = append(warnings, fmt.Sprintf("global skill %q is not published yet", name))
			continue
		}
		link := model.AgentSkill{AgentID: agentID, SkillID: skill.ID}
		result := db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&link)
		if result.Error != nil {
			return warnings, fmt.Errorf("attach global skill %q: %w", name, result.Error)
		}
		if result.RowsAffected == 0 {
			continue
		}
		if err := db.WithContext(ctx).Model(&model.Skill{}).
			Where("id = ?", skill.ID).
			UpdateColumn("install_count", gorm.Expr("install_count + 1")).Error; err != nil {
			logging.FromContext(ctx).WarnContext(ctx, "bump install_count for employee global skill",
				"error", err, "skill_id", skill.ID, "skill_name", name)
		}
	}
	sort.Strings(warnings)
	return warnings, nil
}

func attachEmployeeRequiredSkillsForAgent(ctx context.Context, db *gorm.DB, orgID uuid.UUID, agent *model.Agent) error {
	if agent == nil {
		return nil
	}
	names, _, err := employeeRequiredSkillNames(ctx, db, orgID)
	if err != nil {
		return err
	}
	warnings, err := attachEmployeeRequiredSkills(ctx, db, agent.ID, names)
	if err != nil {
		return err
	}
	for _, warning := range warnings {
		logging.FromContext(ctx).WarnContext(ctx, "employee required skill not attached",
			"warning", warning, "agent_id", agent.ID, "org_id", orgID)
	}
	return nil
}

func loadPublishedGlobalSkillsByName(ctx context.Context, db *gorm.DB, names map[string]bool) (map[string]model.Skill, error) {
	if len(names) == 0 {
		return nil, nil
	}
	nameList := make([]string, 0, len(names))
	for name := range names {
		nameList = append(nameList, name)
	}

	var rows []model.Skill
	if err := db.WithContext(ctx).
		Where("org_id IS NULL AND status = ? AND name IN ?", model.SkillStatusPublished, nameList).
		Order("created_at DESC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]model.Skill, len(rows))
	for _, row := range rows {
		if _, exists := out[row.Name]; exists {
			continue
		}
		out[row.Name] = row
	}
	return out, nil
}

func employeeLockedSkillIDs(ctx context.Context, db *gorm.DB, orgID uuid.UUID, agent *model.Agent) (map[uuid.UUID]bool, error) {
	names, _, err := employeeRequiredSkillNames(ctx, db, orgID)
	if err != nil {
		return nil, err
	}
	skills, err := loadPublishedGlobalSkillsByName(ctx, db, names)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]bool, len(skills))
	for _, skill := range skills {
		out[skill.ID] = true
	}
	return out, nil
}

func (h *EmployeeHandler) markEmployeeSkillLocks(ctx context.Context, orgID uuid.UUID, agent *model.Agent, summaries []employeeSkillSummary) []employeeSkillSummary {
	return markEmployeeSkillSummaries(ctx, h.db, orgID, agent, summaries)
}

func markEmployeeSkillSummaries(ctx context.Context, db *gorm.DB, orgID uuid.UUID, agent *model.Agent, summaries []employeeSkillSummary) []employeeSkillSummary {
	if len(summaries) == 0 {
		return summaries
	}
	names, _, err := employeeRequiredSkillNames(ctx, db, orgID)
	if err != nil {
		logging.FromContext(ctx).WarnContext(ctx, "load employee required skill names",
			"error", err, "agent_id", agent.ID, "org_id", orgID)
		return summaries
	}
	for i := range summaries {
		if names[summaries[i].Name] {
			summaries[i].Locked = true
			summaries[i].Required = true
		}
	}
	return summaries
}
