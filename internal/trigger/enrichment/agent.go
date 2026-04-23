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
	"time"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/mcp/catalog"
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

	connMap := make(map[string]hiveloop.ConnectionWithActions, len(input.Connections))
	for _, conn := range input.Connections {
		connMap[conn.Connection.ID.String()] = conn
	}

	for connID, conn := range connMap {
		logger.Debug("enrichment: connection available",
			"conn_id", connID,
			"provider", conn.Provider,
			"read_actions", len(conn.ReadActions),
		)
	}

	for refKey, refValue := range input.Refs {
		logger.Debug("enrichment: ref",
			"key", refKey,
			"value", refValue,
		)
	}

	var composedMessage string
	var fetchResults []fetchResultEntry
	fetchCount := 0

	handlers := map[string]hiveloop.ToolHandler{
		"fetch":   agent.newFetchHandler(ctx, input.OrgID, connMap, &fetchResults, &fetchCount, logger),
		"compose": newComposeHandler(&composedMessage, logger),
	}

	tools := buildEnrichmentToolDefs(input.Connections)

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
