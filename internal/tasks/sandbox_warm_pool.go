package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/hibiken/asynq"

	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
)

const sandboxWarmPoolReconcileTimeout = 10 * time.Minute

type SandboxWarmPoolReconcilePayload struct {
	ProviderID string `json:"provider_id"`
	Mode       string `json:"mode"`
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
		asynq.Unique(time.Minute),
		asynq.TaskID(fmt.Sprintf("sandbox-warm-pool-reconcile:%s:%s", payload.ProviderID, payload.Mode)),
	}
	return asynq.NewTask(TypeSandboxWarmPoolReconcile, body), opts, nil
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
}

func NewSandboxWarmPoolReconcileHandler(orchestrator *sandbox.Orchestrator) *SandboxWarmPoolReconcileHandler {
	return &SandboxWarmPoolReconcileHandler{orchestrator: orchestrator}
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
	return h.orchestrator.WarmPool().Reconcile(ctx, payload.Mode)
}
