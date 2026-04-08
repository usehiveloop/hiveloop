package forge

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/model"
	"github.com/ziraloop/ziraloop/internal/streaming"
)

// Forge event types.
const (
	EventProvisioned        = "forge.provisioned"
	EventIterationStarted   = "forge.iteration_started"
	EventArchitectStarted   = "forge.architect_started"
	EventArchitectCompleted = "forge.architect_completed"
	EventEvalDesignStarted  = "forge.eval_design_started"
	EventEvalsGenerated     = "forge.evals_generated"
	EventEvalStarted        = "forge.eval_started"
	EventEvalToolCall       = "forge.eval_tool_call"
	EventEvalToolMock       = "forge.eval_tool_mock"
	EventEvalCompleted      = "forge.eval_completed"
	EventJudgeStarted       = "forge.judge_started"
	EventJudgeCompleted     = "forge.judge_completed"
	EventIterationCompleted = "forge.iteration_completed"
	EventCostUpdate         = "forge.cost_update"
	EventEvalsDesigned      = "forge.evals_designed"
	EventEvalsApproved      = "forge.evals_approved"
	EventRunCompleted       = "forge.run_completed"
	EventRunFailed          = "forge.run_failed"
	EventRunCancelled       = "forge.run_cancelled"
)

// eventEmitter handles dual-writing forge events: persisted to ForgeEvent table
// (for dashboard queries) AND published to Redis Streams (for real-time SSE).
type eventEmitter struct {
	db       *gorm.DB
	eventBus *streaming.EventBus
}

// emit writes a forge event to both DB and Redis Streams.
func (e *eventEmitter) emit(ctx context.Context, runID uuid.UUID, eventType string, data map[string]any) {
	payload, err := json.Marshal(data)
	if err != nil {
		slog.Error("forge: failed to marshal event payload",
			"forge_run_id", runID,
			"event_type", eventType,
			"error", err,
		)
		return
	}

	// 1. Persist to DB (source of truth for dashboard).
	event := model.ForgeEvent{
		ForgeRunID: runID,
		EventType:  eventType,
		Payload:    model.RawJSON(payload),
		CreatedAt:  time.Now(),
	}
	if err := e.db.Create(&event).Error; err != nil {
		slog.Error("forge: failed to persist event",
			"forge_run_id", runID,
			"event_type", eventType,
			"error", err,
		)
	}

	// 2. Publish to Redis Streams (real-time SSE for connected clients).
	if e.eventBus != nil {
		streamKey := "forge:" + runID.String()
		if _, err := e.eventBus.Publish(ctx, streamKey, eventType, json.RawMessage(payload)); err != nil {
			slog.Error("forge: failed to publish event to redis",
				"forge_run_id", runID,
				"event_type", eventType,
				"error", err,
			)
		}
	}
}
