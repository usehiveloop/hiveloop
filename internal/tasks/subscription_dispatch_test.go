package tasks

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

// The full handler path requires a live Orchestrator + Bridge + sandbox, which
// we don't spin up in unit tests — those are exercised by the integration test
// harness. What we CAN unit-test is the message formatter, since it's the
// format the agent actually sees and any regression there is user-visible.

func TestBuildSubscriptionEventMessage_WrapsInWebhookEventTag(t *testing.T) {
	rawPayload := []byte(`{"action":"opened","issue":{"number":7}}`)
	payload := SubscriptionDispatchPayload{
		Provider:    "github-app",
		EventType:   "issues",
		EventAction: "opened",
		DeliveryID:  "deliv-123",
		OrgID:       uuid.New(),
		PayloadJSON: rawPayload,
	}
	message := buildSubscriptionEventMessage(payload, "github/foo/bar/issue/7")

	// XML envelope: opening tag with all four attributes, closing tag.
	for _, substr := range []string{
		`<webhook_event provider="github-app" event="issues.opened" resource_key="github/foo/bar/issue/7" delivery="deliv-123">`,
		`</webhook_event>`,
		// Raw payload body preserved verbatim.
		string(rawPayload),
		// Skip-guidance line — agent gets permission to drop irrelevant events.
		`you can safely skip this event`,
	} {
		if !strings.Contains(message, substr) {
			t.Errorf("message missing %q:\n%s", substr, message)
		}
	}
}

func TestBuildSubscriptionEventMessage_ActionlessEvent(t *testing.T) {
	// "push" has no action — event attribute should be the bare type, no dot.
	payload := SubscriptionDispatchPayload{
		Provider:    "github-app",
		EventType:   "push",
		EventAction: "",
		DeliveryID:  "deliv-456",
		PayloadJSON: []byte(`{"ref":"refs/heads/main"}`),
	}
	message := buildSubscriptionEventMessage(payload, "github/foo/bar/branch/main")

	if !strings.Contains(message, `event="push"`) {
		t.Errorf("actionless event should render bare type as event attribute, got:\n%s", message)
	}
	if strings.Contains(message, `event="push."`) {
		t.Errorf("actionless event should not have trailing dot, got:\n%s", message)
	}
}

func TestBuildSubscriptionEventMessage_EmptyPayloadRendersBraces(t *testing.T) {
	// Edge case: payload byte slice empty/nil — the inner body should still be
	// valid JSON ("{}") so the agent's parser doesn't choke on a blank line.
	payload := SubscriptionDispatchPayload{
		Provider:    "github-app",
		EventType:   "issues",
		EventAction: "opened",
		DeliveryID:  "deliv-789",
		PayloadJSON: nil,
	}
	message := buildSubscriptionEventMessage(payload, "github/foo/bar/issue/1")

	if !strings.Contains(message, "\n{}\n") {
		t.Errorf("empty payload should render as {} inside the tag, got:\n%s", message)
	}
}
