package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/sandbox"
)

const cleanupTimeout = 30 * time.Second

func init() {
	RegisterTaskBuilder(TypeAgentConversationCreate, func(payload []byte) (*asynq.Task, error) {
		var p AgentConversationCreatePayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return nil, fmt.Errorf("unmarshal agent conversation create payload: %w", err)
		}
		return NewAgentConversationCreateTask(p)
	})
}

type AgentConversationCreateHandler struct {
	db           *gorm.DB
	orchestrator *sandbox.Orchestrator
	pusher       *sandbox.Pusher
}

func NewAgentConversationCreateHandler(db *gorm.DB, orchestrator *sandbox.Orchestrator, pusher *sandbox.Pusher) *AgentConversationCreateHandler {
	return &AgentConversationCreateHandler{db: db, orchestrator: orchestrator, pusher: pusher}
}

func (handler *AgentConversationCreateHandler) Handle(ctx context.Context, task *asynq.Task) (handlerErr error) {
	var payload AgentConversationCreatePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal payload: %w", err)
	}

	logger := slog.With(
		"delivery_id", payload.DeliveryID,
		"agent_id", payload.AgentID,
		"org_id", payload.OrgID,
	)

	var rollbacks []func(context.Context)
	defer func() {
		if handlerErr == nil {
			return
		}
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
		defer cancel()
		for i := len(rollbacks) - 1; i >= 0; i-- {
			rollbacks[i](cleanupCtx)
		}
		retryCount, _ := asynq.GetRetryCount(ctx)
		maxRetry, _ := asynq.GetMaxRetry(ctx)
		if retryCount < maxRetry {
			return
		}
		if err := PersistTerminalFailure(cleanupCtx, handler.db, FailedEventInput{
			OrgID:        payload.OrgID,
			TriggerID:    payload.RouterTriggerID,
			EventType:    TypeAgentConversationCreate,
			Payload:      task.Payload(),
			Err:          handlerErr,
			AttemptCount: retryCount + 1,
		}); err != nil {
			logger.Error("cleanup: failed to persist failed event", "error", err.Error())
		}
	}()

	logger.Info("step 1: loading agent")
	var agent model.Agent
	if err := handler.db.Where("id = ? AND deleted_at IS NULL", payload.AgentID).First(&agent).Error; err != nil {
		return fmt.Errorf("loading agent %s: %w", payload.AgentID, err)
	}

	logger = logger.With("agent_name", agent.Name)
	logger.Info("step 1: agent loaded",
		"model", agent.Model,
		"has_credential", agent.CredentialID != nil,
		"integration_count", len(agent.Integrations),
		"setup_commands", len(agent.SetupCommands),
		"has_encrypted_env_vars", len(agent.EncryptedEnvVars) > 0,
		"sandbox_template_id", agent.SandboxTemplateID,
	)

	logger.Info("step 2: creating dedicated sandbox",
		"setup_commands", agent.SetupCommands,
		"has_encrypted_env_vars", len(agent.EncryptedEnvVars) > 0,
		"sandbox_template_id", agent.SandboxTemplateID,
	)
	sb, err := handler.orchestrator.CreateDedicatedSandbox(ctx, &agent)
	if err != nil {
		logger.Error("step 2: FAILED to create dedicated sandbox", "error", err.Error())
		return fmt.Errorf("creating dedicated sandbox: %w", err)
	}
	rollbacks = append(rollbacks, func(cleanupCtx context.Context) {
		if err := handler.orchestrator.DeleteSandbox(cleanupCtx, sb); err != nil {
			logger.Error("cleanup: failed to delete sandbox",
				"error", err.Error(),
				"sandbox_id", sb.ID,
				"external_id", sb.ExternalID,
			)
			return
		}
		logger.Info("cleanup: sandbox deleted", "sandbox_id", sb.ID, "external_id", sb.ExternalID)
	})
	logger.Info("step 2: dedicated sandbox created",
		"sandbox_id", sb.ID,
		"external_id", sb.ExternalID,
		"bridge_url", sb.BridgeURL,
		"status", sb.Status,
	)

	logger.Info("step 3: pushing agent to dedicated sandbox")
	if err := handler.pusher.PushAgentToSandbox(ctx, &agent, sb); err != nil {
		logger.Error("step 3: FAILED to push agent to sandbox",
			"error", err.Error(),
			"sandbox_id", sb.ID,
		)
		return fmt.Errorf("pushing agent to dedicated sandbox: %w", err)
	}
	logger.Info("step 3: agent pushed to dedicated sandbox", "sandbox_id", sb.ID)

	logger.Info("step 4: getting bridge client", "sandbox_id", sb.ID)
	client, err := handler.orchestrator.GetBridgeClient(ctx, sb)
	if err != nil {
		logger.Error("step 4: FAILED to get bridge client",
			"error", err.Error(),
			"sandbox_id", sb.ID,
			"bridge_url", sb.BridgeURL,
		)
		return fmt.Errorf("getting bridge client: %w", err)
	}
	logger.Info("step 4: bridge client ready", "sandbox_id", sb.ID)

	logger.Info("step 5: creating conversation", "agent_id", agent.ID, "sandbox_id", sb.ID)
	conv, err := client.CreateConversation(ctx, agent.ID.String())
	if err != nil {
		logger.Error("step 5: FAILED to create conversation",
			"error", err.Error(),
			"agent_id", agent.ID,
			"sandbox_id", sb.ID,
		)
		return fmt.Errorf("creating conversation: %w", err)
	}
	logger.Info("step 5: conversation created",
		"conversation_id", conv.ConversationId,
		"sandbox_id", sb.ID,
	)

	agentConv := model.AgentConversation{
		OrgID:                payload.OrgID,
		AgentID:              payload.AgentID,
		SandboxID:            sb.ID,
		BridgeConversationID: conv.ConversationId,
		Status:               "active",
	}
	if err := handler.db.Create(&agentConv).Error; err != nil {
		logger.Error("step 5b: FAILED to store agent conversation",
			"error", err.Error(),
			"conversation_id", conv.ConversationId,
		)
		return fmt.Errorf("storing agent conversation: %w", err)
	}
	rollbacks = append(rollbacks, func(cleanupCtx context.Context) {
		if err := handler.db.WithContext(cleanupCtx).
			Where("id = ?", agentConv.ID).
			Delete(&model.AgentConversation{}).Error; err != nil {
			logger.Error("cleanup: failed to delete agent conversation",
				"error", err.Error(),
				"agent_conversation_id", agentConv.ID,
			)
			return
		}
		logger.Info("cleanup: agent conversation deleted", "agent_conversation_id", agentConv.ID)
	})
	logger.Info("step 5b: agent conversation stored",
		"agent_conversation_id", agentConv.ID,
		"bridge_conversation_id", conv.ConversationId,
		"sandbox_id", sb.ID,
	)

	logger.Info("step 6: storing router conversation",
		"conversation_id", conv.ConversationId,
		"router_trigger_id", payload.RouterTriggerID,
		"connection_id", payload.ConnectionID,
		"resource_key", payload.ResourceKey,
	)
	routerConv := model.RouterConversation{
		OrgID:                payload.OrgID,
		RouterTriggerID:      payload.RouterTriggerID,
		AgentID:              payload.AgentID,
		ConnectionID:         payload.ConnectionID,
		ResourceKey:          payload.ResourceKey,
		BridgeConversationID: conv.ConversationId,
		SandboxID:            sb.ID,
	}
	if err := handler.db.Create(&routerConv).Error; err != nil {
		logger.Error("step 6: FAILED to store router conversation",
			"error", err.Error(),
			"conversation_id", conv.ConversationId,
		)
	} else {
		rollbacks = append(rollbacks, func(cleanupCtx context.Context) {
			if err := handler.db.WithContext(cleanupCtx).
				Where("id = ?", routerConv.ID).
				Delete(&model.RouterConversation{}).Error; err != nil {
				logger.Error("cleanup: failed to delete router conversation",
					"error", err.Error(),
					"router_conversation_id", routerConv.ID,
				)
				return
			}
			logger.Info("cleanup: router conversation deleted", "router_conversation_id", routerConv.ID)
		})
		logger.Info("step 6: router conversation stored", "conversation_id", conv.ConversationId)
	}

	if payload.Instructions != "" {
		logger.Info("step 7: sending instructions",
			"conversation_id", conv.ConversationId,
			"instruction_bytes", len(payload.Instructions),
		)
		if err := client.SendMessage(ctx, conv.ConversationId, payload.Instructions); err != nil {
			logger.Error("step 7: FAILED to send instructions",
				"error", err.Error(),
				"conversation_id", conv.ConversationId,
			)
			return fmt.Errorf("sending instructions: %w", err)
		}
		logger.Info("step 7: instructions sent",
			"conversation_id", conv.ConversationId,
			"instruction_bytes", len(payload.Instructions),
		)
	} else {
		logger.Info("step 7: no instructions to send")
	}

	logger.Info("conversation ready",
		"conversation_id", conv.ConversationId,
		"sandbox_id", sb.ID,
		"agent_id", agent.ID,
	)

	return nil
}
