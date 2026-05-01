package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/logging"
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
			logging.Capture(cleanupCtx, fmt.Errorf("persist failed event: %w", err))
		}
	}()

	var agent model.Agent
	if err := handler.db.Where("id = ? AND deleted_at IS NULL", payload.AgentID).First(&agent).Error; err != nil {
		return fmt.Errorf("loading agent %s: %w", payload.AgentID, err)
	}

	sb, err := handler.orchestrator.CreateDedicatedSandbox(ctx, &agent)
	if err != nil {
		return fmt.Errorf("creating dedicated sandbox: %w", err)
	}
	rollbacks = append(rollbacks, func(cleanupCtx context.Context) {
		if err := handler.orchestrator.DeleteSandbox(cleanupCtx, sb); err != nil {
			logging.Capture(cleanupCtx, fmt.Errorf("rollback delete sandbox %s: %w", sb.ID, err))
		}
	})

	if err := handler.pusher.PushAgentToSandbox(ctx, &agent, sb); err != nil {
		return fmt.Errorf("pushing agent to dedicated sandbox: %w", err)
	}

	client, err := handler.orchestrator.GetBridgeClient(ctx, sb)
	if err != nil {
		return fmt.Errorf("getting bridge client: %w", err)
	}

	conv, err := client.CreateConversation(ctx, agent.ID.String())
	if err != nil {
		return fmt.Errorf("creating conversation: %w", err)
	}

	agentConv := model.AgentConversation{
		OrgID:                payload.OrgID,
		AgentID:              payload.AgentID,
		SandboxID:            sb.ID,
		BridgeConversationID: conv.ConversationId,
		Status:               "active",
	}
	if err := handler.db.Create(&agentConv).Error; err != nil {
		return fmt.Errorf("storing agent conversation: %w", err)
	}
	rollbacks = append(rollbacks, func(cleanupCtx context.Context) {
		if err := handler.db.WithContext(cleanupCtx).
			Where("id = ?", agentConv.ID).
			Delete(&model.AgentConversation{}).Error; err != nil {
			logging.Capture(cleanupCtx, fmt.Errorf("rollback delete agent conversation %s: %w", agentConv.ID, err))
		}
	})

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
		// Non-fatal: best-effort thread affinity.
		logging.Capture(ctx, fmt.Errorf("store router conversation: %w", err))
	} else {
		rollbacks = append(rollbacks, func(cleanupCtx context.Context) {
			if err := handler.db.WithContext(cleanupCtx).
				Where("id = ?", routerConv.ID).
				Delete(&model.RouterConversation{}).Error; err != nil {
				logging.Capture(cleanupCtx, fmt.Errorf("rollback delete router conversation %s: %w", routerConv.ID, err))
			}
		})
	}

	if payload.Instructions != "" {
		if err := client.SendMessage(ctx, conv.ConversationId, payload.Instructions); err != nil {
			return fmt.Errorf("sending instructions: %w", err)
		}
	}

	return nil
}
