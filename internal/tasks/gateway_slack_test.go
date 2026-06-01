package tasks

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	slacksdk "github.com/slack-go/slack"

	"github.com/usehivy/hivy/internal/gateway"
)

func TestGatewaySlackPayloadStreamURL(t *testing.T) {
	tests := []struct {
		name    string
		payload GatewaySlackPayload
		wantURL string
	}{
		{
			name: "full stream URL used directly",
			payload: GatewaySlackPayload{
				StreamURL:  "https://example.com/gateway/http/streams/stream-123",
				RuntimeURL: "https://example.com",
			},
			wantURL: "https://example.com/gateway/http/streams/stream-123",
		},
		{
			name: "stream URL with different runtime URL",
			payload: GatewaySlackPayload{
				StreamURL:  "https://sandbox.railway.app/gateway/http/streams/abc-456",
				RuntimeURL: "https://other.railway.app",
			},
			wantURL: "https://sandbox.railway.app/gateway/http/streams/abc-456",
		},
		{
			name: "stream URL without runtime URL",
			payload: GatewaySlackPayload{
				StreamURL:  "https://standalone.example.com/gateway/http/streams/xyz-789",
				RuntimeURL: "",
			},
			wantURL: "https://standalone.example.com/gateway/http/streams/xyz-789",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.payload.StreamURL != tt.wantURL {
				t.Errorf("StreamURL = %q, want %q", tt.payload.StreamURL, tt.wantURL)
			}
		})
	}
}

func TestGatewaySlackHandler_UsesStartedStreamTimestamp(t *testing.T) {
	var calls []slackAPICall
	server := newGatewaySlackAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		call := recordSlackAPICall(t, r)
		calls = append(calls, call)
		switch r.URL.Path {
		case "/assistant.threads.setStatus":
			writeSlackOK(t, w, "")
		case "/chat.startStream":
			if call.Form.Get("thread_ts") != "1710000000.123" {
				t.Fatalf("startStream thread_ts = %q", call.Form.Get("thread_ts"))
			}
			if call.Form.Get("recipient_user_id") != "U123" {
				t.Fatalf("startStream recipient_user_id = %q", call.Form.Get("recipient_user_id"))
			}
			writeSlackOK(t, w, "1710000001.456")
		case "/chat.appendStream":
			if call.Form.Get("ts") != "1710000001.456" {
				t.Fatalf("appendStream ts = %q, want stream timestamp", call.Form.Get("ts"))
			}
			writeSlackOK(t, w, "1710000001.456")
		case "/chat.stopStream":
			if call.Form.Get("ts") != "1710000001.456" {
				t.Fatalf("stopStream ts = %q, want stream timestamp", call.Form.Get("ts"))
			}
			writeSlackOK(t, w, "1710000001.456")
		default:
			t.Fatalf("unexpected Slack path: %s", r.URL.Path)
		}
	})
	defer server.Close()

	payload := gatewaySlackTestPayload()
	text, delivered, err := (&GatewaySlackHandler{}).deliverSlackResponse(
		context.Background(),
		payload,
		slacksdk.New("xoxb-test", slacksdk.OptionAPIURL(server.URL+"/")),
		gatewaySlackEvents(
			gateway.SSEEvent{Type: "token", Data: json.RawMessage(`{"text":"Hello "}`)},
			gateway.SSEEvent{Type: "token", Data: json.RawMessage(`{"text":"there"}`)},
			gateway.SSEEvent{Type: "final", Data: json.RawMessage(`{"text":"Hello there"}`)},
		),
		map[string]any{},
	)
	if err != nil {
		t.Fatalf("deliver slack response: %v", err)
	}
	if !delivered || text != "Hello there" {
		t.Fatalf("delivered=%v text=%q", delivered, text)
	}
	if countSlackPath(calls, "/chat.postMessage") != 0 {
		t.Fatalf("unexpected postMessage fallback: %#v", calls)
	}
}

func TestGatewaySlackHandler_PostsThreadReplyWhenStartStreamFails(t *testing.T) {
	var calls []slackAPICall
	server := newGatewaySlackAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		call := recordSlackAPICall(t, r)
		calls = append(calls, call)
		switch r.URL.Path {
		case "/assistant.threads.setStatus":
			writeSlackOK(t, w, "")
		case "/chat.startStream":
			writeSlackError(t, w, "not_allowed_token_type")
		case "/chat.postMessage":
			if call.Form.Get("thread_ts") != "1710000000.123" {
				t.Fatalf("postMessage thread_ts = %q", call.Form.Get("thread_ts"))
			}
			if call.Form.Get("text") != "Fallback answer" {
				t.Fatalf("postMessage text = %q", call.Form.Get("text"))
			}
			writeSlackOK(t, w, "1710000002.789")
		default:
			t.Fatalf("unexpected Slack path: %s", r.URL.Path)
		}
	})
	defer server.Close()

	text, delivered, err := (&GatewaySlackHandler{}).deliverSlackResponse(
		context.Background(),
		gatewaySlackTestPayload(),
		slacksdk.New("xoxb-test", slacksdk.OptionAPIURL(server.URL+"/")),
		gatewaySlackEvents(
			gateway.SSEEvent{Type: "token", Data: json.RawMessage(`{"text":"partial"}`)},
			gateway.SSEEvent{Type: "final", Data: json.RawMessage(`{"text":"Fallback answer"}`)},
		),
		map[string]any{},
	)
	if err != nil {
		t.Fatalf("deliver slack response: %v", err)
	}
	if !delivered || text != "Fallback answer" {
		t.Fatalf("delivered=%v text=%q", delivered, text)
	}
	if countSlackPath(calls, "/chat.appendStream") != 0 || countSlackPath(calls, "/chat.stopStream") != 0 {
		t.Fatalf("stream methods should not be used after start failure: %#v", calls)
	}
	if countSlackPath(calls, "/chat.postMessage") != 1 {
		t.Fatalf("postMessage fallback count = %d", countSlackPath(calls, "/chat.postMessage"))
	}
}

