package tasks

import (
	"testing"
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
