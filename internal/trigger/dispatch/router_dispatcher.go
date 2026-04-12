package dispatch

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/ziraloop/ziraloop/internal/mcp/catalog"
	"github.com/ziraloop/ziraloop/internal/model"
	"github.com/ziraloop/ziraloop/internal/trigger/zira"
)

// RouterDispatchInput is the input to the router dispatch pipeline.
// Populated from the webhook payload by the task handler.
type RouterDispatchInput struct {
	Provider     string
	EventType    string
	EventAction  string
	OrgID        uuid.UUID
	ConnectionID uuid.UUID
	Payload      map[string]any
	Headers      map[string]string
}

// AgentDispatch is the output for one agent that should receive the event.
// The executor creates a Bridge conversation for each dispatch.
type AgentDispatch struct {
	AgentID         uuid.UUID
	Priority        int
	RoutingMode     string // "rule" or "triage"
	EnrichmentPlan  []zira.PlannedEnrichment
	ReplyConnection model.Connection
	ResourceKey     string
	RunIntent       string // "normal" (new conv) or "continue" (existing conv)
	RouterTriggerID uuid.UUID
	RouterPersona   string
	MemoryTeam      string
	Refs            map[string]string

	// For "continue" intent — the existing conversation to send to.
	ExistingConversationID string
	ExistingSandboxID      uuid.UUID
}

// RouterDispatcher orchestrates the full routing pipeline: trigger match →
// thread affinity → base context → route (rule or triage) → build dispatches.
type RouterDispatcher struct {
	store   RouterTriggerStore
	catalog *catalog.Catalog
	agent   *zira.RouterAgent // nil = rule-only mode (no LLM)
	logger  *slog.Logger
}

// NewRouterDispatcher creates a dispatcher. Pass nil for agent if the org
// only uses rule-based routing (no triage calls).
func NewRouterDispatcher(store RouterTriggerStore, actionsCatalog *catalog.Catalog, routerAgent *zira.RouterAgent, logger *slog.Logger) *RouterDispatcher {
	if logger == nil {
		logger = slog.Default()
	}
	return &RouterDispatcher{store: store, catalog: actionsCatalog, agent: routerAgent, logger: logger}
}

