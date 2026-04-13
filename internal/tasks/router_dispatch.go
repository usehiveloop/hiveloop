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
	enricher           *enrichment.EnrichmentAgent    // nil = skip enrichment
	credentialResolver EnrichmentCredentialResolver    // nil = skip enrichment
}

// EnrichmentCredentialResolver resolves an LLM credential for enrichment.
// Passed as a function to avoid coupling the handler to credential internals.
type EnrichmentCredentialResolver func(ctx context.Context, orgID string) (zira.CompletionClient, string, error)

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

	// Decode the raw webhook payload.
	var webhookPayload map[string]any
	if err := json.Unmarshal(payload.PayloadJSON, &webhookPayload); err != nil {
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

	dispatches, err := handler.dispatcher.Run(ctx, input)
	if err != nil {
		slog.Error("router dispatch failed", "error", err, "delivery_id", payload.DeliveryID)
		return fmt.Errorf("router dispatch: %w", err)
	}

	if len(dispatches) == 0 {
		slog.Info("router dispatch: no agents dispatched",
			"event", payload.EventType+"."+payload.EventAction,
			"delivery_id", payload.DeliveryID)
		return nil
	}

	// Run enrichment for new conversations (best effort).
	handler.runEnrichment(ctx, dispatches, input)

	if err := handler.executor.Execute(ctx, dispatches); err != nil {
		slog.Error("router executor failed", "error", err, "delivery_id", payload.DeliveryID)
		return fmt.Errorf("router execute: %w", err)
	}

	slog.Info("router dispatch complete",
		"event", payload.EventType+"."+payload.EventAction,
		"delivery_id", payload.DeliveryID,
		"agents", len(dispatches))

	return nil
}

// runEnrichment gathers context for new conversations. Failures are logged but
// never prevent the specialist from running.
func (handler *RouterDispatchHandler) runEnrichment(ctx context.Context, dispatches []dispatch.AgentDispatch, input dispatch.RouterDispatchInput) {
	if handler.enricher == nil {
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
		return
	}

	// Load all org connections for enrichment.
	connections, err := handler.dispatcher.LoadConnections(ctx, input.OrgID)
	if err != nil {
		slog.Warn("enrichment: failed to load connections", "error", err)
		return
	}
	if len(connections) == 0 {
		return
	}

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

	// The enrichment agent needs a CompletionClient. For now, we skip enrichment
	// if no client is available. In production, credential resolution is wired
	// via the CredentialResolver set on the handler.
	if handler.credentialResolver == nil {
		return
	}
	client, modelID, resolveErr := handler.credentialResolver(ctx, input.OrgID.String())
	if resolveErr != nil {
		slog.Warn("enrichment: no LLM credential available", "org", input.OrgID, "error", resolveErr)
		return
	}

	result, enrichErr := handler.enricher.Enrich(ctx, client, modelID, enrichInput)
	if enrichErr != nil {
		slog.Warn("enrichment: agent failed", "error", enrichErr)
		return
	}

	if result.ComposedMessage == "" {
		return
	}

	// Apply the enriched message to all new-conversation dispatches.
	for index := range dispatches {
		if dispatches[index].RunIntent == "normal" {
			dispatches[index].EnrichedMessage = result.ComposedMessage
		}
	}

	slog.Info("enrichment: complete",
		"fetches", result.FetchCount,
		"turns", result.TurnCount,
		"latency_ms", result.LatencyMs)
}
