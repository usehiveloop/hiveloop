package tasks

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/sandbox"
)

// AgentCleanupHandler cleans up provider sandbox resources left behind after an agent hard delete.
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

	if len(payload.SandboxExternalIDs) > 0 {
		h.cleanupExternalSandboxes(ctx, payload.SandboxExternalIDs)
		return nil
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

	logging.FromContext(ctx).InfoContext(ctx, "agent cleanup complete", "agent_id", agent.ID)
	return nil
}

func (h *AgentCleanupHandler) cleanupExternalSandboxes(ctx context.Context, externalIDs []string) {
	if h.orchestrator == nil {
		return
	}
	for _, externalID := range externalIDs {
		if externalID == "" {
			continue
		}
		if err := h.orchestrator.DeleteSandboxExternal(ctx, externalID); err != nil {
			logging.Capture(ctx, fmt.Errorf("delete provider sandbox %s: %w", externalID, err))
		}
	}
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
