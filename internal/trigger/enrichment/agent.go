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

	"github.com/usehiveloop/hiveloop/internal/mcp/catalog"
	"github.com/usehiveloop/hiveloop/internal/mcpserver"
	"github.com/usehiveloop/hiveloop/internal/nango"
	"github.com/usehiveloop/hiveloop/internal/trigger/hiveloop"
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
	Connections []hiveloop.ConnectionWithActions
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
func (agent *EnrichmentAgent) Enrich(ctx context.Context, client hiveloop.CompletionClient, modelID string, providerGroup string, input EnrichmentInput, logger *slog.Logger) (*EnrichmentResult, error) {
	started := time.Now()

	// Build connection lookup maps.
	connMap := make(map[string]hiveloop.ConnectionWithActions, len(input.Connections))
	for _, conn := range input.Connections {
		connMap[conn.Connection.ID.String()] = conn
	}

	// Log available connections and their action counts.
	for connID, conn := range connMap {
		logger.Debug("enrichment: connection available",
			"conn_id", connID,
			"provider", conn.Provider,
			"read_actions", len(conn.ReadActions),
		)
	}

	// Log refs.
	for refKey, refValue := range input.Refs {
		logger.Debug("enrichment: ref",
			"key", refKey,
			"value", refValue,
		)
	}

	// Shared state accumulated by tool handlers.
	var composedMessage string
	var fetchResults []fetchResultEntry
	fetchCount := 0

	// Build tool handlers.
	handlers := map[string]hiveloop.ToolHandler{
		"fetch":   agent.newFetchHandler(ctx, input.OrgID, connMap, &fetchResults, &fetchCount, logger),
		"compose": newComposeHandler(&composedMessage, logger),
	}

	// Build tool definitions.
	tools := buildEnrichmentToolDefs(input.Connections)

	// Build messages with provider-optimized prompt.
	systemPrompt := getEnrichmentPrompt(providerGroup)
	userMessage := buildUserMessage(input)

	logger.Debug("enrichment: system prompt selected",
		"provider_group", providerGroup,
		"prompt_bytes", len(systemPrompt),
	)
	logger.Debug("enrichment: user message built",
		"message_bytes", len(userMessage),
	)

	messages := []hiveloop.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userMessage},
	}

	for turn := 0; turn < agent.maxTurns; turn++ {
		turnStart := time.Now()
		logger.Info("enrichment: llm turn starting",
			"turn", turn+1,
			"max_turns", agent.maxTurns,
			"message_count", len(messages),
		)

		resp, err := client.ChatCompletion(ctx, hiveloop.CompletionRequest{
			Model:      modelID,
			Messages:   messages,
			Tools:      tools,
			ToolChoice: "required",
			MaxTokens:  4096,
		})
		if err != nil {
			logger.Error("enrichment: llm call failed",
				"turn", turn+1,
				"error", err,
				"latency_ms", time.Since(turnStart).Milliseconds(),
			)
			return nil, fmt.Errorf("enrichment agent turn %d: %w", turn+1, err)
		}

		llmLatency := time.Since(turnStart).Milliseconds()
		assistantMsg := resp.Message

		if len(assistantMsg.ToolCalls) == 0 {
			logger.Warn("enrichment: llm produced text instead of tool calls",
				"turn", turn+1,
				"content", truncateString(assistantMsg.Content, 200),
				"llm_latency_ms", llmLatency,
			)
			break
		}

		logger.Info("enrichment: llm turn complete",
			"turn", turn+1,
			"tool_calls", len(assistantMsg.ToolCalls),
			"llm_latency_ms", llmLatency,
		)

		// Log each tool call the LLM made.
		for callIndex, toolCall := range assistantMsg.ToolCalls {
			logger.Info("enrichment: tool call",
				"turn", turn+1,
				"call_index", callIndex,
				"tool", toolCall.Name,
				"call_id", toolCall.ID,
				"arguments", truncateString(toolCall.Arguments, 500),
			)
		}

		messages = append(messages, assistantMsg)

		for _, toolCall := range assistantMsg.ToolCalls {
			handler, ok := handlers[toolCall.Name]
			if !ok {
				logger.Warn("enrichment: unknown tool called",
					"tool", toolCall.Name,
					"call_id", toolCall.ID,
				)
				messages = append(messages, hiveloop.Message{
					Role:       "tool",
					ToolCallID: toolCall.ID,
					Name:       toolCall.Name,
					Content:    fmt.Sprintf("Unknown tool %q. Available: fetch, compose.", toolCall.Name),
				})
				continue
			}

			handlerStart := time.Now()
			result, done, handlerErr := handler(ctx, toolCall.ID, json.RawMessage(toolCall.Arguments))
			handlerLatency := time.Since(handlerStart).Milliseconds()

			if handlerErr != nil {
				logger.Warn("enrichment: tool handler error",
					"tool", toolCall.Name,
					"call_id", toolCall.ID,
					"error", handlerErr,
					"handler_latency_ms", handlerLatency,
				)
				messages = append(messages, hiveloop.Message{
					Role:       "tool",
					ToolCallID: toolCall.ID,
					Name:       toolCall.Name,
					Content:    fmt.Sprintf("Error: %s", handlerErr.Error()),
				})
				continue
			}

			logger.Info("enrichment: tool handler success",
				"tool", toolCall.Name,
				"call_id", toolCall.ID,
				"result_bytes", len(result),
				"done", done,
				"handler_latency_ms", handlerLatency,
			)

			messages = append(messages, hiveloop.Message{
				Role:       "tool",
				ToolCallID: toolCall.ID,
				Name:       toolCall.Name,
				Content:    result,
			})

			if done {
				totalLatency := int(time.Since(started).Milliseconds())
				logger.Info("enrichment: finished via compose",
					"total_turns", turn+1,
					"total_fetches", fetchCount,
					"composed_message_bytes", len(composedMessage),
					"total_latency_ms", totalLatency,
				)
				return &EnrichmentResult{
					ComposedMessage: composedMessage,
					FetchCount:      fetchCount,
					TurnCount:       turn + 1,
					LatencyMs:       totalLatency,
				}, nil
			}
		}
	}

	// Max turns reached without compose — fallback.
	totalLatency := int(time.Since(started).Milliseconds())
	if composedMessage == "" {
		composedMessage = buildFallbackMessage(input, fetchResults)
		logger.Warn("enrichment: max turns reached, using fallback compose",
			"max_turns", agent.maxTurns,
			"total_fetches", fetchCount,
			"fallback_message_bytes", len(composedMessage),
			"total_latency_ms", totalLatency,
		)
	}

	return &EnrichmentResult{
		ComposedMessage: composedMessage,
		FetchCount:      fetchCount,
		TurnCount:       agent.maxTurns,
		LatencyMs:       totalLatency,
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
	connMap map[string]hiveloop.ConnectionWithActions,
	fetchResults *[]fetchResultEntry,
	fetchCount *int,
	logger *slog.Logger,
) hiveloop.ToolHandler {
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
			logger.Warn("enrichment: fetch connection not found",
				"requested_conn_id", args.ConnectionID,
				"available", strings.Join(available, ", "),
			)
			return "", false, fmt.Errorf("connection %q not found. Available: %s", args.ConnectionID, strings.Join(available, ", "))
		}

		actionDef, actionExists := conn.ReadActions[args.Action]
		if !actionExists {
			var available []string
			for actionKey := range conn.ReadActions {
				available = append(available, actionKey)
			}
			logger.Warn("enrichment: fetch action not found",
				"provider", conn.Provider,
				"requested_action", args.Action,
				"available", strings.Join(available, ", "),
			)
			return "", false, fmt.Errorf("action %q not found for %s. Available: %s", args.Action, conn.Provider, strings.Join(available, ", "))
		}

		paramsJSON, _ := json.Marshal(args.Params)
		logger.Info("enrichment: fetch executing",
			"provider", conn.Provider,
			"action", args.Action,
			"conn_id", args.ConnectionID,
			"params", string(paramsJSON),
		)

		providerCfgKey := fmt.Sprintf("%s_%s", orgID.String(), conn.Connection.InIntegration.UniqueKey)
		nangoConnID := conn.Connection.NangoConnectionID

		fetchStart := time.Now()
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
		fetchLatency := time.Since(fetchStart).Milliseconds()

		if err != nil {
			logger.Warn("enrichment: fetch failed",
				"provider", conn.Provider,
				"action", args.Action,
				"error", err,
				"fetch_latency_ms", fetchLatency,
			)
			return fmt.Sprintf("Fetch failed: %s", err.Error()), false, nil
		}

		resultJSON, _ := json.Marshal(result)
		resultStr := truncateString(string(resultJSON), 4000)

		logger.Info("enrichment: fetch success",
			"provider", conn.Provider,
			"action", args.Action,
			"response_bytes", len(resultJSON),
			"truncated_bytes", len(resultStr),
			"fetch_latency_ms", fetchLatency,
		)

		*fetchResults = append(*fetchResults, fetchResultEntry{Action: args.Action, Result: resultStr})
		*fetchCount++

		return resultStr, false, nil
	}
}

func newComposeHandler(composedMessage *string, logger *slog.Logger) hiveloop.ToolHandler {
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

		logger.Info("enrichment: compose called",
			"message_bytes", len(args.Message),
			"message_preview", truncateString(args.Message, 300),
		)

		return "Message composed.", true, nil
	}
}

// --------------------------------------------------------------------------
// Tool definitions
// --------------------------------------------------------------------------

func buildEnrichmentToolDefs(connections []hiveloop.ConnectionWithActions) []hiveloop.ToolDef {
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
