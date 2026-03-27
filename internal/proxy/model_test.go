package proxy

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestExtractModel_OpenAIFormat(t *testing.T) {
	body := `{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}]}`
	req := makePostRequest(body)

	got := ExtractModel(req)
	if got != "gpt-4o" {
		t.Errorf("expected %q, got %q", "gpt-4o", got)
	}

	// Body must still be fully readable
	remaining, _ := io.ReadAll(req.Body)
	if string(remaining) != body {
		t.Errorf("body should be intact after extraction, got %q", string(remaining))
	}
}

func TestExtractModel_AnthropicFormat(t *testing.T) {
	body := `{"model":"claude-sonnet-4-20250514","max_tokens":1024,"messages":[{"role":"user","content":"hi"}]}`
	req := makePostRequest(body)

	got := ExtractModel(req)
	if got != "claude-sonnet-4-20250514" {
		t.Errorf("expected %q, got %q", "claude-sonnet-4-20250514", got)
	}

	remaining, _ := io.ReadAll(req.Body)
	if string(remaining) != body {
		t.Errorf("body should be intact")
	}
}

func TestExtractModel_GoogleFormat(t *testing.T) {
	// Google often puts model in the URL path, but some request formats include it in body
	body := `{"model":"gemini-1.5-pro","contents":[{"role":"user","parts":[{"text":"hello"}]}]}`
	req := makePostRequest(body)

	got := ExtractModel(req)
	if got != "gemini-1.5-pro" {
		t.Errorf("expected %q, got %q", "gemini-1.5-pro", got)
	}
}

func TestExtractModel_ModelNotFirstKey(t *testing.T) {
	body := `{"messages":[{"role":"user","content":"hello"}],"temperature":0.7,"model":"gpt-4","max_tokens":100}`
	req := makePostRequest(body)

	got := ExtractModel(req)
	if got != "gpt-4" {
		t.Errorf("expected %q, got %q", "gpt-4", got)
	}

	remaining, _ := io.ReadAll(req.Body)
	if string(remaining) != body {
		t.Errorf("body should be intact")
	}
}

func TestExtractModel_NoModelField(t *testing.T) {
	body := `{"messages":[{"role":"user","content":"hello"}],"temperature":0.7}`
	req := makePostRequest(body)

	got := ExtractModel(req)
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}

	remaining, _ := io.ReadAll(req.Body)
	if string(remaining) != body {
		t.Errorf("body should be intact")
	}
}

func TestExtractModel_EmptyBody(t *testing.T) {
	req := makePostRequest("")

	got := ExtractModel(req)
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestExtractModel_NotJSON(t *testing.T) {
	req, _ := http.NewRequest(http.MethodPost, "http://example.com", strings.NewReader("hello world"))
	req.Header.Set("Content-Type", "text/plain")

	got := ExtractModel(req)
	if got != "" {
		t.Errorf("expected empty string for non-JSON, got %q", got)
	}
}

func TestExtractModel_GETRequest(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
	req.Header.Set("Content-Type", "application/json")

	got := ExtractModel(req)
	if got != "" {
		t.Errorf("expected empty string for GET, got %q", got)
	}
}

func TestExtractModel_MalformedJSON(t *testing.T) {
	body := `{"model": broken json`
	req := makePostRequest(body)

	got := ExtractModel(req)
	if got != "" {
		t.Errorf("expected empty string for malformed JSON, got %q", got)
	}

	// Body should still be readable
	remaining, _ := io.ReadAll(req.Body)
	if string(remaining) != body {
		t.Errorf("body should be intact")
	}
}

func TestExtractModel_ModelNotString(t *testing.T) {
	body := `{"model":123,"messages":[]}`
	req := makePostRequest(body)

	got := ExtractModel(req)
	if got != "" {
		t.Errorf("expected empty string for non-string model, got %q", got)
	}
}

func TestExtractModel_LargeBody(t *testing.T) {
	// Model field first, then a large payload
	prefix := `{"model":"gpt-4o","messages":[{"role":"user","content":"`
	// Create content larger than maxPeekBytes
	content := strings.Repeat("x", maxPeekBytes*2)
	suffix := `"}]}`
	body := prefix + content + suffix

	req := makePostRequest(body)
	got := ExtractModel(req)
	if got != "gpt-4o" {
		t.Errorf("expected %q, got %q", "gpt-4o", got)
	}

	// Entire body must be readable (peeked portion + remaining)
	remaining, _ := io.ReadAll(req.Body)
	if string(remaining) != body {
		t.Errorf("body length mismatch: want %d, got %d", len(body), len(remaining))
	}
}

func TestExtractModel_NilBody(t *testing.T) {
	req, _ := http.NewRequest(http.MethodPost, "http://example.com", nil)
	req.Header.Set("Content-Type", "application/json")

	got := ExtractModel(req)
	if got != "" {
		t.Errorf("expected empty string for nil body, got %q", got)
	}
}

func TestExtractModel_NestedObjects(t *testing.T) {
	body := `{"metadata":{"key":"val","nested":{"deep":true}},"model":"claude-3-opus","max_tokens":100}`
	req := makePostRequest(body)

	got := ExtractModel(req)
	if got != "claude-3-opus" {
		t.Errorf("expected %q, got %q", "claude-3-opus", got)
	}
}

func TestExtractModel_ArraysBeforeModel(t *testing.T) {
	body := `{"tools":[{"type":"function","function":{"name":"get_weather"}}],"model":"gpt-4o-mini"}`
	req := makePostRequest(body)

	got := ExtractModel(req)
	if got != "gpt-4o-mini" {
		t.Errorf("expected %q, got %q", "gpt-4o-mini", got)
	}
}

func makePostRequest(body string) *http.Request {
	req, _ := http.NewRequest(http.MethodPost, "http://example.com/v1/chat/completions", io.NopCloser(bytes.NewBufferString(body)))
	req.Header.Set("Content-Type", "application/json")
	return req
}
