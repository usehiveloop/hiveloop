package dispatch

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/usehiveloop/hiveloop/internal/mcp/catalog"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/trigger/hiveloop"
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
	EnrichmentPlan     []hiveloop.PlannedEnrichment
	ReplyConnectionID  uuid.UUID // in_connections ID for the source channel
	ReplyOrgID         uuid.UUID
	ResourceKey        string
	RunIntent       string // "normal" (new conv) or "continue" (existing conv)
	RouterTriggerID uuid.UUID
	RouterPersona   string
	MemoryTeam      string
	Refs            map[string]string

	// Set by the enrichment agent — replaces flat refs in instructions.
	EnrichedMessage string

	// TriggerInstructions is the per-trigger prompt template (cron/http triggers).
	// Takes precedence over flat refs when building the first message.
	TriggerInstructions string

	// For "continue" intent — the existing conversation to send to.
	ExistingConversationID string
	ExistingSandboxID      uuid.UUID
}

// RouterDispatcher orchestrates the full routing pipeline: trigger match →
// thread affinity → base context → route (rule or triage) → build dispatches.
type RouterDispatcher struct {
	store   RouterTriggerStore
	catalog *catalog.Catalog
	agent   *hiveloop.RouterAgent // nil = rule-only mode (no LLM)
	logger  *slog.Logger
}

// NewRouterDispatcher creates a dispatcher. Pass nil for agent if the org
// only uses rule-based routing (no triage calls).
func NewRouterDispatcher(store RouterTriggerStore, actionsCatalog *catalog.Catalog, routerAgent *hiveloop.RouterAgent, logger *slog.Logger) *RouterDispatcher {
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
		dispatcher.logger.Debug("no matching triggers", "event_key", eventKey, "connection_id", input.ConnectionID)
		return nil, nil
	}

	var allDispatches []AgentDispatch

	for _, match := range triggerMatches {
		dispatches, dispatchErr := dispatcher.dispatchForTrigger(ctx, match, input, eventKey)
		if dispatchErr != nil {
			return nil, dispatchErr
		}
		allDispatches = append(allDispatches, dispatches...)
	}

	return allDispatches, nil
}

// RunForTrigger executes the routing pipeline for a specific trigger, bypassing
// trigger matching. Used by HTTP and cron triggers where the trigger is already
// known. The payload is optional — cron triggers pass nil (no webhook body).
func (dispatcher *RouterDispatcher) RunForTrigger(ctx context.Context, triggerID uuid.UUID, payload map[string]any) ([]AgentDispatch, error) {
	match, err := dispatcher.store.FindTriggerByID(ctx, triggerID)
	if err != nil {
		return nil, fmt.Errorf("loading trigger %s: %w", triggerID, err)
	}

	if payload == nil {
		payload = map[string]any{}
	}

	eventKey := match.Trigger.TriggerType // "http" or "cron"

	input := RouterDispatchInput{
		Provider:  match.Trigger.TriggerType,
		EventType: match.Trigger.TriggerType,
		OrgID:     match.Trigger.OrgID,
		Payload:   payload,
	}
	if match.Trigger.ConnectionID != nil {
		input.ConnectionID = *match.Trigger.ConnectionID
	}

	dispatches, err := dispatcher.dispatchForTrigger(ctx, *match, input, eventKey)
	if err != nil {
		return nil, err
	}

	// For cron/http triggers, inject the trigger's instructions into each dispatch.
	if match.Trigger.Instructions != "" {
		for index := range dispatches {
			if dispatches[index].RunIntent == "normal" {
				dispatches[index].TriggerInstructions = match.Trigger.Instructions
			}
		}
	}

	return dispatches, nil
}

