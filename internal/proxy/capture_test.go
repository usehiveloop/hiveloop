package proxy

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ziraloop/ziraloop/internal/observe"
)

func TestCaptureTransport_NonStreaming_OpenAI(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		fmt.Fprint(w, `{"id":"chatcmpl-123","choices":[{"message":{"content":"hello"}}],"usage":{"prompt_tokens":100,"completion_tokens":50}}`)
	}))
	defer upstream.Close()

	captured := &observe.CapturedData{ProviderID: "openai"}
	ctx := observe.WithCapturedData(context.Background(), captured)

	req, _ := http.NewRequestWithContext(ctx, "POST", upstream.URL, nil)
	ct := &CaptureTransport{Inner: http.DefaultTransport}
	resp, err := ct.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	// Body must still be readable
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "chatcmpl-123") {
		t.Error("response body should be intact")
	}

	if captured.Usage.InputTokens != 100 {
		t.Errorf("InputTokens = %d, want 100", captured.Usage.InputTokens)
	}
	if captured.Usage.OutputTokens != 50 {
		t.Errorf("OutputTokens = %d, want 50", captured.Usage.OutputTokens)
	}
	if captured.UpstreamStatus != 200 {
		t.Errorf("UpstreamStatus = %d, want 200", captured.UpstreamStatus)
	}
	if captured.IsStreaming {
		t.Error("should not be streaming")
	}
	if captured.TotalMs < 0 {
		t.Error("TotalMs should be non-negative")
	}
}

