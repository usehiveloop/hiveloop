package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"

	"github.com/ziraloop/ziraloop/internal/trigger/dispatch"
	"github.com/ziraloop/ziraloop/internal/trigger/enrichment"
	"github.com/ziraloop/ziraloop/internal/trigger/executor"
	"github.com/ziraloop/ziraloop/internal/trigger/zira"
)

// RouterDispatchHandler handles the TypeRouterDispatch Asynq task.
// It runs the router dispatcher pipeline, optionally enriches context,
// then runs the executor to create or continue Bridge conversations.
type RouterDispatchHandler struct {
	dispatcher         *dispatch.RouterDispatcher
	executor           *executor.Executor
	enricher           *enrichment.EnrichmentAgent // nil = skip enrichment
	credentialResolver EnrichmentCredentialResolver // nil = skip enrichment
}

// EnrichmentCredentialResolver resolves an LLM credential for enrichment.
// Returns a CompletionClient, model ID, and provider group (e.g. "anthropic",
// "openai", "gemini"). Passed as a function to avoid coupling the handler to
// credential internals.
type EnrichmentCredentialResolver func(ctx context.Context, orgID string) (client zira.CompletionClient, modelID string, providerGroup string, err error)

// NewRouterDispatchHandler creates a task handler with the dispatcher and executor.
func NewRouterDispatchHandler(dispatcher *dispatch.RouterDispatcher, execut *executor.Executor) *RouterDispatchHandler {
	return &RouterDispatchHandler{dispatcher: dispatcher, executor: execut}
}

// SetEnrichment configures the enrichment agent and credential resolver.
// When both are set, the handler gathers context before creating conversations.
func (handler *RouterDispatchHandler) SetEnrichment(enricher *enrichment.EnrichmentAgent, resolver EnrichmentCredentialResolver) {
	handler.enricher = enricher
	handler.credentialResolver = resolver
}

// Handle processes a TypeRouterDispatch task.
func (handler *RouterDispatchHandler) Handle(ctx context.Context, task *asynq.Task) error {
	var payload TriggerDispatchPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal router dispatch payload: %w", err)
	}

	// Build a logger with the delivery ID attached to every message in this flow.
	logger := slog.With(
		"delivery_id", payload.DeliveryID,
		"org_id", payload.OrgID,
		"provider", payload.Provider,
		"event", payload.EventType+"."+payload.EventAction,
		"connection_id", payload.ConnectionID,
	)

	logger.Info("webhook received",
		"payload_bytes", len(payload.PayloadJSON),
		"payload", string(payload.PayloadJSON),
	)

	// Decode the raw webhook payload.
	var webhookPayload map[string]any
	if err := json.Unmarshal(payload.PayloadJSON, &webhookPayload); err != nil {
		logger.Error("failed to unmarshal webhook payload", "error", err)
		return fmt.Errorf("unmarshal webhook payload: %w", err)
	}

	input := dispatch.RouterDispatchInput{
		Provider:     payload.Provider,
		EventType:    payload.EventType,
		EventAction:  payload.EventAction,
		OrgID:        payload.OrgID,
		ConnectionID: payload.ConnectionID,
		Payload:      webhookPayload,
	}

	// Run dispatcher: match triggers, evaluate rules, select agents.
	logger.Info("dispatcher starting")
	dispatches, err := handler.dispatcher.Run(ctx, input)
	if err != nil {
		logger.Error("dispatcher failed", "error", err)
		return fmt.Errorf("router dispatch: %w", err)
	}

	if len(dispatches) == 0 {
		logger.Info("dispatcher matched no agents")
		return nil
	}

	// Log each dispatch decision.
	for dispatchIndex, agentDispatch := range dispatches {
		logger.Info("dispatcher selected agent",
			"dispatch_index", dispatchIndex,
			"agent_id", agentDispatch.AgentID,
			"routing_mode", agentDispatch.RoutingMode,
			"run_intent", agentDispatch.RunIntent,
			"priority", agentDispatch.Priority,
			"resource_key", agentDispatch.ResourceKey,
			"trigger_id", agentDispatch.RouterTriggerID,
			"ref_count", len(agentDispatch.Refs),
		)
	}

	// Run enrichment for new conversations (best effort).
	handler.runEnrichment(ctx, logger, dispatches, input)

	// Execute: create or continue Bridge conversations.
	logger.Info("executor starting", "dispatch_count", len(dispatches))
	if err := handler.executor.Execute(ctx, dispatches); err != nil {
		logger.Error("executor failed", "error", err)
		return fmt.Errorf("router execute: %w", err)
	}

	logger.Info("pipeline complete", "agents_dispatched", len(dispatches))
	return nil
}

// runEnrichment gathers context for new conversations. Failures are logged but
// never prevent the specialist from running.
func (handler *RouterDispatchHandler) runEnrichment(ctx context.Context, logger *slog.Logger, dispatches []dispatch.AgentDispatch, input dispatch.RouterDispatchInput) {
	if handler.enricher == nil {
		logger.Debug("enrichment skipped: no enrichment agent configured")
		return
	}

	// Only enrich if there are new conversations to create.
	hasNewConversations := false
	for _, agentDispatch := range dispatches {
		if agentDispatch.RunIntent == "normal" {
			hasNewConversations = true
			break
		}
	}
	if !hasNewConversations {
		logger.Debug("enrichment skipped: all dispatches are continuations")
		return
	}

	// Load all org connections for enrichment.
	connections, err := handler.dispatcher.LoadConnections(ctx, input.OrgID)
	if err != nil {
		logger.Warn("enrichment skipped: failed to load connections", "error", err)
		return
	}
	if len(connections) == 0 {
		logger.Warn("enrichment skipped: no connections available")
		return
	}

	logger.Info("enrichment starting",
		"connections_available", len(connections),
	)

	// Use refs from the first dispatch (all dispatches share the same event).
	refs := dispatches[0].Refs

	enrichInput := enrichment.EnrichmentInput{
		Provider:    input.Provider,
		EventType:   input.EventType,
		EventAction: input.EventAction,
		OrgID:       input.OrgID,
		Refs:        refs,
		Connections: connections,
	}

	if handler.credentialResolver == nil {
		logger.Warn("enrichment skipped: no credential resolver configured")
		return
	}
	client, modelID, providerGroup, resolveErr := handler.credentialResolver(ctx, input.OrgID.String())
	if resolveErr != nil {
		logger.Warn("enrichment skipped: credential resolution failed",
			"error", resolveErr,
		)
		return
	}

	logger.Info("enrichment credential resolved",
		"model", modelID,
		"provider_group", providerGroup,
	)

	result, enrichErr := handler.enricher.Enrich(ctx, client, modelID, providerGroup, enrichInput, logger)
	if enrichErr != nil {
		logger.Warn("enrichment failed", "error", enrichErr)
		return
	}

	if result.ComposedMessage == "" {
		logger.Warn("enrichment produced empty message")
		return
	}

	// Apply the enriched message to all new-conversation dispatches.
	enrichedCount := 0
	for index := range dispatches {
		if dispatches[index].RunIntent == "normal" {
			dispatches[index].EnrichedMessage = result.ComposedMessage
			enrichedCount++
		}
	}

	logger.Info("enrichment complete",
		"fetch_count", result.FetchCount,
		"turn_count", result.TurnCount,
		"latency_ms", result.LatencyMs,
		"composed_message_bytes", len(result.ComposedMessage),
		"dispatches_enriched", enrichedCount,
	)
}
