package handler

import (
	"context"
	"fmt"
	"sort"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

func employeeRequiredSkills(ctx context.Context, db *gorm.DB, orgID uuid.UUID) (map[uuid.UUID]model.Skill, []string, error) {
	providers, displays, err := activeEmployeeConnectionProviders(ctx, db, orgID)
	if err != nil {
		return nil, nil, err
	}
	if len(providers) == 0 {
		return map[uuid.UUID]model.Skill{}, nil, nil
	}

	skills, err := loadPublishedGlobalSkillsByIntegrationIDs(ctx, db, providers)
	if err != nil {
		return nil, nil, err
	}

	warnings := make([]string, 0)
	mappedProviders := map[string]bool{}
	for _, skill := range skills {
		for _, integrationID := range skill.IntegrationIDs {
			mappedProviders[integrationID] = true
		}
	}
	for _, provider := range providers {
		if mappedProviders[provider] {
			continue
		}
		display := displays[provider]
		if display == "" {
			display = provider
		}
		warnings = append(warnings, fmt.Sprintf("%s connections do not have an employee skill mapping yet", display))
	}
	sort.Strings(warnings)
	return skills, warnings, nil
}

func activeEmployeeConnectionProviders(ctx context.Context, db *gorm.DB, orgID uuid.UUID) ([]string, map[string]string, error) {
	var connections []model.InConnection
	if err := db.WithContext(ctx).
		Preload("InIntegration").
		Joins("JOIN in_integrations ON in_integrations.id = in_connections.in_integration_id AND in_integrations.deleted_at IS NULL").
		Where("in_connections.org_id = ? AND in_connections.revoked_at IS NULL", orgID).
		Find(&connections).Error; err != nil {
		return nil, nil, fmt.Errorf("load employee connection providers: %w", err)
	}

	seen := map[string]bool{}
	providers := make([]string, 0, len(connections))
	displays := make(map[string]string, len(connections))
	for _, conn := range connections {
		provider := conn.InIntegration.Provider
		if provider == "" {
			provider = conn.InIntegration.UniqueKey
		}
		if provider == "" || seen[provider] {
			continue
		}
		seen[provider] = true
		providers = append(providers, provider)
		displays[provider] = conn.InIntegration.DisplayName
	}
	sort.Strings(providers)
	return providers, displays, nil
}

func attachEmployeeRequiredSkills(ctx context.Context, db *gorm.DB, agentID uuid.UUID, skills map[uuid.UUID]model.Skill) error {
	if len(skills) == 0 {
		return nil
	}

	for _, skill := range skills {
		link := model.AgentSkill{AgentID: agentID, SkillID: skill.ID}
		result := db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&link)
		if result.Error != nil {
			return fmt.Errorf("attach global skill %q: %w", skill.Name, result.Error)
		}
		if result.RowsAffected == 0 {
			continue
		}
		if err := db.WithContext(ctx).Model(&model.Skill{}).
			Where("id = ?", skill.ID).
			UpdateColumn("install_count", gorm.Expr("install_count + 1")).Error; err != nil {
			logging.FromContext(ctx).WarnContext(ctx, "bump install_count for employee global skill",
				"error", err, "skill_id", skill.ID, "skill_name", skill.Name)
		}
	}
	return nil
}

func attachEmployeeRequiredSkillsForAgent(ctx context.Context, db *gorm.DB, orgID uuid.UUID, agent *model.Agent) error {
	if agent == nil {
		return nil
	}
	skills, warnings, err := employeeRequiredSkills(ctx, db, orgID)
	if err != nil {
		return err
	}
	if err := attachEmployeeRequiredSkills(ctx, db, agent.ID, skills); err != nil {
		return err
	}
	for _, warning := range warnings {
		logging.FromContext(ctx).WarnContext(ctx, "employee required skill not attached",
			"warning", warning, "agent_id", agent.ID, "org_id", orgID)
	}
	return nil
}

func loadPublishedGlobalSkillsByIntegrationIDs(ctx context.Context, db *gorm.DB, integrationIDs []string) (map[uuid.UUID]model.Skill, error) {
	if len(integrationIDs) == 0 {
		return map[uuid.UUID]model.Skill{}, nil
	}
	var rows []model.Skill
	if err := db.WithContext(ctx).
		Where("org_id IS NULL AND status = ? AND integration_ids && ?", model.SkillStatusPublished, pq.Array(integrationIDs)).
		Order("created_at DESC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]model.Skill, len(rows))
	for _, row := range rows {
		if _, exists := out[row.ID]; exists {
			continue
		}
		out[row.ID] = row
	}
	return out, nil
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
	skills, _, err := employeeRequiredSkills(ctx, db, orgID)
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
	skills, _, err := employeeRequiredSkills(ctx, db, orgID)
	if err != nil {
		logging.FromContext(ctx).WarnContext(ctx, "load employee required skills",
			"error", err, "agent_id", agent.ID, "org_id", orgID)
		return summaries
	}
	requiredIDs := make(map[string]bool, len(skills))
	for id := range skills {
		requiredIDs[id.String()] = true
	}
	for i := range summaries {
		if requiredIDs[summaries[i].ID] {
			summaries[i].Locked = true
			summaries[i].Required = true
		}
	}
	return summaries
}
