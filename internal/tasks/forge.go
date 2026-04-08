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
