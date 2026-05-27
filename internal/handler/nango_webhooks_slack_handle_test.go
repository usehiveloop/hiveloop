package handler

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestHandleSlackWebhookEvent(t *testing.T) {
	ctx := context.Background()
	wctx := slackTestWebhookContext()

	t.Run("url_verification challenge", func(t *testing.T) {
		payload := slackTestPayload(`{"type":"url_verification","challenge":"test-challenge-123"}`)
		wh := &nangoWebhook{Type: "forward", Payload: payload}

		w := httptest.NewRecorder()
		handled := handleSlackWebhookEvent(ctx, w, wh, wctx)

		if !handled {
			t.Error("expected event to be handled")
		}

		var resp map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to unmarshal response: %v", err)
		}
		if resp["challenge"] != "test-challenge-123" {
			t.Errorf("challenge = %q, want %q", resp["challenge"], "test-challenge-123")
		}
	})

	t.Run("app_mention event", func(t *testing.T) {
		payload := slackTestPayload(`{
			"type": "event_callback",
			"event": {
				"type": "app_mention",
				"team_id": "T123",
				"channel": "C456",
				"user": "U789",
				"user_name": "testuser",
				"text": "hello bot",
				"ts": "1234567890.123456",
				"thread_ts": "1234567890.000000"
			}
		}`)
		wh := &nangoWebhook{Type: "forward", Payload: payload}

		w := httptest.NewRecorder()
		handled := handleSlackWebhookEvent(ctx, w, wh, wctx)

		if !handled {
			t.Error("expected event to be handled")
		}
	})

	t.Run("message event without thread", func(t *testing.T) {
		payload := slackTestPayload(`{
			"type": "event_callback",
			"event": {
				"type": "message",
				"channel": "C456",
				"user": "U789",
				"text": "direct message",
				"ts": "1234567890.123456"
			}
		}`)
		wh := &nangoWebhook{Type: "forward", Payload: payload}

		w := httptest.NewRecorder()
		handled := handleSlackWebhookEvent(ctx, w, wh, wctx)

		if !handled {
			t.Error("expected event to be handled")
		}
	})

	t.Run("bot message ignored", func(t *testing.T) {
		payload := slackTestPayload(`{
			"type": "event_callback",
			"event": {
				"type": "message",
				"channel": "C456",
				"bot_id": "B123",
				"text": "bot says hello",
				"ts": "1234567890.123456"
			}
		}`)
		wh := &nangoWebhook{Type: "forward", Payload: payload}

		w := httptest.NewRecorder()
		handled := handleSlackWebhookEvent(ctx, w, wh, wctx)

		if !handled {
			t.Error("expected bot message to be handled (ignored)")
		}
	})

	t.Run("bot_message subtype ignored", func(t *testing.T) {
		payload := slackTestPayload(`{
			"type": "event_callback",
			"event": {
				"type": "message",
				"channel": "C456",
				"subtype": "bot_message",
				"text": "bot says hello",
				"ts": "1234567890.123456"
			}
		}`)
		wh := &nangoWebhook{Type: "forward", Payload: payload}

		w := httptest.NewRecorder()
		handled := handleSlackWebhookEvent(ctx, w, wh, wctx)

		if !handled {
			t.Error("expected bot_message subtype to be handled (ignored)")
		}
	})

	t.Run("non-message event type ignored", func(t *testing.T) {
		payload := slackTestPayload(`{
			"type": "event_callback",
			"event": {
				"type": "reaction_added",
				"channel": "C456",
				"user": "U789"
			}
		}`)
		wh := &nangoWebhook{Type: "forward", Payload: payload}

		w := httptest.NewRecorder()
		handled := handleSlackWebhookEvent(ctx, w, wh, wctx)

		if !handled {
			t.Error("expected non-message event to be handled (ignored)")
		}
	})

	t.Run("invalid json payload", func(t *testing.T) {
		payload := json.RawMessage(`{"data":"not valid json","headers":{}}`)
		wh := &nangoWebhook{Type: "forward", Payload: payload}

		w := httptest.NewRecorder()
		handled := handleSlackWebhookEvent(ctx, w, wh, wctx)

		if !handled {
			t.Error("expected invalid payload to be handled (with error)")
		}
	})

	t.Run("empty payload", func(t *testing.T) {
		payload := json.RawMessage(`{}`)
		wh := &nangoWebhook{Type: "forward", Payload: payload}

		w := httptest.NewRecorder()
		handled := handleSlackWebhookEvent(ctx, w, wh, wctx)

		if !handled {
			t.Error("expected empty payload to be handled (ignored)")
		}
	})
}
