package systemagents

import (
	"encoding/json"

	"github.com/ziraloop/ziraloop/internal/model"
)

// ForgeConfig holds the JSON strings for forge agent DB fields.
// These are set at seed time and persisted, so they survive DB reloads.
type ForgeConfig struct {
	ToolsJSON       string
	AgentConfigJSON string
	PermissionsJSON string
}

// ForgeAgentConfig returns the DB config for a forge system agent type.
// This is the single source of truth — all forge agents get their permissions,
// agent_config, and tools from here. YAML files only define model + system_prompt.
func ForgeAgentConfig(agentType string) ForgeConfig {
	// All forge agents: tool_calls_only + all built-in tools disabled.
	// They only use their MCP tool (start_forge, submit_eval_cases, etc.).
	baseConfig := ForgeConfig{
		ToolsJSON: "{}",
		AgentConfigJSON: mustJSON(map[string]any{
			"tool_calls_only": true,
			"disabled_tools":  model.BuiltInToolIDs(),
		}),
		PermissionsJSON: "{}",
	}

	switch agentType {
	case "forge-context-gatherer":
		baseConfig.PermissionsJSON = mustJSON(map[string]string{"start_forge": "require_approval"})
	case "forge-eval-designer":
		// No special permissions — submit_eval_cases has status guard in MCP handler.
	case "forge-architect":
		// Architect outputs text (system prompt in tags), not tool calls.
		// Disable built-in tools but allow text output.
		baseConfig.AgentConfigJSON = mustJSON(map[string]any{
			"disabled_tools": model.BuiltInToolIDs(),
		})
	case "forge-judge":
		// No special permissions.
		// Judge runs many concurrent eval conversations — raise the limit.
		maxConcurrentJudge := int32(10000)
		baseConfig.AgentConfigJSON = mustJSON(map[string]any{
			"tool_calls_only":              true,
			"disabled_tools":               model.BuiltInToolIDs(),
			"max_concurrent_conversations": maxConcurrentJudge,
		})
	case "forge-planner":
		// No special permissions.
	default:
		// Non-forge system agents: no tool_calls_only, no tool restrictions.
		return ForgeConfig{
			ToolsJSON:       "{}",
			AgentConfigJSON: "{}",
			PermissionsJSON: "{}",
		}
	}

	return baseConfig
}

func mustJSON(value any) string {
	bytes, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(bytes)
}
