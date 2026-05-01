package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/mcp/catalog"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/sandbox"
	"github.com/usehiveloop/hiveloop/internal/subscriptions"
)

// SubscriptionDispatchHandler forwards a webhook event into every active
// conversation_subscription whose resource_key matches the event.
//
// The flow is:
//  1. Resolve the event's canonical resource_key from the catalog's trigger def.
//  2. Find all active conversation_subscriptions with that (org_id, resource_key).
//  3. For each match, wake the sandbox (if needed), get the Bridge client, and
//     send a short event-summary message into the existing bridge conversation.
//
// Delivery is best-effort per subscription: a failure on one subscription must
// not prevent delivery to the others. Retries are handled by Asynq at the task
// level — if the handler returns an error, the whole task is retried, which is
// acceptable because asynq.Unique deduplicates by delivery_id.
type SubscriptionDispatchHandler struct {
	db           *gorm.DB
	orchestrator *sandbox.Orchestrator
	cat          *catalog.Catalog
}

// NewSubscriptionDispatchHandler wires the handler with the dependencies it
// needs to resolve the resource_key, look up matching subscriptions, and
// deliver messages to existing Bridge conversations.
func NewSubscriptionDispatchHandler(db *gorm.DB, orchestrator *sandbox.Orchestrator, cat *catalog.Catalog) *SubscriptionDispatchHandler {
	return &SubscriptionDispatchHandler{db: db, orchestrator: orchestrator, cat: cat}
}

// Handle processes a TypeSubscriptionDispatch task.
func (handler *SubscriptionDispatchHandler) Handle(ctx context.Context, task *asynq.Task) error {
	var payload SubscriptionDispatchPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal subscription dispatch payload: %w", err)
	}

	logger := slog.With(
		"component", "subscription_dispatch",
		"delivery_id", payload.DeliveryID,
		"org_id", payload.OrgID,
		"provider", payload.Provider,
		"event", payload.EventType+"."+payload.EventAction,
		"connection_id", payload.ConnectionID,
	)

	var webhookPayload map[string]any
	if err := json.Unmarshal(payload.PayloadJSON, &webhookPayload); err != nil {
		return fmt.Errorf("unmarshal webhook payload: %w", err)
	}

	resourceKey, ok := subscriptions.ResolveEventResourceKey(
		logger,
		handler.cat,
		payload.Provider,
		payload.EventType,
		payload.EventAction,
		webhookPayload,
	)
	if !ok {
		return nil
	}

	logger = logger.With("resource_key", resourceKey)

	var subs []model.ConversationSubscription
	if err := handler.db.
		Where("org_id = ? AND resource_key = ? AND status = ?",
			payload.OrgID, resourceKey, model.SubscriptionStatusActive).
		Find(&subs).Error; err != nil {
		return fmt.Errorf("load subscriptions: %w", err)
	}

	if len(subs) == 0 {
		return nil
	}

	_, summaryRefs, _ := subscriptions.ResolveEventSummaryRefs(
		handler.cat,
		payload.Provider,
		payload.EventType,
		payload.EventAction,
		webhookPayload,
	)
	content, fullMessage := buildSubscriptionEventMessage(payload, resourceKey, summaryRefs, webhookPayload)

	var waitGroup sync.WaitGroup
	waitGroup.Add(len(subs))
	for _, sub := range subs {
		go func(sub model.ConversationSubscription) {
			defer waitGroup.Done()
			handler.deliverOne(ctx, logger, sub, content, fullMessage)
		}(sub)
	}
	waitGroup.Wait()

	return nil
}

// deliverOne sends the event message into a single subscribed conversation.
// Errors are captured but not returned — one failed subscription must not block
// delivery to the others, and Asynq-level retries (whole-task retries) would
// re-deliver to every subscription, not just the failed one.
func (handler *SubscriptionDispatchHandler) deliverOne(
	ctx context.Context,
	logger *slog.Logger,
	sub model.ConversationSubscription,
	content string,
	fullMessage string,
) {
	var conv model.AgentConversation
	if err := handler.db.Where("id = ?", sub.ConversationID).First(&conv).Error; err != nil {
		logging.Capture(ctx, fmt.Errorf("load conversation %s: %w", sub.ConversationID, err))
		return
	}

	if conv.Status != "active" {
		return
	}

	var sb model.Sandbox
	if err := handler.db.Where("id = ?", conv.SandboxID).First(&sb).Error; err != nil {
		logging.Capture(ctx, fmt.Errorf("load sandbox %s: %w", conv.SandboxID, err))
		return
	}

	if sb.Status == "stopped" {
		woken, err := handler.orchestrator.WakeSandbox(ctx, &sb)
		if err != nil {
			logging.Capture(ctx, fmt.Errorf("wake sandbox %s: %w", sb.ID, err))
			return
		}
		sb = *woken
	}

	client, err := handler.orchestrator.GetBridgeClient(ctx, &sb)
	if err != nil {
		logging.Capture(ctx, fmt.Errorf("get bridge client for sandbox %s: %w", sb.ID, err))
		return
	}

	if err := client.SendMessageWithFullPayload(ctx, conv.BridgeConversationID, content, fullMessage); err != nil {
		logging.Capture(ctx, fmt.Errorf("send message to bridge conversation %s: %w", conv.BridgeConversationID, err))
		return
	}
}
