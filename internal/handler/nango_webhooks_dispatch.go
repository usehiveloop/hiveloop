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
		slog.Info("webhook dispatch: skipping, missing enqueuer or connection context",
			"enqueuer_nil", enqueuer == nil,
			"wctx_nil", wctx == nil,
			"in_connection_nil", wctx == nil || wctx.inConnection == nil,
		)
		return
	}
	if wh.Type != "forward" || len(wh.Payload) == 0 {
		slog.Info("webhook dispatch: skipping non-forward or empty event",
			"type", wh.Type,
			"payload_bytes", len(wh.Payload),
		)
		return
	}

	providerName := wctx.inConnection.InIntegration.Provider

	slog.Info("webhook dispatch: beginning",
		"provider", providerName,
		"org_id", wctx.orgID,
		"in_connection_id", wctx.inConnection.ID,
		"nango_connection_id", wh.ConnectionID,
		"nango_operation", wh.Operation,
		"raw_envelope_bytes", len(wh.Payload),
		"raw_envelope", string(wh.Payload),
	)

	metadata, ok := extractEventMetadata(wh, providerName)
	if !ok {
		slog.Info("webhook dispatch: event metadata extraction returned false, dropping")
		return
	}

	deliveryID := wh.ConnectionID + ":" + uuid.New().String()

	slog.Info("webhook dispatch: metadata extracted",
		"delivery_id", deliveryID,
		"provider", providerName,
		"event_type", metadata.EventType,
		"event_action", metadata.EventAction,
		"org_id", wctx.orgID,
		"in_connection_id", wctx.inConnection.ID,
		"payload_bytes", len(metadata.RawBody),
		"raw_payload", string(metadata.RawBody),
		"headers", metadata.Headers,
	)

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
	slog.Info("webhook dispatch: unwrapped nango envelope",
		"provider", providerName,
		"raw_body_bytes", len(rawBody),
		"header_count", len(headers),
		"headers", headers,
	)

	eventType, eventAction := inferEventFromHeaders(providerName, headers)
	if eventType != "" {
		slog.Info("webhook dispatch: event type inferred from headers",
			"provider", providerName,
			"event_type", eventType,
		)
	} else {
		slog.Info("webhook dispatch: no header-based event type, falling back to payload shape",
			"provider", providerName,
		)
		if providerName == "github" || strings.HasPrefix(providerName, "github") {
			eventType, eventAction = inferGitHubEventFromPayload(rawBody)
			slog.Info("webhook dispatch: github shape inference result",
				"event_type", eventType,
				"event_action", eventAction,
			)
		}
	}
	if eventType == "" {
		slog.Info("webhook dispatch: could not determine event type, skipping",
			"provider", providerName,
		)
		return eventMetadata{}, false
	}

	if eventAction == "" && (providerName == "github" || strings.HasPrefix(providerName, "github")) {
		var probe struct {
			Action string `json:"action"`
		}
		_ = json.Unmarshal(rawBody, &probe)
		eventAction = probe.Action
		slog.Info("webhook dispatch: pulled action from body",
			"event_action", eventAction,
		)
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
	slog.Info("trigger dispatch: enqueued", "delivery_id", deliveryID)
}

func enqueueSubscriptionDispatch(
	enqueuer enqueue.TaskEnqueuer,
	providerName string,
	metadata eventMetadata,
	deliveryID string,
	wctx *webhookContext,
) {
	logger := slog.With(
		"component", "subscription_dispatch_enqueue",
		"delivery_id", deliveryID,
		"provider", providerName,
		"event_type", metadata.EventType,
		"event_action", metadata.EventAction,
		"org_id", wctx.orgID,
	)

	cat := catalog.Global()
	hasTriggers := cat.HasTriggers(providerName)
	_, hasVariant := cat.GetProviderTriggersForVariant(providerName)
	logger.Info("subscription dispatch: catalog check",
		"has_direct_triggers", hasTriggers,
		"has_variant_triggers", hasVariant,
	)
	if !hasTriggers && !hasVariant {
		logger.Info("subscription dispatch: provider has no triggers in catalog, dropping")
		return
	}

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
	logger.Info("subscription dispatch: enqueued",
		"payload_bytes", len(metadata.RawBody),
	)
}
