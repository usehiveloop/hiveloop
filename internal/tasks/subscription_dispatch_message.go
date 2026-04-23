package tasks

import (
	"fmt"
	"strings"
)

const summaryFieldMaxBytes = 1024

const summaryInlineMaxBytes = 120

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
