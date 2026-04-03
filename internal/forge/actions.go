package forge

import (
	"encoding/json"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/llmvault/llmvault/internal/mcp/catalog"
	"github.com/llmvault/llmvault/internal/model"
)

// ResolvedAction is a fully resolved action with its catalog schema, ready
// to be injected into the eval designer's context for accurate mock generation.
type ResolvedAction struct {
	Provider    string          `json:"provider"`     // e.g. "slack", "github"
	ActionKey   string          `json:"action_key"`   // e.g. "send_message"
	ToolName    string          `json:"tool_name"`    // e.g. "slack_send_message"
	DisplayName string          `json:"display_name"`
	Description string          `json:"description"`
	Access      string          `json:"access"`       // "read" or "write"
	Parameters  json.RawMessage `json:"parameters"`   // JSON Schema for input
}

// resolveAgentActions loads the agent's configured integrations, looks up
// each action in the catalog, and returns fully resolved action definitions.
// This gives the eval designer exact schemas for generating accurate tool mocks.
func resolveAgentActions(db *gorm.DB, cat *catalog.Catalog, agent *model.Agent) ([]ResolvedAction, error) {
	if agent.Integrations == nil || len(agent.Integrations) == 0 {
		return nil, nil
	}

	// The agent's Integrations field contains scopes: connection_id → actions.
	// Parse it to extract connection IDs and their allowed actions.
	scopes, err := parseAgentIntegrationScopes(agent.Integrations)
	if err != nil {
		return nil, fmt.Errorf("parsing agent integrations: %w", err)
	}

	var resolved []ResolvedAction

	for _, scope := range scopes {
		if scope.ConnectionID == "" || len(scope.Actions) == 0 {
			continue
		}

		// Load connection to get the provider name.
		var conn model.Connection
		if err := db.Preload("Integration").
			Where("id = ? AND revoked_at IS NULL", scope.ConnectionID).
			First(&conn).Error; err != nil {
			continue // skip if connection not found
		}

		if conn.Integration.DeletedAt != nil {
			continue // skip deleted integrations
		}

		provider := conn.Integration.Provider

		// Look up each action in the catalog.
		for _, actionKey := range scope.Actions {
			actionDef, ok := cat.GetAction(provider, actionKey)
			if !ok {
				continue // skip unknown actions
			}

			resolved = append(resolved, ResolvedAction{
				Provider:    provider,
				ActionKey:   actionKey,
				ToolName:    provider + "_" + actionKey,
				DisplayName: actionDef.DisplayName,
				Description: actionDef.Description,
				Access:      actionDef.Access,
				Parameters:  actionDef.Parameters,
			})
		}
	}

	return resolved, nil
}

// integrationScope mirrors the TokenScope structure for parsing agent.Integrations.
type integrationScope struct {
	ConnectionID string   `json:"connection_id"`
	Actions      []string `json:"actions"`
}

// parseAgentIntegrationScopes extracts integration scope definitions from the
// agent's Integrations JSON field. Supports both array and object formats.
func parseAgentIntegrationScopes(integrations model.JSON) ([]integrationScope, error) {
	raw, err := json.Marshal(integrations)
	if err != nil {
		return nil, err
	}

	// Try as array of scopes first (TokenScope format).
	var scopes []integrationScope
	if err := json.Unmarshal(raw, &scopes); err == nil && len(scopes) > 0 {
		return scopes, nil
	}

	// Try as a map of connection_id → actions.
	var scopeMap map[string][]string
	if err := json.Unmarshal(raw, &scopeMap); err == nil {
		for connID, actions := range scopeMap {
			scopes = append(scopes, integrationScope{
				ConnectionID: connID,
				Actions:      actions,
			})
		}
		return scopes, nil
	}

	return nil, nil
}

// formatActionsForEvalDesigner produces a human-readable summary of resolved
// actions with their full JSON Schema parameters, suitable for injection into
// the eval designer's system prompt or message.
func formatActionsForEvalDesigner(actions []ResolvedAction) string {
	if len(actions) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## Available Integration Actions (with JSON Schemas)\n\n")
	sb.WriteString("The agent has the following integration tools. Generate tool mocks that match these schemas exactly.\n\n")

	for _, a := range actions {
		sb.WriteString(fmt.Sprintf("### `%s` (%s)\n", a.ToolName, a.Access))
		sb.WriteString(fmt.Sprintf("**Description:** %s\n", a.Description))
		sb.WriteString("**Input Parameters Schema:**\n```json\n")
		if len(a.Parameters) > 0 {
			// Pretty-print the JSON schema.
			var pretty json.RawMessage
			if json.Unmarshal(a.Parameters, &pretty) == nil {
				prettyBytes, _ := json.MarshalIndent(pretty, "", "  ")
				sb.Write(prettyBytes)
			} else {
				sb.Write(a.Parameters)
			}
		} else {
			sb.WriteString("{}")
		}
		sb.WriteString("\n```\n\n")
	}

	sb.WriteString("When generating tool_mocks for these actions:\n")
	sb.WriteString("- Use the tool_name as the key (e.g., `slack_send_message`)\n")
	sb.WriteString("- The `match` field should use parameter names from the schema\n")
	sb.WriteString("- The `response` field should contain realistic API response data\n")
	sb.WriteString("- Include error responses for tool_error category evals\n")

	return sb.String()
}

// validateEvalMocks checks that generated mock tool_mocks reference valid
// tool names from the resolved actions. Returns a list of warnings for
// unknown tools (non-fatal — the eval can still run).
func validateEvalMocks(evalCases []EvalCase, actions []ResolvedAction) []string {
	validTools := make(map[string]bool, len(actions))
	for _, a := range actions {
		validTools[a.ToolName] = true
	}

	var warnings []string
	for _, ec := range evalCases {
		for toolName := range ec.ToolMocks {
			if !validTools[toolName] && len(actions) > 0 {
				warnings = append(warnings, fmt.Sprintf(
					"eval %q references unknown tool %q (not in agent's integrations)",
					ec.Name, toolName,
				))
			}
		}
	}
	return warnings
}
