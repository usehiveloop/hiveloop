package tasks

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

// ForgeExecuteFunc is a function that executes a forge run.
// This avoids importing the forge package (which would create an import cycle).
type ForgeExecuteFunc func(ctx context.Context, runID uuid.UUID)

// ForgeRunHandler processes forge:run tasks.
type ForgeRunHandler struct {
	execute ForgeExecuteFunc
}

// NewForgeRunHandler creates a forge run handler.
func NewForgeRunHandler(execute ForgeExecuteFunc) *ForgeRunHandler {
	return &ForgeRunHandler{execute: execute}
}

// Handle executes a forge run.
func (h *ForgeRunHandler) Handle(ctx context.Context, t *asynq.Task) error {
	var p ForgeRunPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("unmarshal forge run payload: %w", err)
	}

	h.execute(ctx, p.RunID)
	return nil
}

// ForgeDesignEvalsFunc is a function that generates eval cases for a forge run.
type ForgeDesignEvalsFunc func(ctx context.Context, runID uuid.UUID)

// ForgeDesignEvalsHandler processes forge:design_evals tasks.
type ForgeDesignEvalsHandler struct {
	execute ForgeDesignEvalsFunc
}

// NewForgeDesignEvalsHandler creates a forge design evals handler.
func NewForgeDesignEvalsHandler(execute ForgeDesignEvalsFunc) *ForgeDesignEvalsHandler {
	return &ForgeDesignEvalsHandler{execute: execute}
}

// Handle generates eval cases for a forge run.
func (h *ForgeDesignEvalsHandler) Handle(ctx context.Context, t *asynq.Task) error {
	var p ForgeDesignEvalsPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("unmarshal forge design evals payload: %w", err)
	}

	h.execute(ctx, p.RunID)
	return nil
}

// ForgeEvalJudgeFunc runs one eval case end-to-end (eval target → judge → save).
type ForgeEvalJudgeFunc func(ctx context.Context, payload ForgeEvalJudgePayload)

// ForgeEvalJudgeHandler processes forge:eval_judge tasks.
type ForgeEvalJudgeHandler struct {
	execute ForgeEvalJudgeFunc
}

// NewForgeEvalJudgeHandler creates a forge eval judge handler.
func NewForgeEvalJudgeHandler(execute ForgeEvalJudgeFunc) *ForgeEvalJudgeHandler {
	return &ForgeEvalJudgeHandler{execute: execute}
}

// Handle runs one eval case and judges it.
func (h *ForgeEvalJudgeHandler) Handle(ctx context.Context, t *asynq.Task) error {
	var p ForgeEvalJudgePayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("unmarshal forge eval judge payload: %w", err)
	}

	h.execute(ctx, p)
	return nil
}
