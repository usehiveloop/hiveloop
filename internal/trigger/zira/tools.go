package zira

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/google/uuid"

	"github.com/ziraloop/ziraloop/internal/mcp/catalog"
	"github.com/ziraloop/ziraloop/internal/model"
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
	mu     sync.Mutex
	steps  map[string]string // as → action key (for existence check)
	order  []string          // insertion order
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

// --------------------------------------------------------------------------
// route_to_agent tool
// --------------------------------------------------------------------------

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

// --------------------------------------------------------------------------
// plan_enrichment tool
// --------------------------------------------------------------------------

type planEnrichmentArgs struct {
	ConnectionID string         `json:"connection_id"`
	Action       string         `json:"action"`
	As           string         `json:"as"`
	Params       map[string]any `json:"params"`
}

// templateRefPattern matches {{step_name.field.path}} references in param values.
var templateRefPattern = regexp.MustCompile(`\{\{(\w+)\.\w[\w.]*\}\}`)

// NewPlanEnrichmentHandler creates a tool handler that validates enrichment
// plans without executing them. Each call checks connection existence, action
// validity (read-only), param completeness, and {{step.field}} ordering.
// On success, registers the step and returns the action's response schema
// so the LLM can plan chained fetches.
func NewPlanEnrichmentHandler(
	connections []ConnectionWithActions,
	actionsCatalog *catalog.Catalog,
	planned *PlannedStepRegistry,
	enrichments *[]PlannedEnrichment,
) ToolHandler {
	connMap := make(map[string]ConnectionWithActions, len(connections))
	for _, conn := range connections {
		connMap[conn.Connection.ID.String()] = conn
	}

	return func(_ context.Context, _ string, raw json.RawMessage) (string, bool, error) {
		var args planEnrichmentArgs
		if err := json.Unmarshal(raw, &args); err != nil {
			return "", false, fmt.Errorf("invalid arguments: %w", err)
		}

		// 1. Connection exists?
		conn, ok := connMap[args.ConnectionID]
		if !ok {
			var listing []string
			for _, candidate := range connections {
				listing = append(listing, fmt.Sprintf("  - %s (%s)", candidate.Connection.ID, candidate.Provider))
			}
			return "", false, fmt.Errorf("connection %q not found. Available connections:\n%s",
				args.ConnectionID, strings.Join(listing, "\n"))
		}

		// 2. Action exists and is read-only?
		actionDef, actionExists := conn.ReadActions[args.Action]
		if !actionExists {
			var listing []string
			for key, action := range conn.ReadActions {
				listing = append(listing, fmt.Sprintf("  - %s: %s", key, truncate(action.Description, 60)))
			}
			return "", false, fmt.Errorf("action %q not found for provider %q. Available read actions:\n%s",
				args.Action, conn.Provider, strings.Join(listing, "\n"))
		}

		// 3. Unique step name?
		if args.As == "" {
			return "", false, fmt.Errorf("'as' (step name) is required")
		}
		if planned.Has(args.As) {
			return "", false, fmt.Errorf("step name %q already used. Planned steps: %v. Pick a unique name.",
				args.As, planned.Names())
		}

		// 4. Validate required params.
		requiredParams := extractRequiredParams(actionDef)
		for _, paramName := range requiredParams {
			val, exists := args.Params[paramName]
			if !exists {
				return "", false, fmt.Errorf("missing required param %q for action %q.\nAction params: %s\nPlanned steps so far: %v\nAvailable refs: use $refs.x for webhook payload refs",
					paramName, args.Action, formatParamSchema(actionDef), planned.Names())
			}

			// 5. If param is a {{step.field}} reference, validate the step exists.
			strVal, isString := val.(string)
			if isString {
				matches := templateRefPattern.FindAllStringSubmatch(strVal, -1)
				for _, match := range matches {
					stepName := match[1]
					if !planned.Has(stepName) {
						return "", false, fmt.Errorf("param %q references step %q which hasn't been planned yet. Planned steps: %v",
							paramName, stepName, planned.Names())
					}
				}
			}
		}

		// 6. Accept — register step and return response schema.
		planned.Add(args.As, args.Action)
		connID, _ := uuid.Parse(args.ConnectionID)
		*enrichments = append(*enrichments, PlannedEnrichment{
			ConnectionID: connID,
			As:           args.As,
			Action:       args.Action,
			Params:       args.Params,
		})

		responseSchemaInfo := ""
		if actionDef.ResponseSchema != "" {
			responseSchemaInfo = fmt.Sprintf("\nResponse schema ref: %s", actionDef.ResponseSchema)
		}

		return fmt.Sprintf("✓ Planned step %q: %s/%s.%s\nYou can reference results from this step as {{%s.field}} in subsequent plan_enrichment params.",
			args.As, conn.Provider, args.Action, responseSchemaInfo, args.As), false, nil
	}
}

// --------------------------------------------------------------------------
// finalize tool
// --------------------------------------------------------------------------

// NewFinalizeHandler creates a tool handler that signals the routing session
// is complete. The agent loop stops and collects all routed agents and
// planned enrichments.
func NewFinalizeHandler() ToolHandler {
	return func(_ context.Context, _ string, _ json.RawMessage) (string, bool, error) {
		return "✓ Routing complete.", true, nil
	}
}

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

// extractRequiredParams parses the action's JSON Schema parameters and
// returns the names of required fields.
func extractRequiredParams(action catalog.ActionDef) []string {
	if action.Parameters == nil {
		return nil
	}
	var schema struct {
		Required []string `json:"required"`
	}
	json.Unmarshal(action.Parameters, &schema)
	return schema.Required
}

// formatParamSchema returns a human-readable summary of an action's params.
func formatParamSchema(action catalog.ActionDef) string {
	if action.Parameters == nil {
		return "(no parameters)"
	}
	var schema struct {
		Properties map[string]struct {
			Type        string `json:"type"`
			Description string `json:"description"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	json.Unmarshal(action.Parameters, &schema)

	requiredSet := make(map[string]bool)
	for _, name := range schema.Required {
		requiredSet[name] = true
	}

	var parts []string
	for name, prop := range schema.Properties {
		req := ""
		if requiredSet[name] {
			req = " [required]"
		}
		parts = append(parts, fmt.Sprintf("  %s (%s%s): %s", name, prop.Type, req, truncate(prop.Description, 60)))
	}
	return strings.Join(parts, "\n")
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
