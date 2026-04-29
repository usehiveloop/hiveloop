package tasks

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// The full handler path requires a live Orchestrator + Bridge + sandbox, which
// we don't spin up in unit tests — those are exercised by the integration test
// harness. What we CAN unit-test is the message formatter, since it's the
// format the agent actually sees and any regression there is user-visible.

func decode(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	if len(raw) == 0 {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	return m
}

func TestBuildSubscriptionEventMessage_RendersHeadline(t *testing.T) {
	rawPayload := []byte(`{"action":"opened","issue":{"number":7}}`)
	payload := SubscriptionDispatchPayload{
		Provider:    "github-app",
		EventType:   "issues",
		EventAction: "opened",
		DeliveryID:  "deliv-123",
		OrgID:       uuid.New(),
		PayloadJSON: rawPayload,
	}
	summary := map[string]string{
		"title":  "Fix retry jitter",
		"author": "alice",
	}
	content, fullMessage := buildSubscriptionEventMessage(payload, "github/foo/bar/issue/7", summary, decode(t, rawPayload))

	for _, substr := range []string{
		"# Webhook event: issues.opened",
		"- **provider**: github-app",
		"- **resource**: github/foo/bar/issue/7",
		"- **delivery**: deliv-123",
		"## Summary",
		"- **author**: alice",
		"- **title**: Fix retry jitter",
		"# Notes",
		"you can safely skip this event",
	} {
		if !strings.Contains(content, substr) {
			t.Errorf("content missing %q:\n---\n%s", substr, content)
		}
	}
	// Regression guard: never emit XML or JSON wrappers — the attachment is
	// JSON, so collision would confuse the agent.
	for _, forbidden := range []string{"<webhook_event", "<title>", `"event":`, `"summary":`} {
		if strings.Contains(content, forbidden) {
			t.Errorf("content unexpectedly contains %q:\n---\n%s", forbidden, content)
		}
	}
	if fullMessage != string(rawPayload) {
		t.Errorf("fullMessage should be the raw payload verbatim, got %q", fullMessage)
	}
}

func TestBuildSubscriptionEventMessage_ActionlessEventNoTrailingDot(t *testing.T) {
	rawPayload := []byte(`{"ref":"refs/heads/main"}`)
	payload := SubscriptionDispatchPayload{
		Provider:    "github-app",
		EventType:   "push",
		EventAction: "",
		DeliveryID:  "deliv-456",
		PayloadJSON: rawPayload,
	}
	content, _ := buildSubscriptionEventMessage(payload, "github/foo/bar/branch/main", nil, decode(t, rawPayload))

	if !strings.Contains(content, "# Webhook event: push\n") {
		t.Errorf("actionless event should render bare type in headline, got:\n%s", content)
	}
	if strings.Contains(content, "# Webhook event: push.") {
		t.Errorf("actionless event should not have a trailing dot, got:\n%s", content)
	}
}

func TestBuildSubscriptionEventMessage_EmptyPayloadStillShipsAttachmentPlaceholder(t *testing.T) {
	payload := SubscriptionDispatchPayload{
		Provider:    "github-app",
		EventType:   "issues",
		EventAction: "opened",
		DeliveryID:  "deliv-789",
		PayloadJSON: nil,
	}
	_, fullMessage := buildSubscriptionEventMessage(payload, "github/foo/bar/issue/1", nil, nil)
	if fullMessage != "{}" {
		t.Errorf("empty payload should produce {} fullMessage, got %q", fullMessage)
	}
}

func TestBuildSubscriptionEventMessage_NoSummaryRefsOmitsSection(t *testing.T) {
	// When a trigger lacks summary_refs, the headline and metadata still
	// ship. The ## Summary section is skipped entirely.
	rawPayload := []byte(`{"action":"started"}`)
	payload := SubscriptionDispatchPayload{
		Provider:    "github-app",
		EventType:   "watch",
		EventAction: "started",
		DeliveryID:  "deliv-watch",
		PayloadJSON: rawPayload,
	}
	content, _ := buildSubscriptionEventMessage(payload, "github/foo/bar", nil, decode(t, rawPayload))

	if !strings.Contains(content, "- **resource**: github/foo/bar") {
		t.Errorf("resource should ship even without summary_refs:\n%s", content)
	}
	if strings.Contains(content, "## Summary") {
		t.Errorf("Summary section should be omitted when no summary_refs:\n%s", content)
	}
}

func TestBuildSubscriptionEventMessage_LongValuesRenderAsBoldedFencedBlock(t *testing.T) {
	// A PR body is multi-line / long → render as **body**: followed by a
	// fenced block. NOT a ### subsection, because the body is webhook data
	// and we don't want its own markdown headings (e.g. "## Problem" inside
	// the body) to look like part of our document structure.
	body := "## Problem\nThe retry loop did not jitter.\n\n## Fix\nAdd full-jitter backoff."
	rawPayload := []byte(`{}`)
	payload := SubscriptionDispatchPayload{
		Provider:    "github",
		EventType:   "pull_request",
		EventAction: "opened",
		DeliveryID:  "deliv-block",
		PayloadJSON: rawPayload,
	}
	content, _ := buildSubscriptionEventMessage(payload, "github/o/r/pull/1", map[string]string{
		"title": "Fix retry jitter",
		"body":  body,
	}, decode(t, rawPayload))

	if !strings.Contains(content, "- **title**: Fix retry jitter") {
		t.Errorf("short value should render inline, got:\n%s", content)
	}
	if !strings.Contains(content, "**body**:\n\n```\n"+body+"\n```") {
		t.Errorf("long value should render as bolded name + fenced block, got:\n%s", content)
	}
	// Regression guard: must not promote a field name to a subsection.
	if strings.Contains(content, "### body") {
		t.Errorf("long value must not render as a ### subsection:\n%s", content)
	}
}

func TestBuildSubscriptionEventMessage_GrowsFenceWhenValueContainsTripleBackticks(t *testing.T) {
	// If a webhook body contains ``` (common in release notes) we must grow
	// the enclosing fence so the value can't terminate it early.
	body := "Use this:\n```\nrun --fast\n```\nto speed things up."
	rawPayload := []byte(`{}`)
	payload := SubscriptionDispatchPayload{
		Provider:    "github",
		EventType:   "release",
		EventAction: "published",
		DeliveryID:  "deliv-fence",
		PayloadJSON: rawPayload,
	}
	content, _ := buildSubscriptionEventMessage(payload, "github/o/r", map[string]string{
		"body": body,
	}, decode(t, rawPayload))

	// Enclosing fence must be 4+ backticks.
	if !strings.Contains(content, "**body**:\n\n````\n"+body+"\n````") {
		t.Errorf("fence should grow to 4 backticks around value containing ``` :\n%s", content)
	}
}

func TestBuildSubscriptionEventMessage_TruncatesLongSummaryValuesAt1KB(t *testing.T) {
	longBody := strings.Repeat("x", 5000)
	rawPayload := []byte(`{}`)
	payload := SubscriptionDispatchPayload{
		Provider:    "github",
		EventType:   "pull_request",
		EventAction: "opened",
		DeliveryID:  "deliv-long",
		PayloadJSON: rawPayload,
	}
	content, _ := buildSubscriptionEventMessage(payload, "github/o/r/pull/1", map[string]string{
		"body": longBody,
	}, decode(t, rawPayload))

	if !strings.Contains(content, "…(truncated)") {
		t.Errorf("long value should carry the truncation marker, got:\n%s", content)
	}
	if !strings.Contains(content, "**body**:") {
		t.Fatalf("body field missing from content:\n%s", content)
	}
	// The raw 5000 bytes must not all land in the rendered message.
	if strings.Count(content, "x") > summaryFieldMaxBytes+16 {
		t.Errorf("body value not truncated: found %d x characters", strings.Count(content, "x"))
	}
}

func TestBuildSubscriptionEventMessage_AvailablePathsAsCodeBlock(t *testing.T) {
	rawPayload := []byte(`{
	  "action":"opened",
	  "pull_request":{
	    "title":"T",
	    "body":"body goes here",
	    "user":{"login":"alice"},
	    "labels":[{"name":"bug","color":"red"},{"name":"perf","color":"blue"}],
	    "assignees":[]
	  },
	  "repository":{"full_name":"acme/api"}
	}`)
	payload := SubscriptionDispatchPayload{
		Provider:    "github",
		EventType:   "pull_request",
		EventAction: "opened",
		DeliveryID:  "deliv-paths",
		PayloadJSON: rawPayload,
	}
	content, _ := buildSubscriptionEventMessage(payload, "github/acme/api/pull/1", map[string]string{
		"title": "T",
	}, decode(t, rawPayload))

	for _, substr := range []string{
		"## Available paths in attached full payload",
		"```\naction: string\n",
		"pull_request.title: string\n",
		"pull_request.body: string\n",
		"pull_request.labels: array[2] of object\n",
		"pull_request.labels[*].name: string\n",
		"pull_request.labels[*].color: string\n",
		"pull_request.assignees: array[0]\n",
		"repository.full_name: string\n",
	} {
		if !strings.Contains(content, substr) {
			t.Errorf("paths section missing %q:\n%s", substr, content)
		}
	}
}