// dispatchForTrigger runs the routing pipeline for a single trigger + input.
// Shared by Run (webhook matching) and RunForTrigger (direct dispatch).
func (dispatcher *RouterDispatcher) dispatchForTrigger(ctx context.Context, match RouterTriggerWithRouter, input RouterDispatchInput, eventKey string) ([]AgentDispatch, error) {
	trigger := match.Trigger
	router := match.Router

	// 1. Extract refs from payload.
	triggerDef := dispatcher.lookupTriggerDef(input.Provider, eventKey)
	refs, _ := extractRefs(input.Payload, triggerDef.Refs)

	// 2. Resolve resource key.
	resourceDef := dispatcher.lookupResourceDef(trigger, triggerDef)
	resourceKey := resolveRouterResourceKey(resourceDef, refs)

	// 3. Thread affinity: check for existing conversation.
	connectionID := input.ConnectionID
	existingConv, err := dispatcher.store.FindExistingConversation(ctx, input.OrgID, connectionID, resourceKey)
	if err != nil {
		dispatcher.logger.Error("thread affinity check failed", "error", err)
	}
	if existingConv != nil {
		return []AgentDispatch{{
			AgentID:                existingConv.AgentID,
			RunIntent:              "continue",
			ExistingConversationID: existingConv.BridgeConversationID,
			ExistingSandboxID:      existingConv.SandboxID,
			ResourceKey:            resourceKey,
			Refs:                   refs,
			RouterTriggerID:        trigger.ID,
			RouterPersona:          router.Persona,
			MemoryTeam:             router.MemoryTeam,
		}}, nil
	}

	// 4. Route: rule-based or LLM triage.
	var selectedAgents []hiveloop.AgentSelection
	var enrichmentPlan []hiveloop.PlannedEnrichment
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
		var connections []hiveloop.ConnectionWithActions
		if trigger.EnrichCrossReferences {
			connections, _ = dispatcher.store.LoadOrgConnections(ctx, input.OrgID, input.ConnectionID)
		}
		recentDecisions, _ := dispatcher.store.LoadRecentDecisions(ctx, input.OrgID, eventKey, 10)

		systemPrompt := hiveloop.BuildRoutingPrompt(router.Persona, orgAgents, connections, recentDecisions)

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

	routingLatency := time.Since(routingStart).Milliseconds()

	// 5. Fallback to default agent if no agents selected.
	if len(selectedAgents) == 0 && router.DefaultAgentID != nil {
		selectedAgents = []hiveloop.AgentSelection{{
			AgentID:  *router.DefaultAgentID,
			Priority: 99,
			Reason:   "fallback to default agent",
		}}
	}

	if len(selectedAgents) == 0 {
		dispatcher.logger.Info("no agents selected", "event", eventKey, "trigger_id", trigger.ID)
		return nil, nil
	}

	dispatcher.logger.Info("trigger matched",
		"trigger_id", trigger.ID,
		"event_key", eventKey,
		"routing_mode", routingMode,
		"routing_latency_ms", routingLatency,
		"ref_count", len(refs),
		"agents_selected", len(selectedAgents),
	)

	// 6. Build dispatches.
	var dispatches []AgentDispatch
	for _, selection := range selectedAgents {
		dispatches = append(dispatches, AgentDispatch{
			AgentID:           selection.AgentID,
			Priority:          selection.Priority,
			RoutingMode:       routingMode,
			EnrichmentPlan:    enrichmentPlan,
			ReplyConnectionID: input.ConnectionID,
			ReplyOrgID:        input.OrgID,
			ResourceKey:       resourceKey,
			RunIntent:         "normal",
			RouterTriggerID:   trigger.ID,
			RouterPersona:     router.Persona,
			MemoryTeam:        router.MemoryTeam,
			Refs:              refs,
		})
	}

	// 7. Store routing decision.
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

	return dispatches, nil
}

// LoadConnections loads all org connections with their read actions.
// Used by the enrichment agent to know what integrations are available.
func (dispatcher *RouterDispatcher) LoadConnections(ctx context.Context, orgID uuid.UUID) ([]hiveloop.ConnectionWithActions, error) {
	return dispatcher.store.LoadOrgConnections(ctx, orgID, uuid.Nil)
}

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

func (dispatcher *RouterDispatcher) lookupTriggerDef(provider, eventKey string) catalog.TriggerDef {
	providerTriggers, ok := dispatcher.catalog.GetProviderTriggers(provider)
	if !ok {
		providerTriggers, ok = dispatcher.catalog.GetProviderTriggersForVariant(provider)
	}
	if ok {
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
