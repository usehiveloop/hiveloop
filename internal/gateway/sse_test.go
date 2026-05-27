package gateway

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func TestSSESubscriberParseEvents(t *testing.T) {
	t.Run("parse token event", func(t *testing.T) {
		data := json.RawMessage(`{"text":"hello world"}`)
		event := SSEEvent{Type: "token", Data: data}

		if event.Type != "token" {
			t.Errorf("event.Type = %q, want %q", event.Type, "token")
		}

		var parsed struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(event.Data, &parsed); err != nil {
			t.Fatalf("failed to parse data: %v", err)
		}
		if parsed.Text != "hello world" {
			t.Errorf("text = %q, want %q", parsed.Text, "hello world")
		}
	})

	t.Run("parse final event", func(t *testing.T) {
		data := json.RawMessage(`{"text":"complete response"}`)
		event := SSEEvent{Type: "final", Data: data}

		if event.Type != "final" {
			t.Errorf("event.Type = %q, want %q", event.Type, "final")
		}
	})

	t.Run("parse done event", func(t *testing.T) {
		data := json.RawMessage(`{"session_id":"http-123"}`)
		event := SSEEvent{Type: "done", Data: data}

		if event.Type != "done" {
			t.Errorf("event.Type = %q, want %q", event.Type, "done")
		}
	})
}

func TestReceiveConnectionResultFields(t *testing.T) {
	now := time.Now()
	result := ReceiveConnectionResult{
		Inbound: InboundEnvelope{
			Provider:  SlackProvider,
			ChannelID: "C456",
			ThreadID:  "1234567890.000000",
			SenderID:  "U789",
			Text:      "hello",
			ReceivedAt: now,
		},
		Session: model.EmployeeSession{
			ID: uuid.New(),
		},
		RuntimeConversationID: "http-gateway-abc123",
		RuntimeSessionID:      "http-gateway-abc123",
		StreamURL:             "http://localhost:25434/gateway/http/streams/stream-123",
		RuntimeURL:            "http://localhost:25434",
		TraceID:               "trace-123",
		TurnID:                "turn-123",
		ActionToken:           "test-token",
	}

	if result.Inbound.ChannelID != "C456" {
		t.Errorf("ChannelID = %q, want %q", result.Inbound.ChannelID, "C456")
	}
	if result.StreamURL == "" {
		t.Error("expected non-empty StreamURL")
	}
	if result.ActionToken != "test-token" {
		t.Errorf("ActionToken = %q, want %q", result.ActionToken, "test-token")
	}
}
