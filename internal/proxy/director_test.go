package proxy

import "testing"

func TestJoinUpstreamPath(t *testing.T) {
	tests := []struct {
		name         string
		basePath     string
		upstreamPath string
		want         string
	}{
		{"v1 dedup", "/v1", "/v1/chat/completions", "/v1/chat/completions"},
		{"v2 dedup", "/v2", "/v2/messages", "/v2/messages"},
		{"api/v1 dedup", "/api/v1", "/api/v1/foo/bar", "/api/v1/foo/bar"},
		{"api/v2 dedup", "/api/v2", "/api/v2/x", "/api/v2/x"},
		{"empty basePath passthrough", "", "/v1/chat/completions", "/v1/chat/completions"},
		{"no overlap concat", "/v1", "/chat/completions", "/v1/chat/completions"},
		{"exact-match v1", "/v1", "/v1", "/v1"},
		{"exact-match api/v1", "/api/v1", "/api/v1", "/api/v1"},
		{"v1 substring should not dedup mid-path", "/foo", "/v1/x", "/foo/v1/x"},
		{"prefix-only no trailing slash should not collapse", "/v1", "/v10/x", "/v1/v10/x"},
		{"basePath /api/v1 longer prefix wins over /v1", "/api/v1", "/api/v1/x", "/api/v1/x"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := joinUpstreamPath(tc.basePath, tc.upstreamPath)
			if got != tc.want {
				t.Fatalf("joinUpstreamPath(%q, %q) = %q; want %q",
					tc.basePath, tc.upstreamPath, got, tc.want)
			}
		})
	}
}
