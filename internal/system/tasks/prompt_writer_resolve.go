package tasks

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/system"
)

type resolvedSkill struct {
	Name        string
	Description string
	SourceType  string
}

type resolvedActionRef struct {
	Slug        string
	DisplayName string
	Description string
}

type resolvedIntegration struct {
	Provider        string
	ConnectionLabel string
	Actions         []resolvedActionRef
}

type resolvedTriggerKey struct {
	Display     string
	Description string
}

type resolvedTrigger struct {
	Type         string
	Provider     string
	Keys         []resolvedTriggerKey
	Instructions string
}

type resolvedTool struct {
	Name        string
	Description string
}

type connRow struct {
	ID          uuid.UUID `gorm:"column:id"`
	Provider    string    `gorm:"column:provider"`
	DisplayName string    `gorm:"column:display_name"`
}

func resolvePromptWriterArgs(ctx context.Context, deps system.ResolveDeps, args map[string]any) (map[string]any, error) {
	// Every key the user template references must be present — render runs
	// with missingkey=error.
	out := map[string]any{
		"name":         stringArg(args, "name"),
		"category":     stringArg(args, "category"),
		"instructions": stringArg(args, "instructions"),
		"skills":       []resolvedSkill{},
		"integrations": []resolvedIntegration{},
		"triggers":     []resolvedTrigger{},
		"tools":        []resolvedTool{},
		"permissions":  map[string]string{},
	}

	skills, err := resolveSkills(deps, stringSliceArg(args, "skill_ids"))
	if err != nil {
		return nil, err
	}
	if len(skills) > 0 {
		out["skills"] = skills
	}

	integrations, err := resolveIntegrations(deps, mapArg(args, "integrations"))
	if err != nil {
		return nil, err
	}
	if len(integrations) > 0 {
		out["integrations"] = integrations
	}

	triggers, err := resolveTriggers(deps, objectListArg(args, "triggers"))
	if err != nil {
		return nil, err
	}
	if len(triggers) > 0 {
		out["triggers"] = triggers
	}

	if tools := resolveBuiltinTools(mapArg(args, "tools"), stringSliceArg(args, "sandbox_tools")); len(tools) > 0 {
		out["tools"] = tools
	}

	if perms := stringMapFromArg(mapArg(args, "permissions")); len(perms) > 0 {
		out["permissions"] = perms
	}

	return out, nil
}

func resolveSkills(deps system.ResolveDeps, rawIDs []string) ([]resolvedSkill, error) {
	ids, err := parseUUIDs(rawIDs, "skill_id", "unknown_skill")
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	var rows []model.Skill
	if err := deps.DB.
		Select("id, name, description, source_type").
		Where("id IN ? AND (org_id = ? OR (org_id IS NULL AND status = ?))",
			ids, deps.OrgID, model.SkillStatusPublished).
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load skills: %w", err)
	}
	if len(rows) != len(ids) {
		return nil, &system.ResolveError{
			Code:    "unknown_skill",
			Message: missingMessage("skill", ids, rowsToIDsSkill(rows)),
		}
	}
	out := make([]resolvedSkill, len(rows))
	for i, row := range rows {
		desc := ""
		if row.Description != nil {
			desc = *row.Description
		}
		out[i] = resolvedSkill{
			Name:        row.Name,
			Description: desc,
			SourceType:  row.SourceType,
		}
	}
	return out, nil
}

func rowsToIDsSkill(rows []model.Skill) []uuid.UUID {
	out := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		out[i] = r.ID
	}
	return out
}
