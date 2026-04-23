package enrichment

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/usehiveloop/hiveloop/internal/mcp/catalog"
	"github.com/usehiveloop/hiveloop/internal/trigger/hiveloop"
)

func buildEnrichmentToolDefs(connections []hiveloop.ConnectionWithActions) []hiveloop.ToolDef {
	connIDs := make([]string, 0, len(connections))
	var actionDescriptions []string
	for _, conn := range connections {
		connID := conn.Connection.ID.String()
		connIDs = append(connIDs, connID)
		for actionKey, actionDef := range conn.ReadActions {
			description := actionDef.Description
			if description == "" {
				description = actionDef.DisplayName
			}
			actionDescriptions = append(actionDescriptions,
				fmt.Sprintf("  %s / %s: %s", conn.Provider, actionKey, truncateString(description, 80)))
		}
	}

	connIDsJSON, _ := json.Marshal(connIDs)
	actionsDoc := strings.Join(actionDescriptions, "\n")

	return []hiveloop.ToolDef{
		{
			Name:        "fetch",
			Description: fmt.Sprintf("Execute a read action against a connected integration. Returns the JSON response.\n\nAvailable actions:\n%s", actionsDoc),
			Parameters: json.RawMessage(fmt.Sprintf(`{
				"type": "object",
				"properties": {
					"connection_id": {"type": "string", "description": "Connection ID", "enum": %s},
					"action": {"type": "string", "description": "Action key from the connection's catalog"},
					"params": {"type": "object", "description": "Action parameters"}
				},
				"required": ["connection_id", "action", "params"]
			}`, string(connIDsJSON))),
		},
		{
			Name:        "compose",
			Description: "Write the specialist agent's first message. Call this after gathering all needed context. The message should be structured markdown summarizing the event and all fetched context.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"message": {"type": "string", "description": "Markdown message for the specialist agent"}
				},
				"required": ["message"]
			}`),
		},
	}
}

func buildUserMessage(input EnrichmentInput) string {
	var builder strings.Builder

	eventKey := input.EventType
	if input.EventAction != "" {
		eventKey = input.EventType + "." + input.EventAction
	}
	builder.WriteString(fmt.Sprintf("Event: %s (provider: %s)\n\n", eventKey, input.Provider))

	builder.WriteString("Refs extracted from the webhook payload:\n")
	for key, value := range input.Refs {
		builder.WriteString(fmt.Sprintf("  %s: %s\n", key, value))
	}

	builder.WriteString(fmt.Sprintf("\nConnections available: %d\n", len(input.Connections)))
	for _, conn := range input.Connections {
		builder.WriteString(fmt.Sprintf("  %s (ID: %s) — %d read actions\n", conn.Provider, conn.Connection.ID.String(), len(conn.ReadActions)))
	}

	return builder.String()
}

func buildFallbackMessage(input EnrichmentInput, fetchResults []fetchResultEntry) string {
	var builder strings.Builder

	eventKey := input.EventType
	if input.EventAction != "" {
		eventKey = input.EventType + "." + input.EventAction
	}
	builder.WriteString(fmt.Sprintf("## Event: %s\n\n", eventKey))

	for key, value := range input.Refs {
		builder.WriteString(fmt.Sprintf("- %s: %s\n", key, value))
	}

	if len(fetchResults) > 0 {
		builder.WriteString("\n## Fetched Context\n\n")
		for _, entry := range fetchResults {
			builder.WriteString(fmt.Sprintf("### %s\n```json\n%s\n```\n\n", entry.Action, entry.Result))
		}
	}

	return builder.String()
}
