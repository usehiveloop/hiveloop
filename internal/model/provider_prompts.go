package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// ProviderPromptConfig holds a provider-specific system prompt and model override.
type ProviderPromptConfig struct {
	SystemPrompt string `json:"system_prompt"`
	Model        string `json:"model,omitempty"`
}

// ProviderPromptsMap is a typed map stored as JSONB. It implements
// driver.Valuer and sql.Scanner so GORM can read/write it directly.
type ProviderPromptsMap map[string]ProviderPromptConfig

// validProviderGroups mirrors the groups returned by
// subagents.MapProviderToGroup. Kept here to avoid an import cycle into
// internal/sub-agents from internal/model.
var validProviderGroups = map[string]bool{
	"anthropic": true,
	"openai":    true,
	"gemini":    true,
	"kimi":      true,
	"minimax":   true,
	"glm":       true,
}

// Validate reports the first problem with the map: an unknown provider group
// key, or an entry with an empty system_prompt. Returns "" when the map is
// nil/empty or every entry is well-formed.
func (m ProviderPromptsMap) Validate() string {
	for group, config := range m {
		if !validProviderGroups[group] {
			return fmt.Sprintf("unknown provider group %q in provider_prompts", group)
		}
		if config.SystemPrompt == "" {
			return fmt.Sprintf("provider_prompts[%q]: system_prompt is required", group)
		}
	}
	return ""
}

func (m ProviderPromptsMap) Value() (driver.Value, error) {
	if m == nil {
		return "{}", nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("marshaling provider prompts: %w", err)
	}
	return string(b), nil
}

func (m *ProviderPromptsMap) Scan(value any) error {
	if value == nil {
		*m = ProviderPromptsMap{}
		return nil
	}
	var bytes []byte
	switch v := value.(type) {
	case string:
		bytes = []byte(v)
	case []byte:
		bytes = v
	default:
		return fmt.Errorf("unsupported type for ProviderPromptsMap: %T", value)
	}
	return json.Unmarshal(bytes, m)
}

// ResolveProviderConfig returns the system prompt and model for a given provider group.
//
// Resolution order:
//  1. If ProviderPrompts[providerGroup] exists with a non-empty system_prompt, use it.
//  2. Otherwise fall back to agent.SystemPrompt and agent.Model.
//
// The providerGroup parameter is the output of MapProviderToGroup (e.g. "anthropic", "openai").
func (agent *Agent) ResolveProviderConfig(providerGroup string) (systemPrompt string, modelName string) {
	systemPrompt = agent.SystemPrompt
	modelName = agent.Model

	config, ok := agent.ProviderPrompts[providerGroup]
	if !ok {
		return
	}

	if config.SystemPrompt != "" {
		systemPrompt = config.SystemPrompt
	}
	if config.Model != "" {
		modelName = config.Model
	}

	return
}

// BridgeAgentID returns the Bridge agent ID for a given provider group.
//
// For system agents (IsSystem=true): returns "{agentID}-{providerGroup}" so that
// each provider variant gets its own Bridge definition from a single DB row.
//
// For non-system agents: returns the plain agent ID string.
func (agent *Agent) BridgeAgentID(providerGroup string) string {
	if agent.IsSystem && providerGroup != "" {
		return fmt.Sprintf("%s-%s", agent.ID.String(), providerGroup)
	}
	return agent.ID.String()
}
