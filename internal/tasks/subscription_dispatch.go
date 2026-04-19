package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"

	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/mcp/catalog"
	"github.com/ziraloop/ziraloop/internal/model"
	"github.com/ziraloop/ziraloop/internal/sandbox"
	"github.com/ziraloop/ziraloop/internal/subscriptions"
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

// summaryFieldMaxBytes caps any single summary field so a huge prose value
// (PR body, release notes, log excerpt) can't bloat the agent's context
// window. Anything over the cap is still fully available via full_message.
const summaryFieldMaxBytes = 1024

// summaryInlineMaxBytes is the cutoff between "render as an inline
// `**name:** value` line" and "render as its own `### name` subsection with
// the value in a code fence". Kept conservative so URLs, branch names, and
// commit SHAs stay on one line but PR/issue bodies always get a subsection.
const summaryInlineMaxBytes = 120

// subscriptionEventGuidance is the per-event prose that gives the agent an
// explicit exit when the event isn't relevant.
const subscriptionEventGuidance = "If this event does not require any action in this conversation (e.g. a CI check that isn't yours, a label change you don't care about, a comment that doesn't mention you), you can safely skip this event and end your turn. Otherwise, act on the event per your workflow."

// buildSubscriptionEventMessage formats the event for delivery to bridge as a
// (content, fullMessage) pair.
//
//   - content is the LLM-visible message, rendered as a markdown document
//     with labelled sections. Markdown is chosen deliberately: the raw
//     webhook payload is JSON, so if we also shipped the wrapper as JSON the
//     agent would have two similar structures to disambiguate. Markdown
//     sections (`# Webhook event`, `## Summary`, `## Available paths …`,
//     `## Instructions`) make it visually obvious which part is our framing
//     and which part is webhook data.
//   - fullMessage is the raw webhook JSON, unchanged. Bridge writes it to a
//     per-conversation attachment file and injects a <system-reminder> with
//     the attachment path, so big payloads don't bloat context on every
//     turn.
func buildSubscriptionEventMessage(
	payload SubscriptionDispatchPayload,
	resourceKey string,
	summaryRefs map[string]string,
	decodedPayload map[string]any,
) (content string, fullMessage string) {
	eventName := payload.EventType
	if payload.EventAction != "" {
		eventName = payload.EventType + "." + payload.EventAction
	}

	var buf strings.Builder

	fmt.Fprintf(&buf, "# Webhook event: %s\n\n", eventName)
	fmt.Fprintf(&buf, "- **provider**: %s\n", payload.Provider)
	fmt.Fprintf(&buf, "- **resource**: %s\n", resourceKey)
	fmt.Fprintf(&buf, "- **delivery**: %s\n", payload.DeliveryID)

	if len(summaryRefs) > 0 {
		inline, block := partitionSummaryFields(summaryRefs)
		buf.WriteString("\n## Summary\n\n")
		for _, name := range inline {
			fmt.Fprintf(&buf, "- **%s**: %s\n", name, summaryRefs[name])
		}
		// Long / multi-line values render as a bolded field name followed by
		// a fenced code block — NOT as a ### subsection. The body is webhook
		// data, not our framing; promoting it to a heading would imply that
		// the value's own markdown (e.g. "## Problem" inside a PR body) is
		// part of our document structure.
		for _, name := range block {
			value := truncateFieldValue(summaryRefs[name], summaryFieldMaxBytes)
			fence := fenceFor(value)
			fmt.Fprintf(&buf, "\n**%s**:\n\n%s\n%s\n%s\n", name, fence, value, fence)
		}
	}

	if paths := renderPayloadPaths(decodedPayload); len(paths) > 0 {
		buf.WriteString("\n## Available paths in attached full payload\n\n")
		buf.WriteString("The full webhook payload is attached to this message. If you need a value not in the summary above, look it up by grepping the attachment for one of these paths:\n\n")
		buf.WriteString("```\n")
		for _, path := range sortedKeys(paths) {
			fmt.Fprintf(&buf, "%s: %s\n", path, paths[path])
		}
		buf.WriteString("```\n")
	}

	buf.WriteString("\n# Notes\n\n")
	buf.WriteString(subscriptionEventGuidance)
	buf.WriteString("\n")

	fullMessage = string(payload.PayloadJSON)
	if fullMessage == "" {
		fullMessage = "{}"
	}
	return buf.String(), fullMessage
}

// partitionSummaryFields splits the summary map into two ordered slices: a
// list of inline field names (short, single-line — render on one line as a
// bullet) and a list of block field names (multi-line or long — render as
// their own ### subsection with a fenced code block). Both slices are
// sorted alphabetically for determinism.
func partitionSummaryFields(summaryRefs map[string]string) (inline, block []string) {
	for name, value := range summaryRefs {
		if isInlineSummaryValue(value) {
			inline = append(inline, name)
		} else {
			block = append(block, name)
		}
	}
	sort.Strings(inline)
	sort.Strings(block)
	return inline, block
}

