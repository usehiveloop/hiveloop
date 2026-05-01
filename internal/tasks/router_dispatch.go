package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/hibiken/asynq"

	"github.com/usehiveloop/hiveloop/internal/enqueue"
	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/trigger/dispatch"
	"github.com/usehiveloop/hiveloop/internal/trigger/enrichment"
)

// RouterDispatchHandler handles the TypeRouterDispatch Asynq task.
// It runs the router dispatcher pipeline, enriches context via deterministic
// API calls, then enqueues agent conversation creation jobs.
type RouterDispatchHandler struct {
	dispatcher            *dispatch.RouterDispatcher
	enqueuer              enqueue.TaskEnqueuer
	deterministicEnricher *enrichment.DeterministicEnricher // nil = skip enrichment
}

// NewRouterDispatchHandler creates a task handler with the dispatcher and enqueuer.
func NewRouterDispatchHandler(dispatcher *dispatch.RouterDispatcher, enqueuer enqueue.TaskEnqueuer) *RouterDispatchHandler {
	return &RouterDispatchHandler{dispatcher: dispatcher, enqueuer: enqueuer}
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

	logger := logging.FromContext(ctx).With(
		"delivery_id", payload.DeliveryID,
		"org_id", payload.OrgID,
		"provider", payload.Provider,
		"event", payload.EventType+"."+payload.EventAction,
		"connection_id", payload.ConnectionID,
	)

	var webhookPayload map[string]any
	if err := json.Unmarshal(payload.PayloadJSON, &webhookPayload); err != nil {
		return fmt.Errorf("unmarshal webhook payload: %w", err)
	}

	var dispatches []dispatch.AgentDispatch
	var dispatchErr error
	if payload.RouterTriggerID != nil {

		dispatches, dispatchErr = handler.dispatcher.RunForTrigger(ctx, *payload.RouterTriggerID, webhookPayload)
	} else {

		input := dispatch.RouterDispatchInput{
			Provider:     payload.Provider,
			EventType:    payload.EventType,
			EventAction:  payload.EventAction,
			OrgID:        payload.OrgID,
			ConnectionID: payload.ConnectionID,
			Payload:      webhookPayload,
		}
		dispatches, dispatchErr = handler.dispatcher.Run(ctx, input)
	}
	if dispatchErr != nil {
		return fmt.Errorf("router dispatch: %w", dispatchErr)
	}

	if len(dispatches) == 0 {
		return nil
	}

	handler.runDeterministicEnrichment(ctx, logger, dispatches, payload)

	enqueuedCount := 0
	for _, agentDispatch := range dispatches {
		if agentDispatch.RunIntent != "normal" {
			continue
		}

		instructions := buildDispatchInstructions(agentDispatch)
		convTask, taskErr := NewAgentConversationCreateTask(AgentConversationCreatePayload{
			AgentID:         agentDispatch.AgentID,
			OrgID:           agentDispatch.ReplyOrgID,
			DeliveryID:      payload.DeliveryID,
			ConnectionID:    agentDispatch.ReplyConnectionID,
			RouterTriggerID: agentDispatch.RouterTriggerID,
			ResourceKey:     agentDispatch.ResourceKey,
			RouterPersona:   agentDispatch.RouterPersona,
			MemoryTeam:      agentDispatch.MemoryTeam,
			Instructions:    instructions,
		})
		if taskErr != nil {
			logging.Capture(ctx, fmt.Errorf("build conversation create task for agent %s: %w", agentDispatch.AgentID, taskErr))
			continue
		}

		if _, enqErr := handler.enqueuer.Enqueue(convTask); enqErr != nil {
			logging.Capture(ctx, fmt.Errorf("enqueue conversation create task for agent %s: %w", agentDispatch.AgentID, enqErr))
			continue
		}
		enqueuedCount++
	}

	return nil
}

// runDeterministicEnrichment pre-fetches context from provider APIs using
// the enrichment actions defined in the trigger catalog. Failures are logged
// but never prevent the agent from running.
func (handler *RouterDispatchHandler) runDeterministicEnrichment(ctx context.Context, logger *slog.Logger, dispatches []dispatch.AgentDispatch, payload TriggerDispatchPayload) {
	if handler.deterministicEnricher == nil {
		return
	}

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
		logging.Capture(ctx, fmt.Errorf("deterministic enrichment: %w", err))
		return
	}
	if composedMessage == "" {
		return
	}

	for index := range dispatches {
		if dispatches[index].RunIntent == "normal" {
			dispatches[index].EnrichedMessage = composedMessage
		}
	}
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

	if agentDispatch.TriggerInstructions != "" {
		builder.WriteString(dispatch.SubstituteRefs(agentDispatch.TriggerInstructions, agentDispatch.Refs))
		if len(agentDispatch.Refs) > 0 {
			builder.WriteString("\n\n---\n\n")
			for key, value := range agentDispatch.Refs {
				builder.WriteString(fmt.Sprintf("%s: %s\n", key, value))
			}
		}
		return builder.String()
	}

	for key, value := range agentDispatch.Refs {
		builder.WriteString(fmt.Sprintf("%s: %s\n", key, value))
	}

	return builder.String()
}
