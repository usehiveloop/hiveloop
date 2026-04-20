package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"
	"gorm.io/gorm"

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

	slog.Info("agent cleanup: starting", "agent_id", payload.AgentID)

	var agent model.Agent
	if err := h.db.Where("id = ?", payload.AgentID).First(&agent).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			slog.Info("agent cleanup: agent already deleted", "agent_id", payload.AgentID)
			return nil
		}
		return fmt.Errorf("loading agent: %w", err)
	}

	if agent.SandboxType == "dedicated" {
		h.cleanupDedicatedSandboxes(ctx, &agent)
	} else if agent.SandboxType == "shared" {
		h.cleanupSharedAgent(ctx, &agent)
	}

	if err := h.db.Where("id = ?", agent.ID).Delete(&model.Agent{}).Error; err != nil {
		return fmt.Errorf("hard-deleting agent: %w", err)
	}

	slog.Info("agent cleanup: complete", "agent_id", agent.ID, "sandbox_type", agent.SandboxType)
	return nil
}

func (h *AgentCleanupHandler) cleanupDedicatedSandboxes(ctx context.Context, agent *model.Agent) {
	if h.orchestrator == nil {
		slog.Warn("agent cleanup: orchestrator not configured, skipping dedicated sandbox cleanup", "agent_id", agent.ID)
		return
	}

	var sandboxes []model.Sandbox
	if err := h.db.Where("agent_id = ?", agent.ID).Find(&sandboxes).Error; err != nil {
		slog.Error("agent cleanup: failed to find dedicated sandboxes", "agent_id", agent.ID, "error", err)
		return
	}

	for _, sb := range sandboxes {
		slog.Info("agent cleanup: destroying dedicated sandbox", "agent_id", agent.ID, "sandbox_id", sb.ID)
		if err := h.orchestrator.DeleteSandbox(ctx, &sb); err != nil {
			slog.Error("agent cleanup: failed to destroy dedicated sandbox", "agent_id", agent.ID, "sandbox_id", sb.ID, "error", err)
		}
	}
}

func (h *AgentCleanupHandler) cleanupSharedAgent(ctx context.Context, agent *model.Agent) {
	if h.pusher == nil {
		slog.Warn("agent cleanup: pusher not configured, skipping shared agent cleanup", "agent_id", agent.ID)
		return
	}

	if err := h.pusher.RemoveAgent(ctx, agent); err != nil {
		slog.Error("agent cleanup: failed to remove shared agent from bridge", "agent_id", agent.ID, "error", err)
	}
}
