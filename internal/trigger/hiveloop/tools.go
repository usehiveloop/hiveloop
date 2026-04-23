package hiveloop

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/mcp/catalog"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// ToolHandler processes a single tool call from the LLM. It returns a
// text result (shown to the LLM on the next turn) and a done flag.
// When done is true, the agent loop stops and collects results.
type ToolHandler func(ctx context.Context, callID string, args json.RawMessage) (result string, done bool, err error)

// AgentSelection records one agent the LLM chose to route an event to.
type AgentSelection struct {
	AgentID  uuid.UUID `json:"agent_id"`
	Priority int       `json:"priority"`
	Reason   string    `json:"reason"`
}

// PlannedEnrichment records one context-gathering step the LLM planned.
// The executor resolves template references and executes the actual fetch.
type PlannedEnrichment struct {
	ConnectionID uuid.UUID      `json:"connection_id"`
	As           string         `json:"as"`
	Action       string         `json:"action"`
	Params       map[string]any `json:"params"`
}

// PlannedStepRegistry tracks enrichment steps planned so far in a routing
// session, for validating {{step.field}} references and uniqueness of `as`.
type PlannedStepRegistry struct {
	mu    sync.Mutex
	steps map[string]string // as → action key (for existence check)
	order []string          // insertion order
}

// NewPlannedStepRegistry creates an empty registry.
func NewPlannedStepRegistry() *PlannedStepRegistry {
	return &PlannedStepRegistry{steps: make(map[string]string)}
}

// Has returns true if a step with the given name has been planned.
func (r *PlannedStepRegistry) Has(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.steps[name]
	return ok
}

// Add registers a step. Returns false if the name is already taken.
func (r *PlannedStepRegistry) Add(name, action string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.steps[name]; ok {
		return false
	}
	r.steps[name] = action
	r.order = append(r.order, name)
	return true
}

// Names returns the planned step names in order.
func (r *PlannedStepRegistry) Names() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]string, len(r.order))
	copy(result, r.order)
	return result
}

// ConnectionWithActions pairs a connection with its provider's read actions.
type ConnectionWithActions struct {
	Connection  model.InConnection
	Provider    string
	ReadActions map[string]catalog.ActionDef // keyed by action key
}

type routeToAgentArgs struct {
	AgentID  string `json:"agent_id"`
	Priority int    `json:"priority"`
	Reason   string `json:"reason"`
}

// NewRouteToAgentHandler creates a tool handler that validates and records
// agent routing decisions. Accumulates selections in the provided slice.
func NewRouteToAgentHandler(agents []model.Agent, selections *[]AgentSelection) ToolHandler {
	agentMap := make(map[string]model.Agent, len(agents))
	for _, agent := range agents {
		agentMap[agent.ID.String()] = agent
	}

	return func(_ context.Context, _ string, raw json.RawMessage) (string, bool, error) {
		var args routeToAgentArgs
		if err := json.Unmarshal(raw, &args); err != nil {
			return "", false, fmt.Errorf("invalid arguments: %w", err)
		}

		if args.AgentID == "" {
			return "", false, fmt.Errorf("agent_id is required")
		}

		agent, ok := agentMap[args.AgentID]
		if !ok {
			var listing []string
			for _, agent := range agents {
				desc := ""
				if agent.Description != nil {
					desc = truncate(*agent.Description, 80)
				}
				listing = append(listing, fmt.Sprintf("  - %s (%s): %s", agent.ID, agent.Name, desc))
			}
			return "", false, fmt.Errorf("agent %q not found in this org. Available agents:\n%s",
				args.AgentID, strings.Join(listing, "\n"))
		}

		if args.Priority < 1 || args.Priority > 5 {
			return "", false, fmt.Errorf("priority must be 1-5, got %d", args.Priority)
		}

		parsedID, _ := uuid.Parse(args.AgentID)
		*selections = append(*selections, AgentSelection{
			AgentID:  parsedID,
			Priority: args.Priority,
			Reason:   args.Reason,
		})

		return fmt.Sprintf("✓ Routed to agent %q (%s) with priority %d.", agent.Name, args.AgentID, args.Priority), false, nil
	}
}
