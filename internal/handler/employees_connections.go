package handler

import (
	"context"
	"fmt"
	"sort"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/model"
)

var employeeConnectionSkillNames = map[string][]string{
	"bugsink":                 {"bugsink"},
	"github":                  {"git-github"},
	"github-app":              {"git-github"},
	"github-app-code-reviews": {"git-github"},
}

func employeeIntegrationsFromConnectionIDs(ids []uuid.UUID) model.JSON {
	out := model.JSON{}
	for _, id := range ids {
		out[id.String()] = map[string]any{"actions": []string{}}
	}
	return out
}

func employeeConnectionIDsFromIntegrations(integrations model.JSON) []uuid.UUID {
	if len(integrations) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, 0, len(integrations))
	for rawID := range integrations {
		id, err := uuid.Parse(rawID)
		if err == nil {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool {
		return ids[i].String() < ids[j].String()
	})
	return ids
}

func validateEmployeeConnectionIDs(ctx context.Context, db *gorm.DB, orgID uuid.UUID, rawIDs []string) ([]uuid.UUID, []model.InConnection, error) {
	if len(rawIDs) == 0 {
		return nil, nil, nil
	}
	seen := map[uuid.UUID]bool{}
	ids := make([]uuid.UUID, 0, len(rawIDs))
	for _, rawID := range rawIDs {
		id, err := uuid.Parse(rawID)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid connection_id %q", rawID)
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil, nil, nil
	}
	var connections []model.InConnection
	if err := db.WithContext(ctx).
		Preload("InIntegration").
		Where("id IN ? AND org_id = ? AND revoked_at IS NULL", ids, orgID).
		Find(&connections).Error; err != nil {
		return nil, nil, fmt.Errorf("load employee connections: %w", err)
	}
	if len(connections) != len(ids) {
		return nil, nil, fmt.Errorf("connection not found or revoked")
	}
	return ids, connections, nil
}

func employeeRequiredSkillNames(ctx context.Context, db *gorm.DB, orgID uuid.UUID, category *string, integrations model.JSON) (map[string]bool, []string, error) {
	names := map[string]bool{}
	if category != nil {
		for _, name := range defaultEmployeeSkills[*category] {
			names[name] = true
		}
	}

	connectionIDs := employeeConnectionIDsFromIntegrations(integrations)
	if len(connectionIDs) == 0 {
		return names, nil, nil
	}
	var connections []model.InConnection
	if err := db.WithContext(ctx).
		Preload("InIntegration").
		Where("id IN ? AND org_id = ? AND revoked_at IS NULL", connectionIDs, orgID).
		Find(&connections).Error; err != nil {
		return nil, nil, fmt.Errorf("load employee connection providers: %w", err)
	}

	warnings := make([]string, 0)
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
	names, _, err := employeeRequiredSkillNames(ctx, db, orgID, agent.Category, agent.Integrations)
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

func (h *EmployeeHandler) markEmployeeSkillLocks(ctx context.Context, orgID uuid.UUID, agent *model.Agent, summaries []agentSkillSummary) []agentSkillSummary {
	return markEmployeeSkillSummaries(ctx, h.db, orgID, agent, summaries)
}

func markEmployeeSkillSummaries(ctx context.Context, db *gorm.DB, orgID uuid.UUID, agent *model.Agent, summaries []agentSkillSummary) []agentSkillSummary {
	if len(summaries) == 0 {
		return summaries
	}
	names, _, err := employeeRequiredSkillNames(ctx, db, orgID, agent.Category, agent.Integrations)
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
