package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/mcp/catalog"
	"github.com/ziraloop/ziraloop/internal/model"
	"github.com/ziraloop/ziraloop/internal/trigger/dispatch"
)

// TriggerDispatchHandler runs the trigger dispatcher for an incoming webhook.
// It loads the connection by ID, decodes the raw payload, and hands the
// envelope to dispatch.Dispatcher. Each non-skipped PreparedRun should be
// enqueued as a TypeAgentRun task by the executor in a follow-up PR — for now
// the handler just logs the decisions.
type TriggerDispatchHandler struct {
	db         *gorm.DB
	dispatcher *dispatch.Dispatcher
}

// NewTriggerDispatchHandler wires the dispatcher with a production GORM store.
// The catalog is the shared embedded singleton — there's nothing per-handler
// to configure.
func NewTriggerDispatchHandler(db *gorm.DB) *TriggerDispatchHandler {
	store := dispatch.NewGormAgentTriggerStore(db)
	dispatcher := dispatch.New(store, catalog.Global(), slog.Default())
	return &TriggerDispatchHandler{db: db, dispatcher: dispatcher}
}

// Handle runs one dispatch job. The execution path is intentionally tiny —
// the heavy lifting (trigger evaluation, ref extraction, request building)
// lives in dispatch.Dispatcher and is unit-tested separately.
func (h *TriggerDispatchHandler) Handle(ctx context.Context, task *asynq.Task) error {
	var payload TriggerDispatchPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal trigger dispatch payload: %w", err)
	}

	logger := slog.With(
		"task", "trigger_dispatch",
		"delivery_id", payload.DeliveryID,
		"provider", payload.Provider,
		"event_type", payload.EventType,
		"event_action", payload.EventAction,
		"org_id", payload.OrgID,
		"connection_id", payload.ConnectionID,
	)
	logger.Info("trigger dispatch: starting")

	// Reload the connection from the DB so we have the freshest revoked_at /
	// integration state. Webhook bodies are short-lived; the connection is the
	// authoritative source of truth.
	var connection model.Connection
	if err := h.db.WithContext(ctx).
		Preload("Integration").
		Where("id = ? AND org_id = ? AND revoked_at IS NULL", payload.ConnectionID, payload.OrgID).
		First(&connection).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			logger.Warn("trigger dispatch: connection not found or revoked, dropping webhook")
			return nil // ack the task — no point retrying a missing connection
		}
		return fmt.Errorf("loading connection: %w", err)
	}

	var webhookPayload map[string]any
	if err := json.Unmarshal(payload.PayloadJSON, &webhookPayload); err != nil {
		return fmt.Errorf("decoding webhook payload: %w", err)
	}

	input := dispatch.DispatchInput{
		Provider:    payload.Provider,
		EventType:   payload.EventType,
		EventAction: payload.EventAction,
		Payload:     webhookPayload,
		DeliveryID:  payload.DeliveryID,
		OrgID:       payload.OrgID,
		Connection:  &connection,
	}

	runs, err := h.dispatcher.Run(ctx, input)
	if err != nil {
		return fmt.Errorf("dispatcher run: %w", err)
	}

	// Tally outcomes for the structured log line. The actual run_agent enqueue
	// happens in the next PR; for now we just summarize.
	enqueueable := 0
	for _, run := range runs {
		if !run.Skipped() {
			enqueueable++
		}
	}
	logger.Info("trigger dispatch: complete",
		"runs_total", len(runs),
		"runs_enqueueable", enqueueable,
		"runs_skipped", len(runs)-enqueueable,
	)

	// TODO(executor): for each non-skipped run, enqueue a TypeAgentRun task
	// here once the executor handler exists. The PreparedRun struct is the
	// payload — it carries everything the executor needs (resolved refs,
	// substituted instructions, ordered context requests, sandbox strategy).

	return nil
}
