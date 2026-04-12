package zira

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/ziraloop/ziraloop/internal/model"
)

// RoutingResult holds the output of a routing session — which agents were
// selected, what enrichment steps were planned, and how many LLM turns it took.
type RoutingResult struct {
	SelectedAgents []AgentSelection
	EnrichmentPlan []PlannedEnrichment
	TurnCount      int
	LatencyMs      int
}

// RouterAgent runs a multi-turn tool-calling LLM session that plans routing
// and enrichment. It calls the LLM with tool_choice="required" so the LLM
// MUST call tools on every turn (no text output). The loop ends when the
// LLM calls finalize() or max_turns is reached.
type RouterAgent struct {
	client   CompletionClient
	model    string
	maxTurns int
}

// NewRouterAgent creates a routing agent with the given LLM client and model.
func NewRouterAgent(client CompletionClient, modelID string, maxTurns int) *RouterAgent {
	if maxTurns <= 0 {
		maxTurns = 10
	}
	return &RouterAgent{client: client, model: modelID, maxTurns: maxTurns}
}

// Route runs the routing session. It builds tool definitions from the provided
// agents and connections, then enters the agent loop until finalize() is called
// or max_turns is reached.
func (agent *RouterAgent) Route(
	ctx context.Context,
	systemPrompt string,
	userMessage string,
	orgAgents []model.Agent,
	connections []ConnectionWithActions,
) (*RoutingResult, error) {
	started := time.Now()

	// Shared state accumulated by tool handlers across turns.
	var selections []AgentSelection
	var enrichments []PlannedEnrichment
	planned := NewPlannedStepRegistry()

	// Build tool handlers.
	handlers := map[string]ToolHandler{
		"route_to_agent":  NewRouteToAgentHandler(orgAgents, &selections),
		"plan_enrichment": NewPlanEnrichmentHandler(connections, nil, planned, &enrichments),
		"finalize":        NewFinalizeHandler(),
	}

	// Build tool definitions for the LLM.
	tools := buildToolDefs(orgAgents, connections)

	// Initial messages.
	messages := []Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userMessage},
	}

	for turn := 0; turn < agent.maxTurns; turn++ {
		resp, err := agent.client.ChatCompletion(ctx, CompletionRequest{
			Model:      agent.model,
			Messages:   messages,
			Tools:      tools,
			ToolChoice: "required",
			MaxTokens:  4096,
		})
		if err != nil {
			return nil, fmt.Errorf("routing agent turn %d: %w", turn+1, err)
		}

		assistantMsg := resp.Message
		if len(assistantMsg.ToolCalls) == 0 {
			slog.Warn("routing agent produced text instead of tool calls",
				"turn", turn+1, "content", truncate(assistantMsg.Content, 100))
			break
		}

		// Append assistant message to history.
		messages = append(messages, assistantMsg)

		// Execute each tool call.
		for _, toolCall := range assistantMsg.ToolCalls {
			handler, ok := handlers[toolCall.Name]
			if !ok {
				messages = append(messages, Message{
					Role:       "tool",
					ToolCallID: toolCall.ID,
					Name:       toolCall.Name,
					Content:    fmt.Sprintf("Unknown tool %q. Available: route_to_agent, plan_enrichment, finalize.", toolCall.Name),
				})
				continue
			}

			result, done, handlerErr := handler(ctx, toolCall.ID, json.RawMessage(toolCall.Arguments))
			if handlerErr != nil {
				messages = append(messages, Message{
					Role:       "tool",
					ToolCallID: toolCall.ID,
					Name:       toolCall.Name,
					Content:    fmt.Sprintf("Error: %s", handlerErr.Error()),
				})
				continue
			}

			messages = append(messages, Message{
				Role:       "tool",
				ToolCallID: toolCall.ID,
				Name:       toolCall.Name,
				Content:    result,
			})

			if done {
				return &RoutingResult{
					SelectedAgents: selections,
					EnrichmentPlan: enrichments,
					TurnCount:      turn + 1,
					LatencyMs:      int(time.Since(started).Milliseconds()),
				}, nil
			}
		}
	}

	// Max turns reached — return whatever was collected.
	return &RoutingResult{
		SelectedAgents: selections,
		EnrichmentPlan: enrichments,
		TurnCount:      agent.maxTurns,
		LatencyMs:      int(time.Since(started).Milliseconds()),
	}, nil
}

// buildToolDefs constructs the JSON Schema tool definitions passed to the LLM.
func buildToolDefs(agents []model.Agent, connections []ConnectionWithActions) []ToolDef {
	// route_to_agent — enum of valid agent IDs.
	agentIDs := make([]string, len(agents))
	for index, agent := range agents {
		agentIDs[index] = agent.ID.String()
	}
	agentIDsJSON, _ := json.Marshal(agentIDs)

	// plan_enrichment — enum of valid connection IDs.
	connIDs := make([]string, len(connections))
	for index, conn := range connections {
		connIDs[index] = conn.Connection.ID.String()
	}
	connIDsJSON, _ := json.Marshal(connIDs)

	return []ToolDef{
		{
			Name:        "route_to_agent",
			Description: "Select an agent to handle this event. Call multiple times for multi-agent dispatch.",
			Parameters: json.RawMessage(fmt.Sprintf(`{
				"type": "object",
				"properties": {
					"agent_id": {"type": "string", "description": "Agent ID from the catalog", "enum": %s},
					"priority": {"type": "integer", "description": "1 = highest priority (runs first), up to 5", "minimum": 1, "maximum": 5},
					"reason": {"type": "string", "description": "Brief explanation of why this agent was selected"}
				},
				"required": ["agent_id", "priority", "reason"]
			}`, string(agentIDsJSON))),
		},
		{
			Name:        "plan_enrichment",
			Description: "Plan a context-gathering step. The executor will run it later. Chain steps using {{step_name.field}} in params.",
			Parameters: json.RawMessage(fmt.Sprintf(`{
				"type": "object",
				"properties": {
					"connection_id": {"type": "string", "description": "Connection ID to fetch from", "enum": %s},
					"action": {"type": "string", "description": "Read action key from the connection's catalog"},
					"as": {"type": "string", "description": "Unique step name for referencing results in later steps"},
					"params": {"type": "object", "description": "Action parameters. Use $refs.x for webhook values, {{step.field}} for chaining."}
				},
				"required": ["connection_id", "action", "as", "params"]
			}`, string(connIDsJSON))),
		},
		{
			Name:        "finalize",
			Description: "Signal that routing is complete. Call after all route_to_agent and plan_enrichment calls.",
			Parameters:  json.RawMessage(`{"type": "object", "properties": {}}`),
		},
	}
}