func TestGatewaySlackHandler_PostsThreadReplyWhenStopStreamFails(t *testing.T) {
	var calls []slackAPICall
	server := newGatewaySlackAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		call := recordSlackAPICall(t, r)
		calls = append(calls, call)
		switch r.URL.Path {
		case "/assistant.threads.setStatus":
			writeSlackOK(t, w, "")
		case "/chat.startStream":
			writeSlackOK(t, w, "1710000001.456")
		case "/chat.appendStream":
			writeSlackOK(t, w, "1710000001.456")
		case "/chat.stopStream":
			if call.Form.Get("ts") != "1710000001.456" {
				t.Fatalf("stopStream ts = %q, want stream timestamp", call.Form.Get("ts"))
			}
			writeSlackError(t, w, "message_not_owned_by_app")
		case "/chat.postMessage":
			if call.Form.Get("thread_ts") != "1710000000.123" {
				t.Fatalf("postMessage thread_ts = %q", call.Form.Get("thread_ts"))
			}
			writeSlackOK(t, w, "1710000002.789")
		default:
			t.Fatalf("unexpected Slack path: %s", r.URL.Path)
		}
	})
	defer server.Close()

	_, delivered, err := (&GatewaySlackHandler{}).deliverSlackResponse(
		context.Background(),
		gatewaySlackTestPayload(),
		slacksdk.New("xoxb-test", slacksdk.OptionAPIURL(server.URL+"/")),
		gatewaySlackEvents(
			gateway.SSEEvent{Type: "token", Data: json.RawMessage(`{"text":"Hello"}`)},
			gateway.SSEEvent{Type: "final", Data: json.RawMessage(`{"text":"Hello"}`)},
		),
		map[string]any{},
	)
	if err != nil {
		t.Fatalf("deliver slack response: %v", err)
	}
	if !delivered {
		t.Fatal("expected fallback delivery")
	}
	if countSlackPath(calls, "/chat.postMessage") != 1 {
		t.Fatalf("postMessage fallback count = %d", countSlackPath(calls, "/chat.postMessage"))
	}
}

type slackAPICall struct {
	Path string
	Form url.Values
}

func newGatewaySlackAPIServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(handler))
}

func recordSlackAPICall(t *testing.T, r *http.Request) slackAPICall {
	t.Helper()
	if err := r.ParseForm(); err != nil {
		t.Fatalf("parse Slack form: %v", err)
	}
	return slackAPICall{Path: r.URL.Path, Form: cloneURLValues(r.Form)}
}

func cloneURLValues(values url.Values) url.Values {
	out := make(url.Values, len(values))
	for key, val := range values {
		out[key] = append([]string(nil), val...)
	}
	return out
}

func writeSlackOK(t *testing.T, w http.ResponseWriter, ts string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	body := map[string]any{"ok": true, "channel": "C123"}
	if ts != "" {
		body["ts"] = ts
	}
	if err := json.NewEncoder(w).Encode(body); err != nil {
		t.Fatalf("write Slack response: %v", err)
	}
}

func writeSlackError(t *testing.T, w http.ResponseWriter, code string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": code}); err != nil {
		t.Fatalf("write Slack error: %v", err)
	}
}

func gatewaySlackEvents(events ...gateway.SSEEvent) <-chan gateway.SSEEvent {
	ch := make(chan gateway.SSEEvent, len(events))
	for _, event := range events {
		ch <- event
	}
	close(ch)
	return ch
}

func gatewaySlackTestPayload() GatewaySlackPayload {
	return GatewaySlackPayload{
		ConnectionID: "conn-1",
		OrgID:        "org-1",
		EmployeeID:   "employee-1",
		ChannelID:    "C123",
		ThreadTS:     "1710000000.123",
		SessionID:    "session-1",
		TraceID:      "trace-1",
		TurnID:       "turn-1",
		SenderID:     "U123",
	}
}

func countSlackPath(calls []slackAPICall, path string) int {
	count := 0
	for _, call := range calls {
		if call.Path == path {
			count++
		}
	}
	return count
}
