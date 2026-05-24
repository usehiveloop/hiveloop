package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/mcp/catalog"
	"github.com/usehivy/hivy/internal/tasks"
)

func dispatchWebhookEvent(
	ctx context.Context,
	enqueuer enqueue.TaskEnqueuer,
	wh *nangoWebhook,
	wctx *webhookContext,
) {
	if enqueuer == nil || wctx == nil || wctx.connection == nil {
		return
	}
	if wh.Type != "forward" || len(wh.Payload) == 0 {
		return
	}

	providerName := wctx.connection.Integration.Provider

	metadata, ok := extractEventMetadata(wh, providerName)
	if !ok {
		return
	}

	deliveryID := wh.ConnectionID + ":" + uuid.New().String()

	enqueueTriggerDispatch(ctx, enqueuer, providerName, metadata, deliveryID, wctx)
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
	ctx context.Context,
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

	task, err := tasks.NewEmployeeTriggerDispatchTask(tasks.EmployeeTriggerDispatchPayload{
		Provider:     providerName,
		EventType:    metadata.EventType,
		EventAction:  metadata.EventAction,
		DeliveryID:   deliveryID,
		OrgID:        wctx.orgID,
		ConnectionID: wctx.connection.ID,
		PayloadJSON:  metadata.RawBody,
	})
	if err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "trigger dispatch: failed to build task",
			"delivery_id", deliveryID, "error", err,
		)
		logging.CaptureWithFields(ctx, fmt.Errorf("trigger dispatch: failed to build task: %w", err), map[string]any{
			"org_id":      wctx.orgID.String(),
			"delivery_id": deliveryID,
			"event_key":   eventKeyForHandler(metadata.EventType, metadata.EventAction),
		})
		return
	}
	if _, err := enqueuer.Enqueue(task); err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "trigger dispatch: failed to enqueue task",
			"delivery_id", deliveryID, "error", err,
		)
		logging.CaptureWithFields(ctx, fmt.Errorf("trigger dispatch: failed to enqueue task: %w", err), map[string]any{
			"org_id":      wctx.orgID.String(),
			"delivery_id": deliveryID,
			"event_key":   eventKeyForHandler(metadata.EventType, metadata.EventAction),
		})
		return
	}
}
