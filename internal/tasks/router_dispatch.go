package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/hibiken/asynq"

	"github.com/ziraloop/ziraloop/internal/trigger/dispatch"
	"github.com/ziraloop/ziraloop/internal/trigger/enrichment"
	"github.com/ziraloop/ziraloop/internal/trigger/executor"
)

// RouterDispatchHandler handles the TypeRouterDispatch Asynq task.
// It runs the router dispatcher pipeline, enriches context via deterministic
// API calls, then runs the executor to create or continue Bridge conversations.
type RouterDispatchHandler struct {
	dispatcher           *dispatch.RouterDispatcher
	executor             *executor.Executor
	deterministicEnricher *enrichment.DeterministicEnricher // nil = skip enrichment
}

// NewRouterDispatchHandler creates a task handler with the dispatcher and executor.
func NewRouterDispatchHandler(dispatcher *dispatch.RouterDispatcher, execut *executor.Executor) *RouterDispatchHandler {
	return &RouterDispatchHandler{dispatcher: dispatcher, executor: execut}
}

// SetDeterministicEnrichment configures the deterministic enrichment engine.
func (handler *RouterDispatchHandler) SetDeterministicEnrichment(enricher *enrichment.DeterministicEnricher) {
	handler.deterministicEnricher = enricher
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

	// Run deterministic enrichment for new conversations (best effort).
	handler.runDeterministicEnrichment(ctx, logger, dispatches, payload)

	// TODO: re-enable executor after dedicated agent sandbox creation is implemented.
	// logger.Info("executor starting", "dispatch_count", len(dispatches))
	// if err := handler.executor.Execute(ctx, dispatches); err != nil {
	// 	logger.Error("executor failed", "error", err)
	// 	return fmt.Errorf("router execute: %w", err)
	// }

	logger.Info("pipeline complete",
		"agents_dispatched", len(dispatches),
		"enriched", len(dispatches) > 0 && dispatches[0].EnrichedMessage != "",
		"instruction_bytes", func() int {
			if len(dispatches) > 0 {
				return len(buildDispatchInstructions(dispatches[0]))
			}
			return 0
		}(),
	)
	return nil
}

// runDeterministicEnrichment pre-fetches context from provider APIs using
// the enrichment actions defined in the trigger catalog. Failures are logged
// but never prevent the agent from running.
func (handler *RouterDispatchHandler) runDeterministicEnrichment(ctx context.Context, logger *slog.Logger, dispatches []dispatch.AgentDispatch, payload TriggerDispatchPayload) {
	if handler.deterministicEnricher == nil {
		logger.Debug("enrichment skipped: no deterministic enricher configured")
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

	// Use refs from the first dispatch (all dispatches share the same event).
	refs := dispatches[0].Refs

	enrichInput := enrichment.DeterministicEnrichInput{
		Provider:     payload.Provider,
		EventType:    payload.EventType,
		EventAction:  payload.EventAction,
		OrgID:        payload.OrgID,
		ConnectionID: payload.ConnectionID,
		Refs:         refs,
	}

	composedMessage, err := handler.deterministicEnricher.Enrich(ctx, enrichInput, logger)
	if err != nil {
		logger.Warn("deterministic enrichment failed", "error", err)
		return
	}
	if composedMessage == "" {
		logger.Info("deterministic enrichment produced no message")
		return
	}

	// Apply the enriched message to all new-conversation dispatches.
	enrichedCount := 0
	for index := range dispatches {
		if dispatches[index].RunIntent == "normal" {
			dispatches[index].EnrichedMessage = composedMessage
			enrichedCount++
		}
	}

	logger.Info("deterministic enrichment applied",
		"dispatches_enriched", enrichedCount,
		"composed_message_bytes", len(composedMessage),
	)
}

// buildDispatchInstructions mirrors the executor's buildInstructions logic
// so we can log exactly what the agent would receive.
func buildDispatchInstructions(agentDispatch dispatch.AgentDispatch) string {
	var builder strings.Builder

	if agentDispatch.RouterPersona != "" {
		builder.WriteString(agentDispatch.RouterPersona)
		builder.WriteString("\n\n---\n\n")
	}

	if agentDispatch.EnrichedMessage != "" {
		builder.WriteString(agentDispatch.EnrichedMessage)
		return builder.String()
	}

	for key, value := range agentDispatch.Refs {
		builder.WriteString(fmt.Sprintf("%s: %s\n", key, value))
	}

	return builder.String()
}

