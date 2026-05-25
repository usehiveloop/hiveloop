package employeeruntime

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

type skillBundle struct {
	Description                  string            `json:"description"`
	Content                      string            `json:"content"`
	Files                        map[string]string `json:"files"`
	Manifest                     map[string]any    `json:"manifest"`
	RequiredEnvironmentVariables []string          `json:"required_environment_variables"`
}

func buildSkills(ctx context.Context, db *gorm.DB, agentID uuid.UUID) ([]SkillSpec, error) {
	return buildSkillsWithDefaultNames(ctx, db, agentID, nil)
}

func buildSkillsWithDefaultNames(ctx context.Context, db *gorm.DB, agentID uuid.UUID, defaultNames []string) ([]SkillSpec, error) {
	if db == nil {
		return []SkillSpec{}, nil
	}
	var links []model.EmployeeSkill
	if err := db.WithContext(ctx).Where("employee_id = ?", agentID).Find(&links).Error; err != nil {
		return nil, err
	}
	defaultNames = normalizeSkillNames(defaultNames)
	if len(links) == 0 && len(defaultNames) == 0 {
		return []SkillSpec{}, nil
	}
	ids := make([]uuid.UUID, 0, len(links))
	for _, link := range links {
		ids = append(ids, link.SkillID)
	}
	var skills []model.Skill
	if len(ids) > 0 {
		if err := db.WithContext(ctx).Where("id IN ?", ids).Find(&skills).Error; err != nil {
			return nil, err
		}
	}
	if len(defaultNames) > 0 {
		var defaultSkills []model.Skill
		if err := db.WithContext(ctx).
			Where("org_id IS NULL AND status = ? AND (name IN ? OR slug IN ?)", model.SkillStatusPublished, defaultNames, defaultNames).
			Find(&defaultSkills).Error; err != nil {
			return nil, err
		}
		skills = appendMissingSkills(skills, defaultSkills)
	}
	sort.SliceStable(skills, func(i, j int) bool {
		if skills[i].Slug == skills[j].Slug {
			return skills[i].ID.String() < skills[j].ID.String()
		}
		return skills[i].Slug < skills[j].Slug
	})
	out := make([]SkillSpec, 0, len(skills))
	for _, skill := range skills {
		if len(skill.Bundle) == 0 {
			continue
		}
		var bundle skillBundle
		if err := json.Unmarshal(skill.Bundle, &bundle); err != nil {
			continue
		}
		description := bundle.Description
		if skill.Description != nil && *skill.Description != "" {
			description = *skill.Description
		}
		tags := []string(skill.Tags)
		sort.Strings(tags)
		requiredEnv := normalizeRequiredEnvironmentVariables(bundle.RequiredEnvironmentVariables)
		if len(requiredEnv) == 0 {
			requiredEnv = requiredEnvironmentVariablesFromManifest(bundle.Manifest)
		}
		out = append(out, SkillSpec{
			Name:                         skill.Slug,
			Description:                  description,
			Trigger:                      map[string]any{"type": "keyword", "patterns": []string{skill.Slug, skill.Name}},
			Instructions:                 composeInstructions(skill, bundle),
			Files:                        bundle.Files,
			Tags:                         tags,
			RelatedSkills:                []string{},
			RequiredEnvironmentVariables: requiredEnv,
			RequiredCredentialFiles:      []string{},
		})
	}
	return out, nil
}

func appendMissingSkills(existing []model.Skill, candidates []model.Skill) []model.Skill {
	seenIDs := make(map[uuid.UUID]bool, len(existing)+len(candidates))
	seenSlugs := make(map[string]bool, len(existing)+len(candidates))
	for _, skill := range existing {
		seenIDs[skill.ID] = true
		seenSlugs[skill.Slug] = true
	}
	for _, skill := range candidates {
		if seenIDs[skill.ID] || seenSlugs[skill.Slug] {
			continue
		}
		existing = append(existing, skill)
		seenIDs[skill.ID] = true
		seenSlugs[skill.Slug] = true
	}
	return existing
}

func normalizeSkillNames(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func normalizeRequiredEnvironmentVariables(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func requiredEnvironmentVariablesFromManifest(manifest map[string]any) []string {
	if len(manifest) == 0 {
		return []string{}
	}
	raw, ok := manifest["required_environment_variables"]
	if !ok {
		return []string{}
	}
	switch value := raw.(type) {
	case []string:
		return normalizeRequiredEnvironmentVariables(value)
	case []any:
		values := make([]string, 0, len(value))
		for _, item := range value {
			if s, ok := item.(string); ok {
				values = append(values, s)
			}
		}
		return normalizeRequiredEnvironmentVariables(values)
	default:
		return []string{}
	}
}

func composeInstructions(skill model.Skill, bundle skillBundle) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("name: ")
	b.WriteString(skill.Slug)
	b.WriteString("\n")
	if bundle.Description != "" {
		b.WriteString("description: ")
		encoded, _ := json.Marshal(bundle.Description)
		b.Write(encoded)
		b.WriteString("\n")
	}
	b.WriteString("---\n\n")
	b.WriteString(bundle.Content)
	if !strings.HasSuffix(bundle.Content, "\n") {
		b.WriteString("\n")
	}
	return b.String()
}
