package subagents

import (
	"embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
)

//go:embed */*.yaml
var subagentsFS embed.FS

type subagentFile struct {
	Name         string            `yaml:"name"`
	Description  string            `yaml:"description"`
	Model        string            `yaml:"model"`
	SystemPrompt string            `yaml:"system_prompt"`
	Skills       []string          `yaml:"skills"`
	Permissions  map[string]string `yaml:"permissions"`
}

// subagentGroup collects all provider YAML files for a single subagent type.
type subagentGroup struct {
	name        string
	description string
	skills      []string
	permissions map[string]string
	providers   map[string]subagentFile // provider_group -> parsed YAML
}

// Seed walks internal/sub-agents/<type>/<provider-group>.yaml, groups all
// provider files per subagent type, and upserts one row per type with
// provider-specific prompts stored in the provider_prompts JSONB column.
func Seed(db *gorm.DB) error {
	typeDirs, err := subagentsFS.ReadDir(".")
	if err != nil {
		return fmt.Errorf("reading sub-agents root: %w", err)
	}

	for _, typeDir := range typeDirs {
		if !typeDir.IsDir() {
			continue
		}

		group, err := loadGroup(typeDir.Name())
		if err != nil {
			return err
		}

		if err := seedGroup(db, group); err != nil {
			return err
		}
	}

	return nil
}

// loadGroup reads all provider YAMLs for a subagent type directory and
// returns them as a single group.
func loadGroup(subagentType string) (*subagentGroup, error) {
	files, err := subagentsFS.ReadDir(subagentType)
	if err != nil {
		return nil, fmt.Errorf("reading %s dir: %w", subagentType, err)
	}

	group := &subagentGroup{
		name:      subagentType,
		providers: make(map[string]subagentFile),
	}

	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".yaml") {
			continue
		}

		providerGroup := strings.TrimSuffix(file.Name(), ".yaml")
		path := subagentType + "/" + file.Name()

		data, err := subagentsFS.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}

		var sf subagentFile
		if err := yaml.Unmarshal(data, &sf); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", path, err)
		}

		if sf.SystemPrompt == "" {
			return nil, fmt.Errorf("%s: system_prompt is required", path)
		}
		if sf.Model == "" {
			return nil, fmt.Errorf("%s: model is required", path)
		}

		if sf.Name != "" {
			group.name = sf.Name
		}
		if sf.Description != "" {
			group.description = sf.Description
		}
		if len(sf.Skills) > 0 && len(group.skills) == 0 {
			group.skills = sf.Skills
		}
		if len(sf.Permissions) > 0 && len(group.permissions) == 0 {
			group.permissions = sf.Permissions
		}

		group.providers[providerGroup] = sf
	}

	if len(group.providers) == 0 {
		return nil, fmt.Errorf("%s: no YAML files found", subagentType)
	}

	return group, nil
}

// seedGroup upserts a single subagent row with all provider prompts merged
// into the provider_prompts JSONB column. The default system_prompt and model
// come from the "openai" provider (fallback for unknown providers).
func seedGroup(db *gorm.DB, group *subagentGroup) error {
	providerPrompts := make(map[string]model.ProviderPromptConfig, len(group.providers))
	for providerGroup, sf := range group.providers {
		providerPrompts[providerGroup] = model.ProviderPromptConfig{
			SystemPrompt: sf.SystemPrompt,
			Model:        sf.Model,
		}
	}

	providerPromptsJSON, err := json.Marshal(providerPrompts)
	if err != nil {
		return fmt.Errorf("marshaling provider_prompts for %s: %w", group.name, err)
	}

	// Use openai as the default system_prompt and model (fallback).
	defaultProvider := group.providers["openai"]
	if defaultProvider.SystemPrompt == "" {
		for _, sf := range group.providers {
			defaultProvider = sf
			break
		}
	}

	var description *string
	if group.description != "" {
		description = &group.description
	}

	skillsJSON := resolveSkills(db, group.skills, group.name)

	permissionsJSON := "{}"
	if len(group.permissions) > 0 {
		permBytes, err := json.Marshal(group.permissions)
		if err == nil {
			permissionsJSON = string(permBytes)
		}
	}

	now := time.Now()

	result := db.Exec(`
		INSERT INTO agents (name, description, is_system, agent_type, system_prompt, model, provider_prompts, status, tools, mcp_servers, skills, integrations, agent_config, permissions, created_at, updated_at)
		VALUES (?, ?, true, 'subagent', ?, ?, ?, 'active', '{}', '{}', ?, '{}', '{}', ?, ?, ?)
		ON CONFLICT (name) WHERE org_id IS NULL
		DO UPDATE SET description = EXCLUDED.description, system_prompt = EXCLUDED.system_prompt, model = EXCLUDED.model, provider_prompts = EXCLUDED.provider_prompts, agent_type = 'subagent', skills = EXCLUDED.skills, permissions = EXCLUDED.permissions, updated_at = EXCLUDED.updated_at
	`, group.name, description, defaultProvider.SystemPrompt, defaultProvider.Model, string(providerPromptsJSON), skillsJSON, permissionsJSON, now, now)

	if result.Error != nil {
		return fmt.Errorf("seeding subagent %s: %w", group.name, result.Error)
	}

	slog.Debug("subagent seeded", "name", group.name, "providers", len(group.providers))
	return nil
}

// resolveSkills looks up public marketplace skills by slug and returns a JSON
// string suitable for the agent.skills JSONB column. Skills not found in the
// marketplace are logged and skipped.
func resolveSkills(db *gorm.DB, slugs []string, agentName string) string {
	if len(slugs) == 0 {
		return "{}"
	}

	type skillRow struct {
		ID   string
		Slug string
	}

	var rows []skillRow
	if err := db.Raw(`SELECT id, slug FROM skills WHERE slug IN ? AND org_id IS NULL AND status = 'published'`, slugs).Scan(&rows).Error; err != nil {
		slog.Warn("failed to resolve skills for subagent", "agent", agentName, "error", err)
		return "{}"
	}

	found := make(map[string]string, len(rows))
	for _, row := range rows {
		found[row.Slug] = row.ID
	}

	skillsMap := make(map[string]any, len(slugs))
	for _, slug := range slugs {
		skillID, ok := found[slug]
		if !ok {
			slog.Warn("skill not found in marketplace, skipping", "slug", slug, "agent", agentName)
			continue
		}
		skillsMap[slug] = map[string]string{
			"skill_id": skillID,
			"slug":     slug,
		}
	}

	if len(skillsMap) == 0 {
		return "{}"
	}

	raw, err := json.Marshal(skillsMap)
	if err != nil {
		slog.Warn("failed to marshal skills for subagent", "agent", agentName, "error", err)
		return "{}"
	}

	return string(raw)
}
