package systemagents

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

// Embed all YAML files: <agent-type>/<provider-group>.yaml
//
//go:embed */*.yaml
var agentsFS embed.FS

// agentFile is the structure of a single YAML definition file.
type agentFile struct {
	Model        string `yaml:"model"`
	SystemPrompt string `yaml:"system_prompt"`
}

// Seed walks internal/system-agents/<type>/<provider-group>.yaml
// and upserts each system agent into the DB.
// Safe for concurrent calls from multiple server instances.
func Seed(db *gorm.DB) error {
	typeDirs, err := agentsFS.ReadDir(".")
	if err != nil {
		return fmt.Errorf("reading system-agents root: %w", err)
	}

	for _, typeDir := range typeDirs {
		if !typeDir.IsDir() {
			continue
		}
		agentType := typeDir.Name()

		files, err := agentsFS.ReadDir(agentType)
		if err != nil {
			return fmt.Errorf("reading %s dir: %w", agentType, err)
		}

		for _, file := range files {
			if file.IsDir() || !strings.HasSuffix(file.Name(), ".yaml") {
				continue
			}

			providerGroup := strings.TrimSuffix(file.Name(), ".yaml")

			if err := seedAgent(db, agentType, providerGroup, filepath.Join(agentType, file.Name())); err != nil {
				return err
			}
		}
	}

	return nil
}

func seedAgent(db *gorm.DB, agentType, providerGroup, path string) error {
	data, err := agentsFS.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}

	var af agentFile
	if err := yaml.Unmarshal(data, &af); err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}

	if af.SystemPrompt == "" {
		return fmt.Errorf("%s: system_prompt is required", path)
	}
	if af.Model == "" {
		return fmt.Errorf("%s: model is required", path)
	}

	name := fmt.Sprintf("%s-%s", agentType, providerGroup)

	// Forge agent config is set by ForgeAgentConfig() based on agent type, not YAML.
	forgeConfig := ForgeAgentConfig(agentType)

	now := time.Now()
	result := db.Exec(`
		INSERT INTO agents (name, is_system, provider_group, system_prompt, model, sandbox_type, status, tools, mcp_servers, skills, integrations, agent_config, permissions, created_at, updated_at)
		VALUES (?, true, ?, ?, ?, 'shared', 'active', ?, '{}', '{}', '{}', ?, ?, ?, ?)
		ON CONFLICT (name) WHERE org_id IS NULL
		DO UPDATE SET system_prompt = EXCLUDED.system_prompt, model = EXCLUDED.model, provider_group = EXCLUDED.provider_group,
		             permissions = EXCLUDED.permissions, agent_config = EXCLUDED.agent_config, tools = EXCLUDED.tools, updated_at = EXCLUDED.updated_at
	`, name, providerGroup, af.SystemPrompt, af.Model, forgeConfig.ToolsJSON, forgeConfig.AgentConfigJSON, forgeConfig.PermissionsJSON, now, now)

	if result.Error != nil {
		return fmt.Errorf("seeding %s: %w", name, result.Error)
	}

	slog.Debug("system agent seeded", "name", name, "provider_group", providerGroup)
	return nil
}

// MapProviderToGroup maps a credential's provider ID to a prompt group.
// Uses the same mapping as the forge (internal/forge/prompts.go).
func MapProviderToGroup(providerID string) string {
	switch providerID {
	case "anthropic":
		return "anthropic"
	case "openai":
		return "openai"
	case "google", "google-vertex":
		return "gemini"
	case "kimi":
		return "kimi"
	case "minimax":
		return "minimax"
	case "glm":
		return "glm"
	default:
		return "openai"
	}
}