func TestCaptureTransport_NonStreaming_Anthropic(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"type":"message","usage":{"input_tokens":150,"output_tokens":80,"cache_read_input_tokens":40}}`)
	}))
	defer upstream.Close()

	captured := &observe.CapturedData{ProviderID: "anthropic"}
	ctx := observe.WithCapturedData(context.Background(), captured)
	req, _ := http.NewRequestWithContext(ctx, "POST", upstream.URL, nil)

	ct := &CaptureTransport{Inner: http.DefaultTransport}
	resp, err := ct.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	assertUsage(t, UsageData{
		InputTokens:  captured.Usage.InputTokens,
		OutputTokens: captured.Usage.OutputTokens,
		CachedTokens: captured.Usage.CachedTokens,
	}, 150, 80, 40, 0)
}

func TestCaptureTransport_Streaming_OpenAI(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		flusher := w.(http.Flusher)

		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n")
		flusher.Flush()

		time.Sleep(10 * time.Millisecond)

		fmt.Fprint(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":200,\"completion_tokens\":100,\"completion_tokens_details\":{\"reasoning_tokens\":15}}}\n\n")
		flusher.Flush()

		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer upstream.Close()

	captured := &observe.CapturedData{ProviderID: "openai"}
	ctx := observe.WithCapturedData(context.Background(), captured)
	req, _ := http.NewRequestWithContext(ctx, "POST", upstream.URL, nil)

	ct := &CaptureTransport{Inner: http.DefaultTransport}
	resp, err := ct.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}

	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if !strings.Contains(string(body), "data: ") {
		t.Error("streaming body should contain SSE events")
	}

	if captured.IsStreaming != true {
		t.Error("should be streaming")
	}
	if captured.Usage.InputTokens != 200 {
		t.Errorf("InputTokens = %d, want 200", captured.Usage.InputTokens)
	}
	if captured.Usage.OutputTokens != 100 {
		t.Errorf("OutputTokens = %d, want 100", captured.Usage.OutputTokens)
	}
	if captured.Usage.ReasoningTokens != 15 {
		t.Errorf("ReasoningTokens = %d, want 15", captured.Usage.ReasoningTokens)
	}
	if captured.TTFBMs < 0 {
		t.Error("TTFBMs should be non-negative")
	}
	if captured.TotalMs < 0 {
		t.Error("TotalMs should be non-negative")
	}
	if captured.TotalMs < captured.TTFBMs {
		t.Error("TotalMs should be >= TTFBMs")
	}
}

func TestCaptureTransport_Streaming_Anthropic(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)

		fmt.Fprint(w, "data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":150,\"output_tokens\":0}}}\n\n")
		flusher.Flush()

		fmt.Fprint(w, "data: {\"type\":\"content_block_delta\",\"delta\":{\"text\":\"hello\"}}\n\n")
		flusher.Flush()

		fmt.Fprint(w, "data: {\"type\":\"message_delta\",\"usage\":{\"input_tokens\":150,\"output_tokens\":80,\"cache_read_input_tokens\":40}}\n\n")
		flusher.Flush()

		fmt.Fprint(w, "data: {\"type\":\"message_stop\"}\n\n")
		flusher.Flush()
	}))
	defer upstream.Close()

	captured := &observe.CapturedData{ProviderID: "anthropic"}
	ctx := observe.WithCapturedData(context.Background(), captured)
	req, _ := http.NewRequestWithContext(ctx, "POST", upstream.URL, nil)

	ct := &CaptureTransport{Inner: http.DefaultTransport}
	resp, err := ct.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp.Body)
	resp.Body.Close()

	if captured.Usage.InputTokens != 150 {
		t.Errorf("InputTokens = %d, want 150", captured.Usage.InputTokens)
	}
	if captured.Usage.OutputTokens != 80 {
		t.Errorf("OutputTokens = %d, want 80", captured.Usage.OutputTokens)
	}
	if captured.Usage.CachedTokens != 40 {
		t.Errorf("CachedTokens = %d, want 40", captured.Usage.CachedTokens)
	}
}

func TestCaptureTransport_UpstreamError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(429)
		fmt.Fprint(w, `{"error":{"message":"rate limit exceeded"}}`)
	}))
	defer upstream.Close()

	captured := &observe.CapturedData{ProviderID: "openai"}
	ctx := observe.WithCapturedData(context.Background(), captured)
	req, _ := http.NewRequestWithContext(ctx, "POST", upstream.URL, nil)

	ct := &CaptureTransport{Inner: http.DefaultTransport}
	resp, err := ct.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}

	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), "rate limit") {
		t.Error("error body should be intact")
	}

	if captured.UpstreamStatus != 429 {
		t.Errorf("UpstreamStatus = %d, want 429", captured.UpstreamStatus)
	}
	if captured.ErrorType != "rate_limit" {
		t.Errorf("ErrorType = %q, want %q", captured.ErrorType, "rate_limit")
	}
}

func TestCaptureTransport_Upstream500(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		fmt.Fprint(w, `{"error":"internal"}`)
	}))
	defer upstream.Close()

	captured := &observe.CapturedData{ProviderID: "openai"}
	ctx := observe.WithCapturedData(context.Background(), captured)
	req, _ := http.NewRequestWithContext(ctx, "POST", upstream.URL, nil)

	ct := &CaptureTransport{Inner: http.DefaultTransport}
	resp, err := ct.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if captured.ErrorType != "upstream_error" {
		t.Errorf("ErrorType = %q, want %q", captured.ErrorType, "upstream_error")
	}
}

func TestCaptureTransport_Upstream401(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		fmt.Fprint(w, `{"error":"invalid api key"}`)
	}))
	defer upstream.Close()

	captured := &observe.CapturedData{ProviderID: "openai"}
	ctx := observe.WithCapturedData(context.Background(), captured)
	req, _ := http.NewRequestWithContext(ctx, "POST", upstream.URL, nil)

	ct := &CaptureTransport{Inner: http.DefaultTransport}
	resp, err := ct.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if captured.ErrorType != "auth" {
		t.Errorf("ErrorType = %q, want %q", captured.ErrorType, "auth")
	}
}

func TestCaptureTransport_NoCapturedContext(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer upstream.Close()

	req, _ := http.NewRequest("GET", upstream.URL, nil)
	ct := &CaptureTransport{Inner: http.DefaultTransport}
	resp, err := ct.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if string(body) != `{"ok":true}` {
		t.Errorf("body = %q, want %q", string(body), `{"ok":true}`)
	}
}

func TestCaptureTransport_ConnectionError(t *testing.T) {
	captured := &observe.CapturedData{ProviderID: "openai"}
	ctx := observe.WithCapturedData(context.Background(), captured)
	req, _ := http.NewRequestWithContext(ctx, "POST", "http://127.0.0.1:1", nil)

	ct := &CaptureTransport{Inner: http.DefaultTransport}
	_, err := ct.RoundTrip(req)
	if err == nil {
		t.Fatal("expected transport error")
	}

	if captured.ErrorType != "connection_error" {
		t.Errorf("ErrorType = %q, want %q", captured.ErrorType, "connection_error")
	}
	if captured.ErrorMessage == "" {
		t.Error("ErrorMessage should not be empty")
	}
}

func TestClassifyTransportError(t *testing.T) {
	tests := []struct {
		msg  string
		want string
	}{
		{"context deadline exceeded", "timeout"},
		{"dial tcp: i/o timeout", "timeout"},
		{"connection refused", "connection_error"},
		{"no such host", "connection_error"},
		{"something else", "transport_error"},
	}
	for _, tt := range tests {
		got := classifyTransportError(fmt.Errorf("%s", tt.msg))
		if got != tt.want {
			t.Errorf("classifyTransportError(%q) = %q, want %q", tt.msg, got, tt.want)
		}
	}
}

func TestClassifyHTTPError(t *testing.T) {
	tests := []struct {
		status int
		want   string
	}{
		{429, "rate_limit"},
		{401, "auth"},
		{403, "auth"},
		{500, "upstream_error"},
		{503, "upstream_error"},
		{400, "client_error"},
		{404, "client_error"},
	}
	for _, tt := range tests {
		got := classifyHTTPError(tt.status)
		if got != tt.want {
			t.Errorf("classifyHTTPError(%d) = %q, want %q", tt.status, got, tt.want)
		}
	}
}
