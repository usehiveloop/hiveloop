package proxy

import (
	"net/http"
	"net/url"
	"testing"
)

func TestAttachAuth_Bearer(t *testing.T) {
	req := &http.Request{Header: make(http.Header)}
	AttachAuth(req, "bearer", []byte("sk-test-key"))

	got := req.Header.Get("Authorization")
	want := "Bearer sk-test-key"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestAttachAuth_XAPIKey(t *testing.T) {
	req := &http.Request{Header: make(http.Header)}
	AttachAuth(req, "x-api-key", []byte("sk-ant-test"))

	got := req.Header.Get("x-api-key")
	want := "sk-ant-test"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestAttachAuth_APIKey(t *testing.T) {
	req := &http.Request{Header: make(http.Header)}
	AttachAuth(req, "api-key", []byte("azure-key-123"))

	got := req.Header.Get("api-key")
	want := "azure-key-123"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestAttachAuth_QueryParam(t *testing.T) {
	req := &http.Request{
		Header: make(http.Header),
		URL:    &url.URL{RawQuery: "existing=param"},
	}
	AttachAuth(req, "query_param", []byte("AIza-test-key"))

	got := req.URL.Query().Get("key")
	want := "AIza-test-key"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}

	// Verify existing params preserved
	if req.URL.Query().Get("existing") != "param" {
		t.Error("existing query params should be preserved")
	}
}

func TestAttachAuth_PreservesOtherHeaders(t *testing.T) {
	req := &http.Request{Header: make(http.Header)}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Custom", "value")

	AttachAuth(req, "bearer", []byte("sk-test"))

	if req.Header.Get("Content-Type") != "application/json" {
		t.Error("Content-Type header should be preserved")
	}
	if req.Header.Get("X-Custom") != "value" {
		t.Error("X-Custom header should be preserved")
	}
}

func TestAttachAuth_UnknownScheme(t *testing.T) {
	req := &http.Request{Header: make(http.Header)}
	AttachAuth(req, "unknown", []byte("key"))

	if req.Header.Get("Authorization") != "" {
		t.Error("unknown scheme should not set Authorization")
	}
}
