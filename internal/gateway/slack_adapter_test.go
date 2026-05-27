package gateway

import (
	"context"
	"encoding/json"
	"testing"
)

func TestSlackAdapterDecodeInbound(t *testing.T) {
	adapter := NewSlackAdapter()

	t.Run("app_mention event", func(t *testing.T) {
		payload := json.RawMessage(`{
			"type": "event_callback",
			"team_id": "T123",
			"event_id": "Ev123",
			"event": {
				"type": "app_mention",
				"channel": "C456",
				"user": "U789",
				"text": "hello bot",
				"ts": "1234567890.123456",
				"thread_ts": "1234567890.000000"
			}
		}`)
		envelope := WebhookEnvelope{Body: payload}

		inbound, ok, err := adapter.DecodeInbound(context.Background(), envelope)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Fatal("expected event to be decoded")
		}
		if inbound.Provider != SlackProvider {
			t.Errorf("Provider = %q, want %q", inbound.Provider, SlackProvider)
		}
		if inbound.ChannelID != "C456" {
			t.Errorf("ChannelID = %q, want %q", inbound.ChannelID, "C456")
		}
		if inbound.ThreadID != "1234567890.000000" {
			t.Errorf("ThreadID = %q, want %q", inbound.ThreadID, "1234567890.000000")
		}
		if inbound.ThreadKey != "C456:1234567890.000000" {
			t.Errorf("ThreadKey = %q, want %q", inbound.ThreadKey, "C456:1234567890.000000")
		}
		if inbound.SenderID != "U789" {
			t.Errorf("SenderID = %q, want %q", inbound.SenderID, "U789")
		}
		if inbound.Text != "hello bot" {
			t.Errorf("Text = %q, want %q", inbound.Text, "hello bot")
		}
		if inbound.DedupeKey != "Ev123" {
			t.Errorf("DedupeKey = %q, want %q", inbound.DedupeKey, "Ev123")
		}
	})

	t.Run("message without thread", func(t *testing.T) {
		payload := json.RawMessage(`{
			"type": "event_callback",
			"team_id": "T123",
			"event_id": "Ev456",
			"event": {
				"type": "message",
				"channel": "C456",
				"user": "U789",
				"text": "direct message",
				"ts": "1234567890.123456"
			}
		}`)
		envelope := WebhookEnvelope{Body: payload}

		inbound, ok, err := adapter.DecodeInbound(context.Background(), envelope)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Fatal("expected event to be decoded")
		}
		if inbound.ThreadKey != "C456:1234567890.123456" {
			t.Errorf("ThreadKey = %q, want %q (should use ts when no thread_ts)", inbound.ThreadKey, "C456:1234567890.123456")
		}
	})

	t.Run("bot message ignored", func(t *testing.T) {
		payload := json.RawMessage(`{
			"type": "event_callback",
			"event_id": "Ev789",
			"event": {
				"type": "message",
				"channel": "C456",
				"bot_id": "B123",
				"text": "bot says hello",
				"ts": "1234567890.123456"
			}
		}`)
		envelope := WebhookEnvelope{Body: payload}

		_, ok, err := adapter.DecodeInbound(context.Background(), envelope)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok {
			t.Fatal("bot message should be ignored")
		}
	})

	t.Run("url_verification ignored", func(t *testing.T) {
		payload := json.RawMessage(`{
			"type": "url_verification",
			"challenge": "test-challenge"
		}`)
		envelope := WebhookEnvelope{Body: payload}

		_, ok, err := adapter.DecodeInbound(context.Background(), envelope)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok {
			t.Fatal("url_verification should be ignored")
		}
	})

	t.Run("non-message event ignored", func(t *testing.T) {
		payload := json.RawMessage(`{
			"type": "event_callback",
			"event_id": "Ev000",
			"event": {
				"type": "reaction_added",
				"channel": "C456",
				"user": "U789"
			}
		}`)
		envelope := WebhookEnvelope{Body: payload}

		_, ok, err := adapter.DecodeInbound(context.Background(), envelope)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok {
			t.Fatal("non-message event should be ignored")
		}
	})

	t.Run("action_token extracted", func(t *testing.T) {
		payload := json.RawMessage(`{
			"type": "event_callback",
			"team_id": "T123",
			"event_id": "Ev999",
			"event": {
				"type": "app_mention",
				"channel": "C456",
				"user": "U789",
				"text": "hey",
				"ts": "1234567890.123456",
				"assistant_thread": {
					"action_token": "test-token-123"
				}
			}
		}`)
		envelope := WebhookEnvelope{Body: payload}

		inbound, ok, err := adapter.DecodeInbound(context.Background(), envelope)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Fatal("expected event to be decoded")
		}
		actionToken, _ := inbound.Raw["action_token"].(string)
		if actionToken != "test-token-123" {
			t.Errorf("action_token = %q, want %q", actionToken, "test-token-123")
		}
	})
}

func TestSlackAdapterFormatAgentRequest(t *testing.T) {
	adapter := NewSlackAdapter()

	inbound := InboundEnvelope{
		SenderID: "U789",
		ChannelID: "C456",
		Text:      "hello bot",
	}

	req, err := adapter.FormatAgentRequest(context.Background(), inbound)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Markdown == "" {
		t.Error("expected non-empty markdown")
	}
	if req.Metadata["sender_id"] != "U789" {
		t.Errorf("metadata sender_id = %q, want %q", req.Metadata["sender_id"], "U789")
	}
}

func TestSlackAdapterActionToken(t *testing.T) {
	adapter := NewSlackAdapter()

	t.Run("with action_token", func(t *testing.T) {
		raw := map[string]any{"action_token": "test-token"}
		if got := adapter.ActionToken(raw); got != "test-token" {
			t.Errorf("ActionToken() = %q, want %q", got, "test-token")
		}
	})

	t.Run("without action_token", func(t *testing.T) {
		raw := map[string]any{"other": "value"}
		if got := adapter.ActionToken(raw); got != "" {
			t.Errorf("ActionToken() = %q, want empty", got)
		}
	})

	t.Run("nil map", func(t *testing.T) {
		if got := adapter.ActionToken(nil); got != "" {
			t.Errorf("ActionToken() = %q, want empty", got)
		}
	})
}

func TestSlackAdapterRenderResponse(t *testing.T) {
	adapter := NewSlackAdapter()

	t.Run("valid response", func(t *testing.T) {
		response := AgentResponse{
			Text: "Hello world",
			ChannelID: "C456",
			ThreadID: "1234567890.000000",
		}

		payload, err := adapter.RenderResponse(context.Background(), response)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if payload.Text != "Hello world" {
			t.Errorf("Text = %q, want %q", payload.Text, "Hello world")
		}
		if payload.ChannelID != "C456" {
			t.Errorf("ChannelID = %q, want %q", payload.ChannelID, "C456")
		}
	})

	t.Run("empty text error", func(t *testing.T) {
		response := AgentResponse{Text: ""}
		_, err := adapter.RenderResponse(context.Background(), response)
		if err == nil {
			t.Error("expected error for empty text")
		}
	})
}
