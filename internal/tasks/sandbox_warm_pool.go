package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
)

const (
	sandboxWarmPoolReconcileTimeout = 10 * time.Minute
	sandboxWarmSlotCheckTimeout     = 30 * time.Second
	sandboxWarmSlotCheckDelay       = 45 * time.Second
	sandboxWarmSlotMaxChecks        = 20
)

type SandboxWarmPoolReconcilePayload struct {
	ProviderID string `json:"provider_id"`
	Mode       string `json:"mode"`
}

type SandboxWarmSlotCheckPayload struct {
	ProviderID string    `json:"provider_id"`
	SlotID     uuid.UUID `json:"slot_id"`
	Attempt    int       `json:"attempt"`
}

func NewSandboxWarmPoolReconcileTask(payload SandboxWarmPoolReconcilePayload) (*asynq.Task, []asynq.Option, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, err
	}
	opts := []asynq.Option{
		asynq.Queue(QueueDefault),
		asynq.MaxRetry(3),
		asynq.Timeout(sandboxWarmPoolReconcileTimeout),
		asynq.Unique(10 * time.Second),
	}
	return asynq.NewTask(TypeSandboxWarmPoolReconcile, body), opts, nil
}

func NewSandboxWarmSlotCheckTask(payload SandboxWarmSlotCheckPayload) (*asynq.Task, []asynq.Option, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, err
	}
	opts := []asynq.Option{
		asynq.Queue(QueueDefault),
		asynq.MaxRetry(5),
		asynq.Timeout(sandboxWarmSlotCheckTimeout),
	}
	return asynq.NewTask(TypeSandboxWarmSlotCheck, body), opts, nil
}

func EnqueueSandboxWarmPoolReconcile(ctx context.Context, enqueuer enqueue.TaskEnqueuer, providerID, mode string) error {
	if enqueuer == nil {
		return nil
	}
	task, opts, err := NewSandboxWarmPoolReconcileTask(SandboxWarmPoolReconcilePayload{
		ProviderID: providerID,
		Mode:       mode,
	})
	if err != nil {
		return err
	}
	if _, err := enqueuer.EnqueueContext(ctx, task, opts...); err != nil {
		if errors.Is(err, asynq.ErrDuplicateTask) || errors.Is(err, asynq.ErrTaskIDConflict) {
			return nil
		}
		return err
	}
	return nil
}

func EnqueueSandboxWarmSlotCheck(ctx context.Context, enqueuer enqueue.TaskEnqueuer, providerID string, slotID uuid.UUID, attempt int, delay time.Duration) error {
	if enqueuer == nil {
		return nil
	}
	task, opts, err := NewSandboxWarmSlotCheckTask(SandboxWarmSlotCheckPayload{
		ProviderID: providerID,
		SlotID:     slotID,
		Attempt:    attempt,
	})
	if err != nil {
		return err
	}
	if delay > 0 {
		opts = append(opts, asynq.ProcessIn(delay))
	}
	opts = append(opts, asynq.TaskID(fmt.Sprintf("sandbox-warm-slot-check:%s:%d", slotID, attempt)))
	if _, err := enqueuer.EnqueueContext(ctx, task, opts...); err != nil {
		if errors.Is(err, asynq.ErrDuplicateTask) || errors.Is(err, asynq.ErrTaskIDConflict) {
			return nil
		}
		return err
	}
	return nil
}

func EnqueueConfiguredWarmPoolReconciles(ctx context.Context, enqueuer enqueue.TaskEnqueuer, orchestrator *sandbox.Orchestrator) {
	if orchestrator == nil || orchestrator.WarmPool() == nil {
		return
	}
	for _, mode := range []string{model.SandboxWarmSlotModeEmployee, model.SandboxWarmSlotModeSpecialist} {
		if orchestrator.WarmPool().DesiredCount(mode) > 0 {
			_ = EnqueueSandboxWarmPoolReconcile(ctx, enqueuer, orchestrator.ProviderID(), mode)
		}
	}
}

type SandboxWarmPoolReconcileHandler struct {
	orchestrator *sandbox.Orchestrator
	enqueuer     enqueue.TaskEnqueuer
}

func NewSandboxWarmPoolReconcileHandler(orchestrator *sandbox.Orchestrator, enqueuer enqueue.TaskEnqueuer) *SandboxWarmPoolReconcileHandler {
	return &SandboxWarmPoolReconcileHandler{orchestrator: orchestrator, enqueuer: enqueuer}
}

func (h *SandboxWarmPoolReconcileHandler) Handle(ctx context.Context, task *asynq.Task) error {
	if h.orchestrator == nil || h.orchestrator.WarmPool() == nil {
		return nil
	}
	var payload SandboxWarmPoolReconcilePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return err
	}
	if payload.ProviderID != h.orchestrator.ProviderID() {
		return nil
	}
	_, err := h.orchestrator.WarmPool().Reconcile(ctx, payload.Mode, func(ctx context.Context, slotID uuid.UUID) error {
		if err := EnqueueSandboxWarmSlotCheck(ctx, h.enqueuer, payload.ProviderID, slotID, 1, sandboxWarmSlotCheckDelay); err != nil {
			return fmt.Errorf("enqueue warm slot check: %w", err)
		}
		return nil
	})
	return err
}

type SandboxWarmSlotCheckHandler struct {
	orchestrator *sandbox.Orchestrator
	enqueuer     enqueue.TaskEnqueuer
}

func NewSandboxWarmSlotCheckHandler(orchestrator *sandbox.Orchestrator, enqueuer enqueue.TaskEnqueuer) *SandboxWarmSlotCheckHandler {
	return &SandboxWarmSlotCheckHandler{orchestrator: orchestrator, enqueuer: enqueuer}
}

func (h *SandboxWarmSlotCheckHandler) Handle(ctx context.Context, task *asynq.Task) error {
	if h.orchestrator == nil || h.orchestrator.WarmPool() == nil {
		return nil
	}
	var payload SandboxWarmSlotCheckPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return err
	}
	if payload.ProviderID != h.orchestrator.ProviderID() {
		return nil
	}
	result, err := h.orchestrator.WarmPool().CheckWarmSlot(ctx, payload.SlotID)
	if err != nil {
		return err
	}
	if result == nil || !result.Pending {
		return nil
	}
	if payload.Attempt >= sandboxWarmSlotMaxChecks {
		_ = h.orchestrator.WarmPool().MarkError(ctx, payload.SlotID,
			fmt.Sprintf("warm slot did not become ready after %d checks", payload.Attempt))
		if mode, modeErr := h.orchestrator.WarmPool().SlotMode(ctx, payload.SlotID); modeErr == nil {
			_ = EnqueueSandboxWarmPoolReconcile(ctx, h.enqueuer, payload.ProviderID, mode)
		}
		return fmt.Errorf("warm slot %s did not become ready after %d checks", payload.SlotID, payload.Attempt)
	}
	return EnqueueSandboxWarmSlotCheck(ctx, h.enqueuer, payload.ProviderID, payload.SlotID, payload.Attempt+1, sandboxWarmSlotCheckDelay)
}
