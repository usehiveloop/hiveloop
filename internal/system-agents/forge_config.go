package systemagents

import "encoding/json"

// ForgeConfig holds the JSON strings for forge agent DB fields.
// These are set at seed time and persisted, so they survive DB reloads.
type ForgeConfig struct {
	ToolsJSON       string
	AgentConfigJSON string
	PermissionsJSON string
}

// allBuiltInTools is the complete list of Bridge built-in tool names.
// Forge system agents disable ALL of them — they only use MCP tools.
var allBuiltInTools = []string{
	// Filesystem
	"Read", "write", "edit", "multiedit", "apply_patch", "Glob", "Grep", "LS",
	// Shell
	"bash",
	// Web
	"web_fetch", "web_search", "web_crawl", "web_get_links", "web_screenshot", "web_transform",
	// Agent orchestration
	"agent", "sub_agent", "parallel_agent", "batch", "join",
	// Task management
	"todowrite", "todoread",
	// Journal
	"journal_write", "journal_read",
	// Code intelligence
	"lsp", "skill",
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
			"disabled_tools":  allBuiltInTools,
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
			"disabled_tools": allBuiltInTools,
		})
	case "forge-judge":
		// No special permissions.
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