func isInlineSummaryValue(value string) bool {
	return len(value) <= summaryInlineMaxBytes && !strings.ContainsRune(value, '\n')
}

// fenceFor returns a run of backticks long enough that the given value
// cannot contain it — so rendering `value` inside the fence can't close it
// prematurely. Starts at 3 and grows as needed.
func fenceFor(value string) string {
	fence := "```"
	for strings.Contains(value, fence) {
		fence += "`"
	}
	return fence
}

// sortedKeys returns the map's keys in ascending order for deterministic
// rendering across workers, retries, and test runs.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// payloadPathsMaxDepth caps recursion when walking the payload so a
// pathologically nested provider body can't blow up the outline. Real webhook
// payloads top out around depth 5 (e.g. workflow_run.head_commit.committer.email),
// so 6 leaves headroom without letting exotic payloads run away.
const payloadPathsMaxDepth = 6

// renderPayloadPaths walks the decoded webhook payload and returns a map of
// dot-path → type-string for every field. It's a schematic of what lives in
// the `full_message` attachment, so the agent can decide whether to open it
// and knows exactly which path to grep once it does — without the values
// leaking into the context window.
//
// Shape (keys are sorted by json.Marshal alphabetically in the envelope):
//
//	{
//	  "action": "string",
//	  "pull_request.title": "string",
//	  "pull_request.body": "string (2134 bytes)",
//	  "pull_request.labels": "array[3] of object",
//	  "pull_request.labels[*].name": "string",
//	  "pull_request.labels[*].color": "string"
//	}
//
// For arrays of objects we recurse once into the first element with the
// `[*]` marker so the agent sees the element shape; we don't iterate
// N times. For arrays of scalars we emit only `array[N] of <scalar>`.
// Returns nil when the payload is empty so the envelope omits the field.
func renderPayloadPaths(payload map[string]any) map[string]string {
	if len(payload) == 0 {
		return nil
	}
	paths := map[string]string{}
	walkPayloadPaths(payload, "", 0, paths)
	return paths
}

func walkPayloadPaths(value any, prefix string, depth int, out map[string]string) {
	if depth > payloadPathsMaxDepth {
		out[prefix] = "…(truncated, max depth)"
		return
	}
	switch v := value.(type) {
	case map[string]any:
		if len(v) == 0 {
			out[prefix] = "object{}"
			return
		}
		for key, child := range v {
			next := key
			if prefix != "" {
				next = prefix + "." + key
			}
			walkPayloadPaths(child, next, depth+1, out)
		}
	case []any:
		if len(v) == 0 {
			out[prefix] = "array[0]"
			return
		}
		if obj, ok := v[0].(map[string]any); ok {
			out[prefix] = fmt.Sprintf("array[%d] of object", len(v))
			for key, child := range obj {
				walkPayloadPaths(child, prefix+"[*]."+key, depth+1, out)
			}
			return
		}
		out[prefix] = fmt.Sprintf("array[%d] of %s", len(v), jsonScalarType(v[0]))
	case string:
		if len(v) > 100 {
			out[prefix] = fmt.Sprintf("string (%d bytes)", len(v))
		} else {
			out[prefix] = "string"
		}
	case float64:
		out[prefix] = "number"
	case bool:
		out[prefix] = "bool"
	case nil:
		out[prefix] = "null"
	default:
		out[prefix] = "?"
	}
}

func jsonScalarType(v any) string {
	switch v.(type) {
	case string:
		return "string"
	case float64:
		return "number"
	case bool:
		return "bool"
	case nil:
		return "null"
	default:
		return "?"
	}
}

// truncateFieldValue caps a single summary value at maxBytes. The "…(truncated)"
// suffix is a signal to the agent that the rest lives in the attachment.
func truncateFieldValue(value string, maxBytes int) string {
	if len(value) <= maxBytes {
		return value
	}
	return value[:maxBytes] + "…(truncated)"
}

// topLevelKeys returns the top-level keys of a decoded JSON object, for
// log visibility. Used so we can see the shape of the payload at a glance
// without dumping the whole thing twice.
func topLevelKeys(payload map[string]any) []string {
	keys := make([]string, 0, len(payload))
	for key := range payload {
		keys = append(keys, key)
	}
	return keys
}

// previewString returns the first limit runes of s, with a "…(+N bytes)"
// suffix when truncated. Used to attach a preview of the outgoing message to
// logs without dumping multi-KB JSON bodies twice.
func previewString(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	return s[:limit] + fmt.Sprintf("…(+%d bytes)", len(s)-limit)
}
