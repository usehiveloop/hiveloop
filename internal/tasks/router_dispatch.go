package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"

	"github.com/ziraloop/ziraloop/internal/trigger/dispatch"
	"github.com/ziraloop/ziraloop/internal/trigger/executor"
)

// RouterDispatchHandler handles the TypeRouterDispatch Asynq task.
// It runs the router dispatcher pipeline, then the executor to create
// or continue Bridge conversations.
type RouterDispatchHandler struct {
	dispatcher *dispatch.RouterDispatcher
	executor   *executor.Executor
}

// NewRouterDispatchHandler creates a task handler with the dispatcher and executor.
func NewRouterDispatchHandler(dispatcher *dispatch.RouterDispatcher, execut *executor.Executor) *RouterDispatchHandler {
	return &RouterDispatchHandler{dispatcher: dispatcher, executor: execut}
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
