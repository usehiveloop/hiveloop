package tasks

import (
	"context"
	"fmt"

	"github.com/hibiken/asynq"

	"github.com/ziraloop/ziraloop/internal/sandbox"
)

// SystemAgentSyncHandler runs the periodic SystemAgentSync task. It ensures
// the singleton system sandbox exists and is running, then re-pushes every
// system agent definition into its Bridge. This catches three failure modes
// without per-request overhead:
//
//   - the system sandbox died and needs recreating
//   - the system sandbox restarted and Bridge lost loaded agent definitions
//   - a YAML system_prompt edit was deployed and Bridge has a stale definition
type SystemAgentSyncHandler struct {
	orchestrator *sandbox.Orchestrator
	pusher       *sandbox.Pusher
}

func NewSystemAgentSyncHandler(orchestrator *sandbox.Orchestrator, pusher *sandbox.Pusher) *SystemAgentSyncHandler {
	return &SystemAgentSyncHandler{orchestrator: orchestrator, pusher: pusher}
}

func (h *SystemAgentSyncHandler) Handle(ctx context.Context, _ *asynq.Task) error {
	sb, err := h.orchestrator.EnsureSystemSandbox(ctx)
	if err != nil {
		return fmt.Errorf("ensuring system sandbox: %w", err)
	}
	if err := h.pusher.PushAllSystemAgents(ctx, sb); err != nil {
		return fmt.Errorf("pushing system agents: %w", err)
	}
	return nil
}
