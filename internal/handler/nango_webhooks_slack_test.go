package handler

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/usehivy/hivy/internal/gateway"
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

func TestNormalizedHeaders(t *testing.T) {
	tests := []struct {
		name     string
		input    http.Header
		expected map[string]string
	}{
		{
			name:     "empty headers",
			input:    http.Header{},
			expected: map[string]string{},
		},
		{
			name:     "single header",
			input:    http.Header{"Content-Type": {"application/json"}},
			expected: map[string]string{"content-type": "application/json"},
		},
		{
			name:     "multiple headers",
			input:    http.Header{"Content-Type": {"application/json"}, "X-Custom": {"value"}},
			expected: map[string]string{"content-type": "application/json", "x-custom": "value"},
		},
		{
			name:     "header case normalization",
			input:    http.Header{"CONTENT-TYPE": {"application/json"}},
			expected: map[string]string{"content-type": "application/json"},
		},
		{
			name:     "skip empty values",
			input:    http.Header{"Content-Type": {}},
			expected: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizedHeaders(tt.input)
			if len(got) != len(tt.expected) {
				t.Errorf("normalizedHeaders() returned %d headers, want %d", len(got), len(tt.expected))
				return
			}
			for k, v := range tt.expected {
				if got[k] != v {
					t.Errorf("normalizedHeaders()[%q] = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestGatewaySlackPayloadIncludesRuntimeAPIKey(t *testing.T) {
	connectionID := uuid.New()
	orgID := uuid.New()
	employeeID := uuid.New()
	sessionID := uuid.New()

	envelope := gateway.WebhookEnvelope{
		ConnectionID: connectionID,
		OrgID:        orgID,
		EmployeeID:   employeeID,
	}
	result := &gateway.ReceiveConnectionResult{
		Inbound: gateway.InboundEnvelope{
			ChannelID: "C123",
			ThreadID:  "1710000000.123",
			SenderID:  "U123",
		},
		Session: model.EmployeeSession{
			ID: sessionID,
		},
		RuntimeConversationID: "gateway-conversation",
		RuntimeAPIKey:         "runtime-secret",
		RuntimeURL:            "https://runtime.example.com",
		StreamURL:             "https://runtime.example.com/gateway/http/streams/stream-123",
		TraceID:               "trace-123",
		TurnID:                "turn-123",
		ActionToken:           "action-token",
	}
	conn := &model.Connection{NangoConnectionID: "nango-conn"}

	payload := gatewaySlackPayload(envelope, result, conn, "slack")

	if payload.RuntimeAPIKey != result.RuntimeAPIKey {
		t.Fatalf("RuntimeAPIKey = %q, want %q", payload.RuntimeAPIKey, result.RuntimeAPIKey)
	}
	if payload.StreamURL != result.StreamURL {
		t.Fatalf("StreamURL = %q, want %q", payload.StreamURL, result.StreamURL)
	}
	if payload.NangoConnID != conn.NangoConnectionID {
		t.Fatalf("NangoConnID = %q, want %q", payload.NangoConnID, conn.NangoConnectionID)
	}
}
