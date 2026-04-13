// Package enrichment implements a context-gathering agent that runs between
// webhook trigger matching and specialist agent invocation. It fetches data
// from connected integrations via the Nango proxy, chains cross-platform
// lookups, and composes a rich first message for the specialist agent.
package enrichment

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/ziraloop/ziraloop/internal/mcp/catalog"
	"github.com/ziraloop/ziraloop/internal/mcpserver"
	"github.com/ziraloop/ziraloop/internal/nango"
	"github.com/ziraloop/ziraloop/internal/trigger/zira"
)

// EnrichmentAgent gathers context for webhook-triggered specialist agents.
// It calls fetch() to execute real API calls via the Nango proxy, sees results
// in real-time, chains cross-platform lookups, and composes the specialist's
// first message via compose().
type EnrichmentAgent struct {
	nangoClient *nango.Client
	catalog     *catalog.Catalog
	maxTurns    int
}

// EnrichmentInput is the context the enrichment agent works with.
type EnrichmentInput struct {
	Provider    string
	EventType   string
	EventAction string
	OrgID       uuid.UUID
	Refs        map[string]string
	Connections []zira.ConnectionWithActions
}

// EnrichmentResult is the output of the enrichment agent.
type EnrichmentResult struct {
	ComposedMessage string
	FetchCount      int
	TurnCount       int
	LatencyMs       int
}

// NewEnrichmentAgent creates an enrichment agent with the given dependencies.
func NewEnrichmentAgent(nangoClient *nango.Client, actionsCatalog *catalog.Catalog, maxTurns int) *EnrichmentAgent {
	if maxTurns <= 0 {
		maxTurns = 6
	}
	return &EnrichmentAgent{nangoClient: nangoClient, catalog: actionsCatalog, maxTurns: maxTurns}
}

// Enrich runs the enrichment loop. The CompletionClient, model, and provider
// group are passed per-call because they are resolved from the org's
// credentials at runtime. The provider group ("anthropic", "openai", "gemini",
// etc.) selects the provider-optimized system prompt.
func (agent *EnrichmentAgent) Enrich(ctx context.Context, client zira.CompletionClient, modelID string, providerGroup string, input EnrichmentInput) (*EnrichmentResult, error) {
	started := time.Now()

	// Build connection lookup maps.
	connMap := make(map[string]zira.ConnectionWithActions, len(input.Connections))
	for _, conn := range input.Connections {
		connMap[conn.Connection.ID.String()] = conn
	}

	// Shared state accumulated by tool handlers.
	var composedMessage string
	var fetchResults []fetchResultEntry
	fetchCount := 0

	// Build tool handlers.
	handlers := map[string]zira.ToolHandler{
		"fetch": agent.newFetchHandler(ctx, input.OrgID, connMap, &fetchResults, &fetchCount),
		"compose": newComposeHandler(&composedMessage),
	}

	// Build tool definitions.
	tools := buildEnrichmentToolDefs(input.Connections)

	// Build messages with provider-optimized prompt.
	messages := []zira.Message{
		{Role: "system", Content: getEnrichmentPrompt(providerGroup)},
		{Role: "user", Content: buildUserMessage(input)},
	}

	for turn := 0; turn < agent.maxTurns; turn++ {
		resp, err := client.ChatCompletion(ctx, zira.CompletionRequest{
			Model:      modelID,
			Messages:   messages,
			Tools:      tools,
			ToolChoice: "required",
			MaxTokens:  4096,
		})
		if err != nil {
			return nil, fmt.Errorf("enrichment agent turn %d: %w", turn+1, err)
		}

		assistantMsg := resp.Message
		if len(assistantMsg.ToolCalls) == 0 {
			slog.Warn("enrichment agent produced text instead of tool calls",
				"turn", turn+1, "content", truncateString(assistantMsg.Content, 100))
			break
		}

		messages = append(messages, assistantMsg)

		for _, toolCall := range assistantMsg.ToolCalls {
			handler, ok := handlers[toolCall.Name]
			if !ok {
				messages = append(messages, zira.Message{
					Role:       "tool",
					ToolCallID: toolCall.ID,
					Name:       toolCall.Name,
					Content:    fmt.Sprintf("Unknown tool %q. Available: fetch, compose.", toolCall.Name),
				})
				continue
			}

			result, done, handlerErr := handler(ctx, toolCall.ID, json.RawMessage(toolCall.Arguments))
			if handlerErr != nil {
				messages = append(messages, zira.Message{
					Role:       "tool",
					ToolCallID: toolCall.ID,
					Name:       toolCall.Name,
					Content:    fmt.Sprintf("Error: %s", handlerErr.Error()),
				})
				continue
			}

			messages = append(messages, zira.Message{
				Role:       "tool",
				ToolCallID: toolCall.ID,
				Name:       toolCall.Name,
				Content:    result,
			})

			if done {
				return &EnrichmentResult{
					ComposedMessage: composedMessage,
					FetchCount:      fetchCount,
					TurnCount:       turn + 1,
					LatencyMs:       int(time.Since(started).Milliseconds()),
				}, nil
			}
		}
	}

	// Max turns reached without compose — fallback.
	if composedMessage == "" {
		composedMessage = buildFallbackMessage(input, fetchResults)
	}

	return &EnrichmentResult{
		ComposedMessage: composedMessage,
		FetchCount:      fetchCount,
		TurnCount:       agent.maxTurns,
		LatencyMs:       int(time.Since(started).Milliseconds()),
	}, nil
}