// Run executes the routing pipeline for one inbound webhook event.
// Returns zero or more AgentDispatch instructions for the executor.
func (dispatcher *RouterDispatcher) Run(ctx context.Context, input RouterDispatchInput) ([]AgentDispatch, error) {
	eventKey := input.EventType
	if input.EventAction != "" {
		eventKey = input.EventType + "." + input.EventAction
	}

	// 1. Find matching router triggers.
	triggerMatches, err := dispatcher.store.FindMatchingTriggers(ctx, input.OrgID, input.ConnectionID, []string{eventKey})
	if err != nil {
		return nil, fmt.Errorf("finding matching triggers: %w", err)
	}
	if len(triggerMatches) == 0 {
		dispatcher.logger.Debug("no matching router triggers", "event", eventKey, "org", input.OrgID)
		return nil, nil
	}

	var allDispatches []AgentDispatch

	for _, match := range triggerMatches {
		trigger := match.Trigger
		router := match.Router

		// 2. Extract refs from payload.
		triggerDef := dispatcher.lookupTriggerDef(trigger, eventKey)
		refs, _ := extractRefs(input.Payload, triggerDef.Refs)

		// 3. Resolve resource key.
		resourceDef := dispatcher.lookupResourceDef(trigger, triggerDef)
		resourceKey := resolveRouterResourceKey(resourceDef, refs)

		// 4. Thread affinity: check for existing conversation.
		existingConv, err := dispatcher.store.FindExistingConversation(ctx, input.OrgID, input.ConnectionID, resourceKey)
		if err != nil {
			dispatcher.logger.Error("thread affinity check failed", "error", err)
		}
		if existingConv != nil {
			allDispatches = append(allDispatches, AgentDispatch{
				AgentID:                existingConv.AgentID,
				RunIntent:              "continue",
				ExistingConversationID: existingConv.BridgeConversationID,
				ExistingSandboxID:      existingConv.SandboxID,
				ResourceKey:            resourceKey,
				Refs:                   refs,
				RouterTriggerID:        trigger.ID,
				RouterPersona:          router.Persona,
				MemoryTeam:             router.MemoryTeam,
			})
			continue
		}

		// 5. Route: rule-based or LLM triage.
		var selectedAgents []zira.AgentSelection
		var enrichmentPlan []zira.PlannedEnrichment
		routingMode := trigger.RoutingMode
		routingStart := time.Now()

		switch routingMode {
		case "rule":
			rules, rulesErr := dispatcher.store.LoadRulesForTrigger(ctx, trigger.ID)
			if rulesErr != nil {
				return nil, fmt.Errorf("loading rules: %w", rulesErr)
			}
			selectedAgents = EvaluateRules(rules, input.Payload)

		case "triage":
			if dispatcher.agent == nil {
				return nil, fmt.Errorf("triage routing requested but no LLM agent configured")
			}
			orgAgents, agentsErr := dispatcher.store.LoadOrgAgents(ctx, input.OrgID)
			if agentsErr != nil {
				return nil, fmt.Errorf("loading org agents: %w", agentsErr)
			}
			var connections []zira.ConnectionWithActions
			if trigger.EnrichCrossReferences {
				connections, _ = dispatcher.store.LoadOrgConnections(ctx, input.OrgID, input.ConnectionID)
			}
			recentDecisions, _ := dispatcher.store.LoadRecentDecisions(ctx, input.OrgID, eventKey, 10)

			systemPrompt := zira.BuildRoutingPrompt(router.Persona, orgAgents, connections, recentDecisions)

			// Build user message from event context.
			userMessage := buildTriageUserMessage(input, refs)

			result, triageErr := dispatcher.agent.Route(ctx, systemPrompt, userMessage, orgAgents, connections)
			if triageErr != nil {
				dispatcher.logger.Error("triage routing failed", "error", triageErr)
				// Fall through to default agent.
			} else {
				selectedAgents = result.SelectedAgents
				enrichmentPlan = result.EnrichmentPlan
			}
		}

		// 6. Fallback to default agent if no agents selected.
		if len(selectedAgents) == 0 && router.DefaultAgentID != nil {
			selectedAgents = []zira.AgentSelection{{
				AgentID:  *router.DefaultAgentID,
				Priority: 99,
				Reason:   "fallback to default agent",
			}}
		}

		if len(selectedAgents) == 0 {
			dispatcher.logger.Info("no agents selected for event", "event", eventKey, "trigger", trigger.ID)
			continue
		}

		// 7. Build dispatches.
		// The reply connection is resolved by the executor from the trigger's
		// ConnectionID — the dispatcher just passes the ID through.
		for _, selection := range selectedAgents {
			allDispatches = append(allDispatches, AgentDispatch{
				AgentID:         selection.AgentID,
				Priority:        selection.Priority,
				RoutingMode:     routingMode,
				EnrichmentPlan:  enrichmentPlan,
				ReplyConnection: model.Connection{ID: input.ConnectionID, OrgID: input.OrgID},
				ResourceKey:     resourceKey,
				RunIntent:       "normal",
				RouterTriggerID: trigger.ID,
				RouterPersona:   router.Persona,
				MemoryTeam:      router.MemoryTeam,
				Refs:            refs,
			})
		}

		// 9. Store routing decision.
		agentIDs := make(pq.StringArray, len(selectedAgents))
		for index, selection := range selectedAgents {
			agentIDs[index] = selection.AgentID.String()
		}
		intentSummary := ""
		if len(selectedAgents) > 0 {
			intentSummary = selectedAgents[0].Reason
		}
		dispatcher.store.StoreDecision(ctx, &model.RoutingDecision{
			OrgID:           input.OrgID,
			RouterTriggerID: trigger.ID,
			RoutingMode:     routingMode,
			EventType:       eventKey,
			ResourceKey:     resourceKey,
			IntentSummary:   intentSummary,
			SelectedAgents:  agentIDs,
			EnrichmentSteps: len(enrichmentPlan),
			LatencyMs:       int(time.Since(routingStart).Milliseconds()),
		})
	}

	return allDispatches, nil
}

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

func (dispatcher *RouterDispatcher) lookupTriggerDef(trigger model.RouterTrigger, eventKey string) catalog.TriggerDef {
	// Try the trigger's connection provider first, then variant fallback.
	var connection model.Connection
	// In production, we'd load the connection's integration.provider.
	// For now, use the catalog's variant lookup.
	providerTriggers, ok := dispatcher.catalog.GetProviderTriggers(trigger.ConnectionID.String())
	if !ok {
		// Try common providers.
		for _, provider := range []string{"github", "slack", "linear", "discord"} {
			providerTriggers, ok = dispatcher.catalog.GetProviderTriggers(provider)
			if ok {
				break
			}
		}
	}
	_ = connection
	if providerTriggers != nil {
		if def, ok := providerTriggers.Triggers[eventKey]; ok {
			return def
		}
	}
	return catalog.TriggerDef{}
}

func (dispatcher *RouterDispatcher) lookupResourceDef(trigger model.RouterTrigger, triggerDef catalog.TriggerDef) *catalog.ResourceDef {
	if triggerDef.ResourceType == "" {
		return nil
	}
	// Same provider resolution issue as lookupTriggerDef — simplified for now.
	return nil
}

func resolveRouterResourceKey(resourceDef *catalog.ResourceDef, refs map[string]string) string {
	if resourceDef == nil || resourceDef.ResourceKeyTemplate == "" {
		return ""
	}
	return substituteRefs(resourceDef.ResourceKeyTemplate, refs)
}

func buildTriageUserMessage(input RouterDispatchInput, refs map[string]string) string {
	// Build a concise event summary for the LLM.
	msg := fmt.Sprintf("Provider: %s\nEvent: %s", input.Provider, input.EventType)
	if input.EventAction != "" {
		msg += "." + input.EventAction
	}
	msg += "\n"

	// Include key refs.
	for key, value := range refs {
		msg += fmt.Sprintf("%s: %s\n", key, value)
	}

	// Include message text if present (Slack mentions, GitHub comments, etc.).
	if text, ok := refs["text"]; ok {
		msg += fmt.Sprintf("\nMessage: %s\n", text)
	}

	return msg
}
