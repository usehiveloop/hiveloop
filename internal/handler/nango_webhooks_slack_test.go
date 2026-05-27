package handler

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func TestIsSlackProvider(t *testing.T) {
	tests := []struct {
		name string
		conn *model.Connection
		want bool
	}{
		{
			name: "slack connection",
			conn: &model.Connection{
				Integration: model.Integration{Provider: "slack"},
			},
			want: true,
		},
		{
			name: "github connection",
			conn: &model.Connection{
				Integration: model.Integration{Provider: "github"},
			},
			want: false,
		},
		{
			name: "nil connection",
			conn: nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSlackProvider(tt.conn)
			if got != tt.want {
				t.Errorf("isSlackProvider() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUnwrapSlackEvent(t *testing.T) {
	t.Run("valid nango envelope", func(t *testing.T) {
		payload := json.RawMessage(`{
			"data": {"type":"event_callback","event":{"type":"app_mention","channel":"C123"}},
			"headers": {"x-slack-signature": "abc123"}
		}`)

		body, headers := unwrapSlackEvent(payload)

		if len(body) == 0 {
			t.Fatal("expected non-empty body")
		}

		var event slackEventCallback
		if err := json.Unmarshal(body, &event); err != nil {
			t.Fatalf("failed to unmarshal body: %v", err)
		}
		if event.Type != "event_callback" {
			t.Errorf("event.Type = %q, want %q", event.Type, "event_callback")
		}
		if event.Event.Channel != "C123" {
			t.Errorf("event.Event.Channel = %q, want %q", event.Event.Channel, "C123")
		}

		if headers["x-slack-signature"] != "abc123" {
			t.Errorf("headers[x-slack-signature] = %q, want %q", headers["x-slack-signature"], "abc123")
		}
	})

	t.Run("raw payload without envelope", func(t *testing.T) {
		payload := json.RawMessage(`{"type":"event_callback","event":{"type":"message"}}`)

		body, headers := unwrapSlackEvent(payload)

		if len(body) == 0 {
			t.Fatal("expected non-empty body")
		}

		var event slackEventCallback
		if err := json.Unmarshal(body, &event); err != nil {
			t.Fatalf("failed to unmarshal body: %v", err)
		}
		if event.Type != "event_callback" {
			t.Errorf("event.Type = %q, want %q", event.Type, "event_callback")
		}

		if len(headers) != 0 {
			t.Errorf("expected empty headers, got %v", headers)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		payload := json.RawMessage(`not valid json`)

		body, headers := unwrapSlackEvent(payload)

		if string(body) != "not valid json" {
			t.Errorf("body = %q, want original payload", string(body))
		}
		if headers != nil {
			t.Errorf("headers = %v, want nil", headers)
		}
	})
}

func slackTestWebhookContext() *webhookContext {
	return &webhookContext{
		orgID: uuid.New(),
		connection: &model.Connection{
			ID:          uuid.New(),
			Integration: model.Integration{Provider: "slack"},
		},
	}
}

func slackTestPayload(eventJSON string) json.RawMessage {
	return json.RawMessage(`{"data":` + eventJSON + `,"headers":{}}`)
}
