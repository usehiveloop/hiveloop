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

// dispatchTrigger enqueues a TypeTriggerDispatch task for an incoming Nango
// "forward" webhook. The dispatcher does the heavy lifting; this function's
// job is to (1) decide whether the provider has triggers configured, (2)
// extract event type/action from the payload, and (3) hand off to asynq.
//
// Called from NangoWebhookHandler.Handle after identify() resolves the
// connection. Returns silently if there's nothing to dispatch — the existing
// org-webhook forward path runs independently and is unaffected.
func dispatchTrigger(
	enqueuer enqueue.TaskEnqueuer,
	wh *nangoWebhook,
	wctx *webhookContext,
) {
	if enqueuer == nil || wctx == nil || wctx.connection == nil || wctx.integration == nil {
		return
	}
	if wh.Type != "forward" || len(wh.Payload) == 0 {
		return
	}

	// Skip providers without triggers in the catalog. The variant fallback
	// (github-app → github) means we have to check both names.
	providerName := wctx.integration.Provider
	cat := catalog.Global()
	if !cat.HasTriggers(providerName) {
		if _, ok := cat.GetProviderTriggersForVariant(providerName); !ok {
			return
		}
	}

	// Decode the payload once to extract event metadata. Nango sometimes wraps
	// the provider body inside {headers, data}; handle both shapes.
	rawBody, headers := unwrapNangoPayload(wh.Payload)

	eventType, eventAction := inferEventFromHeaders(providerName, headers)
	if eventType == "" {
		// Fallback: shape-based inference for providers we know.
		if providerName == "github" || strings.HasPrefix(providerName, "github") {
			eventType, eventAction = inferGitHubEventFromPayload(rawBody)
		}
	}
	if eventType == "" {
		slog.Info("trigger dispatch: could not determine event type, skipping",
			"provider", providerName,
			"connection_id", wctx.connection.ID,
		)
		return
	}

	deliveryID := wh.ConnectionID + ":" + uuid.New().String()

	slog.Info("trigger dispatch: webhook received",
		"delivery_id", deliveryID,
		"provider", providerName,
		"event_type", eventType,
		"event_action", eventAction,
		"org_id", wctx.orgID,
		"connection_id", wctx.connection.ID,
		"payload_bytes", len(rawBody),
		"payload", string(rawBody),
	)

	task, err := tasks.NewRouterDispatchTask(tasks.TriggerDispatchPayload{
		Provider:     providerName,
		EventType:    eventType,
		EventAction:  eventAction,
		DeliveryID:   deliveryID,
		OrgID:        wctx.orgID,
		ConnectionID: wctx.connection.ID,
		PayloadJSON:  rawBody,
	})
	if err != nil {
		slog.Error("trigger dispatch: failed to build task",
			"delivery_id", deliveryID,
			"error", err,
		)
		return
	}
	if _, err := enqueuer.Enqueue(task); err != nil {
		slog.Error("trigger dispatch: failed to enqueue task",
			"delivery_id", deliveryID,
			"error", err,
		)
		return
	}
	slog.Info("trigger dispatch: enqueued",
		"delivery_id", deliveryID,
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
		// Push events have no `action` field — use empty string and let the
		// dispatcher use just "push" as the trigger key.
		return "push", ""
	case probe.RefType != "" && eventAction == "":
		// create/delete events also lack an `action` field; ref_type is the
		// distinguishing marker. We can't tell create from delete here without
		// a header — return ambiguous and let the inference fail.
		return "", ""
	}
	return "", ""
}
