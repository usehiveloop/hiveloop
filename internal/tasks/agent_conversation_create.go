package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/model"
	"github.com/ziraloop/ziraloop/internal/sandbox"
)

// AgentConversationCreateHandler provisions a dedicated sandbox, pushes the
// agent to Bridge, creates a conversation, and sends the first message.
type AgentConversationCreateHandler struct {
	db           *gorm.DB
	orchestrator *sandbox.Orchestrator
	pusher       *sandbox.Pusher
}

// NewAgentConversationCreateHandler creates the handler.
func NewAgentConversationCreateHandler(db *gorm.DB, orchestrator *sandbox.Orchestrator, pusher *sandbox.Pusher) *AgentConversationCreateHandler {
	return &AgentConversationCreateHandler{db: db, orchestrator: orchestrator, pusher: pusher}
}

// Handle processes a TypeAgentConversationCreate task.
func (handler *AgentConversationCreateHandler) Handle(ctx context.Context, task *asynq.Task) error {
	var payload AgentConversationCreatePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal payload: %w", err)
	}

	logger := slog.With(
		"delivery_id", payload.DeliveryID,
		"agent_id", payload.AgentID,
		"org_id", payload.OrgID,
	)

	// 1. Load agent.
	var agent model.Agent
	if err := handler.db.Where("id = ? AND deleted_at IS NULL", payload.AgentID).First(&agent).Error; err != nil {
		return fmt.Errorf("loading agent %s: %w", payload.AgentID, err)
	}

	logger = logger.With("agent_name", agent.Name, "sandbox_type", agent.SandboxType)
	logger.Info("creating conversation")

	// 2. Get or create sandbox.
	var sb *model.Sandbox
	if agent.SandboxType == "shared" && agent.SandboxID != nil {
		// Shared agent: reuse existing sandbox.
		var existing model.Sandbox
		if err := handler.db.Where("id = ?", *agent.SandboxID).First(&existing).Error; err != nil {
			return fmt.Errorf("loading shared sandbox: %w", err)
		}
		sb = &existing

		// Ensure agent is pushed (idempotent).
		if err := handler.pusher.PushAgentToSandbox(ctx, &agent, sb); err != nil {
			return fmt.Errorf("pushing shared agent to sandbox: %w", err)
		}
	} else {
		// Dedicated agent: create a new sandbox.
		logger.Info("creating dedicated sandbox")
		var err error
		sb, err = handler.orchestrator.CreateDedicatedSandbox(ctx, &agent)
		if err != nil {
			return fmt.Errorf("creating dedicated sandbox: %w", err)
		}
		logger.Info("dedicated sandbox created", "sandbox_id", sb.ID)

		// Push agent to the new sandbox.
		if err := handler.pusher.PushAgentToSandbox(ctx, &agent, sb); err != nil {
			return fmt.Errorf("pushing agent to dedicated sandbox: %w", err)
		}
		logger.Info("agent pushed to sandbox", "sandbox_id", sb.ID)
	}

	// 3. Get Bridge client.
	client, err := handler.orchestrator.GetBridgeClient(ctx, sb)
	if err != nil {
		return fmt.Errorf("getting bridge client: %w", err)
	}

	// 4. Create conversation.
	conv, err := client.CreateConversation(ctx, agent.ID.String())
	if err != nil {
		return fmt.Errorf("creating conversation: %w", err)
	}

	logger.Info("conversation created",
		"conversation_id", conv.ConversationId,
		"sandbox_id", sb.ID,
	)

	// 5. Store RouterConversation for thread affinity.
	if err := handler.db.Create(&model.RouterConversation{
		OrgID:                payload.OrgID,
		RouterTriggerID:      payload.RouterTriggerID,
		AgentID:              payload.AgentID,
		ConnectionID:         payload.ConnectionID,
		ResourceKey:          payload.ResourceKey,
		BridgeConversationID: conv.ConversationId,
		SandboxID:            sb.ID,
	}).Error; err != nil {
		slog.Error("failed to store router conversation", "error", err)
	}

	// 6. Send instructions as first message.
	if payload.Instructions != "" {
		if err := client.SendMessage(ctx, conv.ConversationId, payload.Instructions); err != nil {
			return fmt.Errorf("sending instructions: %w", err)
		}
		logger.Info("instructions sent",
			"conversation_id", conv.ConversationId,
			"instruction_bytes", len(payload.Instructions),
		)
	}

	logger.Info("conversation ready",
		"conversation_id", conv.ConversationId,
		"sandbox_id", sb.ID,
	)

	return nil
}
