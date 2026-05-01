package handler

import (
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/enqueue"
	"github.com/usehiveloop/hiveloop/internal/mcp/catalog"
	"github.com/usehiveloop/hiveloop/internal/tasks"
)

func dispatchWebhookEvent(
	enqueuer enqueue.TaskEnqueuer,
	wh *nangoWebhook,
	wctx *webhookContext,
) {
	if enqueuer == nil || wctx == nil || wctx.inConnection == nil {
		return
	}
	if wh.Type != "forward" || len(wh.Payload) == 0 {
		return
	}

	providerName := wctx.inConnection.InIntegration.Provider

	metadata, ok := extractEventMetadata(wh, providerName)
	if !ok {
		return
	}

	deliveryID := wh.ConnectionID + ":" + uuid.New().String()

	enqueueTriggerDispatch(enqueuer, providerName, metadata, deliveryID, wctx)
	enqueueSubscriptionDispatch(enqueuer, providerName, metadata, deliveryID, wctx)
}

type eventMetadata struct {
	EventType   string
	EventAction string
	RawBody     []byte
	Headers     map[string]string
}

func extractEventMetadata(wh *nangoWebhook, providerName string) (eventMetadata, bool) {
	rawBody, headers := unwrapNangoPayload(wh.Payload)

	eventType, eventAction := inferEventFromHeaders(providerName, headers)
	if eventType == "" {
		if providerName == "github" || strings.HasPrefix(providerName, "github") {
			eventType, eventAction = inferGitHubEventFromPayload(rawBody)
		}
	}
	if eventType == "" {
		return eventMetadata{}, false
	}

	if eventAction == "" && (providerName == "github" || strings.HasPrefix(providerName, "github")) {
		var probe struct {
			Action string `json:"action"`
		}
		_ = json.Unmarshal(rawBody, &probe)
		eventAction = probe.Action
	}

	return eventMetadata{
		EventType:   eventType,
		EventAction: eventAction,
		RawBody:     rawBody,
		Headers:     headers,
	}, true
}

func enqueueTriggerDispatch(
	enqueuer enqueue.TaskEnqueuer,
	providerName string,
	metadata eventMetadata,
	deliveryID string,
	wctx *webhookContext,
) {
	cat := catalog.Global()
	if !cat.HasTriggers(providerName) {
		if _, ok := cat.GetProviderTriggersForVariant(providerName); !ok {
			return
		}
	}

	task, err := tasks.NewRouterDispatchTask(tasks.TriggerDispatchPayload{
		Provider:     providerName,
		EventType:    metadata.EventType,
		EventAction:  metadata.EventAction,
		DeliveryID:   deliveryID,
		OrgID:        wctx.orgID,
		ConnectionID: wctx.inConnection.ID,
		PayloadJSON:  metadata.RawBody,
	})
	if err != nil {
		slog.Error("trigger dispatch: failed to build task",
			"delivery_id", deliveryID, "error", err,
		)
		return
	}
	if _, err := enqueuer.Enqueue(task); err != nil {
		slog.Error("trigger dispatch: failed to enqueue task",
			"delivery_id", deliveryID, "error", err,
		)
		return
	}
}

func enqueueSubscriptionDispatch(
	enqueuer enqueue.TaskEnqueuer,
	providerName string,
	metadata eventMetadata,
	deliveryID string,
	wctx *webhookContext,
) {
	cat := catalog.Global()
	hasTriggers := cat.HasTriggers(providerName)
	_, hasVariant := cat.GetProviderTriggersForVariant(providerName)
	if !hasTriggers && !hasVariant {
		return
	}

	logger := slog.With(
		"component", "subscription_dispatch_enqueue",
		"delivery_id", deliveryID,
		"provider", providerName,
		"event_type", metadata.EventType,
		"event_action", metadata.EventAction,
		"org_id", wctx.orgID,
	)

	task, err := tasks.NewSubscriptionDispatchTask(tasks.SubscriptionDispatchPayload{
		Provider:     providerName,
		EventType:    metadata.EventType,
		EventAction:  metadata.EventAction,
		DeliveryID:   deliveryID,
		OrgID:        wctx.orgID,
		ConnectionID: wctx.inConnection.ID,
		PayloadJSON:  metadata.RawBody,
	})
	if err != nil {
		logger.Error("subscription dispatch: failed to build task", "error", err)
		return
	}
	if _, err := enqueuer.Enqueue(task); err != nil {
		logger.Error("subscription dispatch: failed to enqueue task", "error", err)
		return
	}
}
