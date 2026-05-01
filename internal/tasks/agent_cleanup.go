package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/sandbox"
)

// AgentCleanupHandler cleans up an agent's sandbox resources and then hard-deletes it.
type AgentCleanupHandler struct {
	db           *gorm.DB
	orchestrator *sandbox.Orchestrator
	pusher       *sandbox.Pusher
}

// NewAgentCleanupHandler creates a new agent cleanup handler.
func NewAgentCleanupHandler(db *gorm.DB, orchestrator *sandbox.Orchestrator, pusher *sandbox.Pusher) *AgentCleanupHandler {
	return &AgentCleanupHandler{db: db, orchestrator: orchestrator, pusher: pusher}
}

// Handle processes an agent:cleanup task.
func (h *AgentCleanupHandler) Handle(ctx context.Context, t *asynq.Task) error {
	var payload AgentCleanupPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal agent cleanup payload: %w", err)
	}

	var agent model.Agent
	if err := h.db.Where("id = ?", payload.AgentID).First(&agent).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil
		}
		return fmt.Errorf("loading agent: %w", err)
	}

	h.cleanupAgentSandboxes(ctx, &agent)

	if err := h.db.Where("id = ?", agent.ID).Delete(&model.Agent{}).Error; err != nil {
		return fmt.Errorf("hard-deleting agent: %w", err)
	}

	slog.Info("agent cleanup complete", "agent_id", agent.ID)
	return nil
}

func (h *AgentCleanupHandler) cleanupAgentSandboxes(ctx context.Context, agent *model.Agent) {
	if h.orchestrator == nil {
		return
	}

	var sandboxes []model.Sandbox
	if err := h.db.Where("agent_id = ?", agent.ID).Find(&sandboxes).Error; err != nil {
		logging.Capture(ctx, fmt.Errorf("find sandboxes for agent %s: %w", agent.ID, err))
		return
	}

	for _, sb := range sandboxes {
		if err := h.orchestrator.DeleteSandbox(ctx, &sb); err != nil {
			logging.Capture(ctx, fmt.Errorf("delete sandbox %s for agent %s: %w", sb.ID, agent.ID, err))
		}
	}
}
