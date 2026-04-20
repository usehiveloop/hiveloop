package zira

import (
	"fmt"
	"strings"

	"github.com/usehiveloop/hiveloop/internal/model"
)

// BuildRoutingPrompt constructs the system prompt for Zira's routing agent.
// It teaches the LLM how to use route_to_agent + plan_enrichment + finalize
// tools, provides the full agent catalog and connection action schemas (with
// response schemas), and injects recent routing decisions as few-shot examples.
func BuildRoutingPrompt(
	persona string,
	agents []model.Agent,
	connections []ConnectionWithActions,
	recentDecisions []model.RoutingDecision,
) string {
	var builder strings.Builder

	// Role
	builder.WriteString(`You are Zira's routing engine. Your job is to decide which agent(s) should handle an incoming event and what context should be gathered beforehand.

You MUST only use tool calls. Do not produce any text output. Every response must contain at least one tool call.

## Tools

1. **route_to_agent(agent_id, priority, reason)** — Select an agent to handle this event. Call multiple times for multi-agent dispatch. Priority 1 = highest (runs first).

2. **plan_enrichment(connection_id, action, as, params)** — Plan a context-gathering step. The executor will run it later. Use $refs.x for webhook payload values. Use {{step_name.field}} to reference results from a previous plan_enrichment step.

3. **finalize()** — Signal that routing is complete. Call this after all route_to_agent and plan_enrichment calls.

## Rules

- Call route_to_agent at least once before finalize (unless no agent is relevant).
- For plan_enrichment, chain steps using {{step_name.field}} when a later fetch needs data from an earlier one. The step_name must match the 'as' value of a previous plan_enrichment call.
- Only plan READ actions. Write actions are handled by the agent, not the routing engine.
- Be decisive. Pick 1-3 agents, not more.
- For enrichment, think about what context the selected agent will NEED. Don't just fetch the obvious entity — fetch related context too (project scope, milestones, review comments, thread history, etc.).
`)

	// Persona
	if persona != "" {
		builder.WriteString("\n## Zira Persona\n\n")
		builder.WriteString(persona)
		builder.WriteString("\n")
	}

	// Agent catalog
	builder.WriteString("\n## Available Agents\n\n")
	if len(agents) == 0 {
		builder.WriteString("No agents configured in this organization.\n")
	} else {
		for _, agent := range agents {
			description := "(no description)"
			if agent.Description != nil && *agent.Description != "" {
				description = *agent.Description
			}
			builder.WriteString(fmt.Sprintf("- **%s** (ID: `%s`): %s\n", agent.Name, agent.ID, description))
		}
	}

	// Connection catalog with action schemas
	builder.WriteString("\n## Available Connections for Enrichment\n\n")
	if len(connections) == 0 {
		builder.WriteString("No additional connections available for enrichment.\n")
	} else {
		for _, conn := range connections {
			builder.WriteString(fmt.Sprintf("### %s (Connection ID: `%s`)\n\n", conn.Provider, conn.Connection.ID))
			if len(conn.ReadActions) == 0 {
				builder.WriteString("No read actions available.\n\n")
				continue
			}
			for actionKey, action := range conn.ReadActions {
				builder.WriteString(fmt.Sprintf("**%s** — %s\n", actionKey, action.Description))
				paramInfo := formatParamSchema(action)
				if paramInfo != "(no parameters)" {
					builder.WriteString(fmt.Sprintf("Params:\n%s\n", paramInfo))
				}
				if action.ResponseSchema != "" {
					builder.WriteString(fmt.Sprintf("Response schema ref: %s\n", action.ResponseSchema))
				}
				builder.WriteString("\n")
			}
		}
	}

	// Recent routing decisions as few-shot examples
	if len(recentDecisions) > 0 {
		builder.WriteString("## Recent Routing Decisions (for reference)\n\n")
		for _, decision := range recentDecisions {
			agentList := strings.Join(decision.SelectedAgents, ", ")
			builder.WriteString(fmt.Sprintf("- Event: %s | Intent: %s | Routed to: [%s]\n",
				decision.EventType, decision.IntentSummary, agentList))
		}
		builder.WriteString("\n")
	}

	return builder.String()
}
