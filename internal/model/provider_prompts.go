package model

import (
	"encoding/json"
	"fmt"
)

// ProviderPromptConfig holds a provider-specific system prompt and model override.
type ProviderPromptConfig struct {
	SystemPrompt string `json:"system_prompt"`
	Model        string `json:"model"`
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

	if len(agent.ProviderPrompts) == 0 {
		return
	}

	raw, err := json.Marshal(agent.ProviderPrompts)
	if err != nil {
		return
	}

	var prompts map[string]ProviderPromptConfig
	if err := json.Unmarshal(raw, &prompts); err != nil {
		return
	}

	config, ok := prompts[providerGroup]
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

// ProviderPromptsMap parses the ProviderPrompts JSON field into a typed map.
// Returns an empty map if the field is nil or cannot be parsed.
func (agent *Agent) ProviderPromptsMap() map[string]ProviderPromptConfig {
	if len(agent.ProviderPrompts) == 0 {
		return nil
	}

	raw, err := json.Marshal(agent.ProviderPrompts)
	if err != nil {
		return nil
	}

	var prompts map[string]ProviderPromptConfig
	if err := json.Unmarshal(raw, &prompts); err != nil {
		return nil
	}

	return prompts
}

// SetProviderPrompts encodes a typed map into the ProviderPrompts JSON field.
func (agent *Agent) SetProviderPrompts(prompts map[string]ProviderPromptConfig) error {
	raw, err := json.Marshal(prompts)
	if err != nil {
		return fmt.Errorf("marshaling provider prompts: %w", err)
	}

	var result JSON
	if err := json.Unmarshal(raw, &result); err != nil {
		return fmt.Errorf("converting provider prompts to JSON: %w", err)
	}

	agent.ProviderPrompts = result
	return nil
}
