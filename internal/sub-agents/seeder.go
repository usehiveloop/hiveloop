package subagents

import (
	"embed"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
	"gorm.io/gorm"
)

//go:embed */*.yaml
var subagentsFS embed.FS

type subagentFile struct {
	Name         string `yaml:"name"`
	Description  string `yaml:"description"`
	Model        string `yaml:"model"`
	SystemPrompt string `yaml:"system_prompt"`
}

// Seed walks internal/sub-agents/<type>/<provider-group>.yaml and upserts
// each as a system subagent (agent_type='subagent', is_system=true, org_id=NULL).
func Seed(db *gorm.DB) error {
	typeDirs, err := subagentsFS.ReadDir(".")
	if err != nil {
		return fmt.Errorf("reading sub-agents root: %w", err)
	}

	for _, typeDir := range typeDirs {
		if !typeDir.IsDir() {
			continue
		}
		subagentType := typeDir.Name()

		files, err := subagentsFS.ReadDir(subagentType)
		if err != nil {
			return fmt.Errorf("reading %s dir: %w", subagentType, err)
		}

		for _, file := range files {
			if file.IsDir() || !strings.HasSuffix(file.Name(), ".yaml") {
				continue
			}

			providerGroup := strings.TrimSuffix(file.Name(), ".yaml")

			if err := seedSubagent(db, subagentType, providerGroup, filepath.Join(subagentType, file.Name())); err != nil {
				return err
			}
		}
	}

	return nil
}

func seedSubagent(db *gorm.DB, subagentType, providerGroup, path string) error {
	data, err := subagentsFS.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}

	var sf subagentFile
	if err := yaml.Unmarshal(data, &sf); err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}

	if sf.SystemPrompt == "" {
		return fmt.Errorf("%s: system_prompt is required", path)
	}
	if sf.Model == "" {
		return fmt.Errorf("%s: model is required", path)
	}

	name := fmt.Sprintf("%s-%s", subagentType, providerGroup)
	if sf.Name != "" {
		name = fmt.Sprintf("%s-%s", sf.Name, providerGroup)
	}

	now := time.Now()

	var description *string
	if sf.Description != "" {
		description = &sf.Description
	}

	result := db.Exec(`
		INSERT INTO agents (name, description, is_system, agent_type, provider_group, system_prompt, model, sandbox_type, status, tools, mcp_servers, skills, integrations, agent_config, permissions, created_at, updated_at)
		VALUES (?, ?, true, 'subagent', ?, ?, ?, '', 'active', '{}', '{}', '{}', '{}', '{}', '{}', ?, ?)
		ON CONFLICT (name) WHERE org_id IS NULL
		DO UPDATE SET description = EXCLUDED.description, system_prompt = EXCLUDED.system_prompt, model = EXCLUDED.model, provider_group = EXCLUDED.provider_group, agent_type = 'subagent', updated_at = EXCLUDED.updated_at
	`, name, description, providerGroup, sf.SystemPrompt, sf.Model, now, now)

	if result.Error != nil {
		return fmt.Errorf("seeding subagent %s: %w", name, result.Error)
	}

	slog.Debug("subagent seeded", "name", name, "provider_group", providerGroup)
	return nil
}
