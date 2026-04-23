package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"github.com/hibiken/asynq"
	"gorm.io/gorm"

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

	logger.Info("subscription dispatch: task received",
		"payload_bytes", len(payload.PayloadJSON),
		"raw_payload", string(payload.PayloadJSON),
	)

	var webhookPayload map[string]any
	if err := json.Unmarshal(payload.PayloadJSON, &webhookPayload); err != nil {
		logger.Error("subscription dispatch: failed to unmarshal webhook payload", "error", err)
		return fmt.Errorf("unmarshal webhook payload: %w", err)
	}
	logger.Info("subscription dispatch: payload decoded",
		"top_level_keys", topLevelKeys(webhookPayload),
	)

	resourceKey, ok := subscriptions.ResolveEventResourceKey(
		logger,
		handler.cat,
		payload.Provider,
		payload.EventType,
		payload.EventAction,
		webhookPayload,
	)
	if !ok {
		logger.Info("subscription dispatch: unresolvable resource_key, dropping event")
		return nil
	}

	logger = logger.With("resource_key", resourceKey)
	logger.Info("subscription dispatch: resource_key resolved, checking subscriptions")

	var subs []model.ConversationSubscription
	if err := handler.db.
		Where("org_id = ? AND resource_key = ? AND status = ?",
			payload.OrgID, resourceKey, model.SubscriptionStatusActive).
		Find(&subs).Error; err != nil {
		logger.Error("subscription dispatch: failed to load subscriptions", "error", err)
		return fmt.Errorf("load subscriptions: %w", err)
	}

	logger.Info("subscription dispatch: subscription query complete",
		"subscription_count", len(subs),
	)

	if len(subs) == 0 {
		logger.Info("subscription dispatch: no active subscriptions for resource_key, dropping")
		return nil
	}

	for index, sub := range subs {
		logger.Info("subscription dispatch: matched subscription",
			"match_index", index,
			"subscription_id", sub.ID,
			"conversation_id", sub.ConversationID,
			"agent_id", sub.AgentID,
			"resource_type", sub.ResourceType,
			"resource_id", sub.ResourceID,
			"source", sub.Source,
			"created_at", sub.CreatedAt,
		)
	}

	_, summaryRefs, _ := subscriptions.ResolveEventSummaryRefs(
		handler.cat,
		payload.Provider,
		payload.EventType,
		payload.EventAction,
		webhookPayload,
	)
	content, fullMessage := buildSubscriptionEventMessage(payload, resourceKey, summaryRefs, webhookPayload)
	logger.Info("subscription dispatch: outgoing message built",
		"content_bytes", len(content),
		"full_message_bytes", len(fullMessage),
		"summary_field_count", len(summaryRefs),
		"content_preview", previewString(content, 512),
	)

	logger.Info("subscription dispatch: fanning out",
		"subscription_count", len(subs),
	)

	var waitGroup sync.WaitGroup
	waitGroup.Add(len(subs))
	for _, sub := range subs {
		go func(sub model.ConversationSubscription) {
			defer waitGroup.Done()
			handler.deliverOne(ctx, logger, sub, content, fullMessage)
		}(sub)
	}
	waitGroup.Wait()

	logger.Info("subscription dispatch: fanout complete",
		"subscription_count", len(subs),
	)
	return nil
}

// deliverOne sends the event message into a single subscribed conversation.
// Errors are logged but not returned — one failed subscription must not block
// delivery to the others, and Asynq-level retries (whole-task retries) would
// re-deliver to every subscription, not just the failed one.
func (handler *SubscriptionDispatchHandler) deliverOne(
	ctx context.Context,
	logger *slog.Logger,
	sub model.ConversationSubscription,
	content string,
	fullMessage string,
) {
	subLogger := logger.With(
		"subscription_id", sub.ID,
		"conversation_id", sub.ConversationID,
		"agent_id", sub.AgentID,
	)

	subLogger.Info("subscription delivery: step 1 — loading conversation")
	var conv model.AgentConversation
	if err := handler.db.Where("id = ?", sub.ConversationID).First(&conv).Error; err != nil {
		subLogger.Error("subscription delivery: failed to load conversation", "error", err)
		return
	}
	subLogger.Info("subscription delivery: conversation loaded",
		"bridge_conversation_id", conv.BridgeConversationID,
		"sandbox_id", conv.SandboxID,
		"conversation_status", conv.Status,
		"conversation_name", conv.Name,
	)

	if conv.Status != "active" {
		subLogger.Info("subscription delivery: skipping inactive conversation",
			"conversation_status", conv.Status,
		)
		return
	}

	subLogger.Info("subscription delivery: step 2 — loading sandbox", "sandbox_id", conv.SandboxID)
	var sb model.Sandbox
	if err := handler.db.Where("id = ?", conv.SandboxID).First(&sb).Error; err != nil {
		subLogger.Error("subscription delivery: failed to load sandbox", "error", err)
		return
	}
	subLogger.Info("subscription delivery: sandbox loaded",
		"sandbox_id", sb.ID,
		"sandbox_status", sb.Status,
		"external_id", sb.ExternalID,
	)

	if sb.Status == "stopped" {
		subLogger.Info("subscription delivery: step 2b — sandbox stopped, waking",
			"sandbox_id", sb.ID,
		)
		woken, err := handler.orchestrator.WakeSandbox(ctx, &sb)
		if err != nil {
			subLogger.Error("subscription delivery: failed to wake sandbox",
				"sandbox_id", sb.ID, "error", err)
			return
		}
		sb = *woken
		subLogger.Info("subscription delivery: sandbox woken",
			"sandbox_id", sb.ID,
			"sandbox_status", sb.Status,
		)
	}

	subLogger.Info("subscription delivery: step 3 — getting bridge client", "sandbox_id", sb.ID)
	client, err := handler.orchestrator.GetBridgeClient(ctx, &sb)
	if err != nil {
		subLogger.Error("subscription delivery: failed to get bridge client",
			"sandbox_id", sb.ID, "error", err)
		return
	}
	subLogger.Info("subscription delivery: bridge client ready", "bridge_url", sb.BridgeURL)

	subLogger.Info("subscription delivery: step 4 — sending message",
		"bridge_conversation_id", conv.BridgeConversationID,
		"content_bytes", len(content),
		"full_message_bytes", len(fullMessage),
	)
	if err := client.SendMessageWithFullPayload(ctx, conv.BridgeConversationID, content, fullMessage); err != nil {
		subLogger.Error("subscription delivery: failed to send message",
			"bridge_conversation_id", conv.BridgeConversationID, "error", err)
		return
	}

	subLogger.Info("subscription delivery: event delivered successfully",
		"bridge_conversation_id", conv.BridgeConversationID,
		"content_bytes", len(content),
		"full_message_bytes", len(fullMessage),
	)
}
