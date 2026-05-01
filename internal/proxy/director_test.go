package proxy

import (
	"bytes"
	"context"
	"net/http"
	"testing"

	"github.com/usehiveloop/hiveloop/internal/cache"
	"github.com/usehiveloop/hiveloop/internal/middleware"
)

// stubCredentialResolver returns a fixed credential for testing.
type stubCredentialResolver struct {
	cred *cache.DecryptedCredential
	err  error
}

func (s *stubCredentialResolver) GetDecryptedCredentialByID(_ context.Context, _ string) (*cache.DecryptedCredential, error) {
	return s.cred, s.err
}

// cloudflareHeaders is the set of headers Cloudflare injects inbound and that
// the director must strip before forwarding to the upstream.
var cloudflareHeaders = map[string]string{
	"CF-Connecting-IP":   "1.2.3.4",
	"CF-Ray":             "abc123-LAX",
	"CF-Visitor":         `{"scheme":"https"}`,
	"CF-IPCountry":       "US",
	"CF-Connecting-IPv6": "::ffff:1.2.3.4",
	"True-Client-IP":     "1.2.3.4",
	"CDN-Loop":           "cloudflare",
	"X-Forwarded-For":    "1.2.3.4, 5.6.7.8",
	"X-Forwarded-Proto":  "https",
	"X-Forwarded-Host":   "proxy.usehiveloop.com",
	"X-Real-IP":          "5.6.7.8",
}

func TestDirector_StripsCloudflareAndForwardedHeaders(t *testing.T) {
	// Allow loopback so ValidateBaseURL doesn't need real DNS.
	original := AllowLoopback
	AllowLoopback = true
	t.Cleanup(func() { AllowLoopback = original })

	stub := &stubCredentialResolver{
		cred: &cache.DecryptedCredential{
			BaseURL:    "https://upstream.example.com/v1",
			AuthScheme: "bearer",
			APIKey:     []byte("sk-test"),
		},
	}

	director := NewDirector(stub)

	body := bytes.NewReader([]byte(`{"model":"gpt-4","messages":[{"role":"user","content":"hello"}]}`))
	req, err := http.NewRequest(http.MethodPost, "/v1/proxy/chat/completions", body)
	if err != nil {
		t.Fatal(err)
	}

	// Set inbound headers (including the proxy token auth).
	for k, v := range cloudflareHeaders {
		req.Header.Set(k, v)
	}
	req.Header.Set("Authorization", "Bearer ptok_test")
	req.Header.Set("Content-Type", "application/json")

	// Attach token claims the director needs.
	req = middleware.WithClaims(req, &middleware.TokenClaims{
		OrgID:        "org_123",
		CredentialID: "cred_456",
		JTI:          "jti_789",
	})

	// Sanity check: headers present before director runs.
	for h := range cloudflareHeaders {
		if req.Header.Get(h) == "" {
			t.Fatalf("test setup: expected header %q to be set before director runs", h)
		}
	}
	if req.Header.Get("Authorization") != "Bearer ptok_test" {
		t.Fatalf("test setup: expected Authorization header to be set before director runs")
	}

	director(req)

	// 1. Assert all Cloudflare and forwarded headers are stripped.
	for h := range cloudflareHeaders {
		if v := req.Header.Get(h); v != "" {
			t.Errorf("director should strip header %q, but got %q", h, v)
		}
	}

	// 2. Assert URL was rewritten correctly.
	wantURL := "https://upstream.example.com/v1/chat/completions"
	if req.URL.String() != wantURL {
		t.Errorf("expected URL %q, got %q", wantURL, req.URL.String())
	}
	if req.Host != "upstream.example.com" {
		t.Errorf("expected Host %q, got %q", "upstream.example.com", req.Host)
	}

	// 3. Assert Authorization is the upstream key, not the inbound ptok.
	if got, want := req.Header.Get("Authorization"), "Bearer sk-test"; got != want {
		t.Errorf("expected Authorization %q, got %q", want, got)
	}

	// 4. Assert X-Request-ID was set.
	if req.Header.Get("X-Request-ID") == "" {
		t.Error("expected X-Request-ID to be set after director runs")
	}
}
