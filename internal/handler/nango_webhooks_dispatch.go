package handler

import (
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/google/uuid"

	"github.com/ziraloop/ziraloop/internal/enqueue"
	"github.com/ziraloop/ziraloop/internal/mcp/catalog"
	"github.com/ziraloop/ziraloop/internal/tasks"
)

// dispatchWebhookEvent fans an inbound Nango "forward" webhook into two
// parallel async pipelines:
//
//  1. Trigger dispatch — the existing router path. Spawns new conversations
//     for agents whose AgentTrigger / RouterTrigger config matches this event.
//  2. Subscription dispatch — the new subscription-driven path. Forwards the
//     event into every active conversation_subscription whose resource_key
//     the event resolves to.
//
// The two paths operate on the same input (provider, event_type, event_action,
// payload) but are independent: a failure in one doesn't affect the other.
// An event can legitimately trigger both — e.g. `issues.opened` spawns a new
// Kira conversation AND matches a pre-existing Kobi subscription.
//
// Called from NangoWebhookHandler.Handle once the inbound connection is
// identified. Returns silently if there's nothing to dispatch — the existing
// org-webhook forward runs independently and is unaffected.
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

// eventMetadata bundles the per-event fields derived from a Nango webhook
// that both dispatch pipelines consume.
type eventMetadata struct {
	EventType   string
	EventAction string
	RawBody     []byte
	Headers     map[string]string
}

// extractEventMetadata unwraps the Nango envelope, resolves the provider
// event_type/action (from headers when available, falling back to shape
// inference), and returns what both dispatch pipelines need. The second
// return value is false when the event can't be identified — callers drop
// silently in that case; the original webhook body is still stored by the
// org-forward pipeline for audit.
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

	// For GitHub events where the header says the type but the body carries
	// the action, pull the action from the body if we don't already have one.
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

// enqueueTriggerDispatch sends the event into the existing router/trigger
// dispatcher — unchanged behavior from before this refactor, just moved
// behind the shared metadata helper.
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

// enqueueSubscriptionDispatch sends the event into the subscription-driven
// forwarder. We only enqueue when the provider has triggers in the catalog —
// without a trigger def, the handler can't resolve a resource_key, so there's
// no point waking a worker to drop it.
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

// unwrapNangoPayload handles the two shapes Nango may forward:
//
//   1. The raw provider body: {"action": "opened", "issue": {...}, ...}
//   2. The wrapped envelope:  {"headers": {...}, "data": {...}}
//
// Returns (rawProviderBody, headers). If the payload is unwrapped, headers is
// nil. The dispatcher always wants the raw provider body — that's what trigger
// refs are dot-pathed against.
func unwrapNangoPayload(payload json.RawMessage) ([]byte, map[string]string) {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(payload, &probe); err != nil {
		return payload, nil
	}
	dataField, hasData := probe["data"]
	headersField, hasHeaders := probe["headers"]
	if !hasData || !hasHeaders {
		return payload, nil
	}
	// Wrapped shape — extract headers + data.
	headers := make(map[string]string)
	var headerProbe map[string]any
	if err := json.Unmarshal(headersField, &headerProbe); err == nil {
		for key, value := range headerProbe {
			if str, ok := value.(string); ok {
				headers[strings.ToLower(key)] = str
			}
		}
	}
	return dataField, headers
}

// inferEventFromHeaders pulls the event type out of provider-specific headers
// (the canonical source). Returns empty when the header isn't present so the
// caller can fall back to shape-based inference.
func inferEventFromHeaders(provider string, headers map[string]string) (eventType, eventAction string) {
	if len(headers) == 0 {
		return "", ""
	}
	switch {
	case provider == "github" || strings.HasPrefix(provider, "github"):
		// X-GitHub-Event: "issues", "pull_request", "push", etc.
		// The action sub-key lives in the body's "action" field — caller
		// extracts it from rawBody if needed.
		eventType = headers["x-github-event"]
	}
	return eventType, ""
}

// inferGitHubEventFromPayload guesses the event type by looking at which
// top-level objects are present in the body. This is a fallback for when
// the X-GitHub-Event header isn't passed through; the header is the canonical
// source. The mapping mirrors the events we have triggers for in the catalog
// (see internal/mcp/catalog/providers/github.triggers.json).
//
// For events with sub-actions (issues.opened, pull_request.synchronize), the
// sub-action comes from the body's top-level "action" field.
func inferGitHubEventFromPayload(body []byte) (eventType, eventAction string) {
	var probe struct {
		Action       string          `json:"action"`
		Issue        json.RawMessage `json:"issue"`
		Comment      json.RawMessage `json:"comment"`
		PullRequest  json.RawMessage `json:"pull_request"`
		Review       json.RawMessage `json:"review"`
		WorkflowRun  json.RawMessage `json:"workflow_run"`
		WorkflowJob  json.RawMessage `json:"workflow_job"`
		CheckRun     json.RawMessage `json:"check_run"`
		CheckSuite   json.RawMessage `json:"check_suite"`
		Release      json.RawMessage `json:"release"`
		Discussion   json.RawMessage `json:"discussion"`
		Deployment   json.RawMessage `json:"deployment"`
		Ref          string          `json:"ref"`
		RefType      string          `json:"ref_type"`
		Before       string          `json:"before"`
		After        string          `json:"after"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		return "", ""
	}
	eventAction = probe.Action

	// Order matters — more specific shapes first.
	switch {
	case len(probe.WorkflowJob) > 0:
		return "workflow_job", eventAction
	case len(probe.WorkflowRun) > 0:
		return "workflow_run", eventAction
	case len(probe.CheckRun) > 0:
		return "check_run", eventAction
	case len(probe.CheckSuite) > 0:
		return "check_suite", eventAction
	case len(probe.Review) > 0 && len(probe.PullRequest) > 0:
		return "pull_request_review", eventAction
	case len(probe.PullRequest) > 0 && len(probe.Comment) > 0:
		return "pull_request_review_comment", eventAction
	case len(probe.Comment) > 0 && len(probe.Issue) > 0:
		return "issue_comment", eventAction
	case len(probe.Issue) > 0:
		return "issues", eventAction
	case len(probe.PullRequest) > 0:
		return "pull_request", eventAction
	case len(probe.Release) > 0:
		return "release", eventAction
	case len(probe.Discussion) > 0 && len(probe.Comment) > 0:
		return "discussion_comment", eventAction
	case len(probe.Discussion) > 0:
		return "discussion", eventAction
	case len(probe.Deployment) > 0:
		return "deployment_status", eventAction
	case probe.Before != "" && probe.After != "":
		return "push", ""
	case probe.RefType != "" && eventAction == "":
		return "", ""
	}
	return "", ""
}