// --------------------------------------------------------------------------
// Tool handlers
// --------------------------------------------------------------------------

type fetchResultEntry struct {
	Action string
	Result string
}

func (agent *EnrichmentAgent) newFetchHandler(
	ctx context.Context,
	orgID uuid.UUID,
	connMap map[string]zira.ConnectionWithActions,
	fetchResults *[]fetchResultEntry,
	fetchCount *int,
) zira.ToolHandler {
	return func(_ context.Context, _ string, raw json.RawMessage) (string, bool, error) {
		var args struct {
			ConnectionID string         `json:"connection_id"`
			Action       string         `json:"action"`
			Params       map[string]any `json:"params"`
		}
		if err := json.Unmarshal(raw, &args); err != nil {
			return "", false, fmt.Errorf("invalid arguments: %w", err)
		}

		conn, ok := connMap[args.ConnectionID]
		if !ok {
			var available []string
			for connID, connEntry := range connMap {
				available = append(available, fmt.Sprintf("%s (%s)", connID, connEntry.Provider))
			}
			return "", false, fmt.Errorf("connection %q not found. Available: %s", args.ConnectionID, strings.Join(available, ", "))
		}

		actionDef, actionExists := conn.ReadActions[args.Action]
		if !actionExists {
			var available []string
			for actionKey := range conn.ReadActions {
				available = append(available, actionKey)
			}
			return "", false, fmt.Errorf("action %q not found for %s. Available: %s", args.Action, conn.Provider, strings.Join(available, ", "))
		}

		providerCfgKey := fmt.Sprintf("%s_%s", orgID.String(), conn.Connection.Integration.UniqueKey)
		nangoConnID := conn.Connection.NangoConnectionID

		result, err := mcpserver.ExecuteAction(
			ctx,
			agent.nangoClient,
			conn.Provider,
			providerCfgKey,
			nangoConnID,
			&actionDef,
			args.Params,
			nil, // no resource access restrictions for enrichment
		)
		if err != nil {
			return fmt.Sprintf("Fetch failed: %s", err.Error()), false, nil
		}

		resultJSON, _ := json.Marshal(result)
		resultStr := truncateString(string(resultJSON), 4000)

		*fetchResults = append(*fetchResults, fetchResultEntry{Action: args.Action, Result: resultStr})
		*fetchCount++

		return resultStr, false, nil
	}
}

func newComposeHandler(composedMessage *string) zira.ToolHandler {
	return func(_ context.Context, _ string, raw json.RawMessage) (string, bool, error) {
		var args struct {
			Message string `json:"message"`
		}
		if err := json.Unmarshal(raw, &args); err != nil {
			return "", false, fmt.Errorf("invalid arguments: %w", err)
		}
		if args.Message == "" {
			return "", false, fmt.Errorf("message is required")
		}
		*composedMessage = args.Message
		return "Message composed.", true, nil
	}
}

// --------------------------------------------------------------------------
// Tool definitions
// --------------------------------------------------------------------------

func buildEnrichmentToolDefs(connections []zira.ConnectionWithActions) []zira.ToolDef {
	// Build connection enum and action descriptions.
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

	return []zira.ToolDef{
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

// --------------------------------------------------------------------------
// Message builders
// --------------------------------------------------------------------------

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

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

func truncateString(value string, maxLen int) string {
	if len(value) <= maxLen {
		return value
	}
	return value[:maxLen-3] + "..."
}
