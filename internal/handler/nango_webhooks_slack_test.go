package handler

import (
	"net/http"
	"testing"

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
