package system

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newFakeUpstream(t *testing.T, handler http.HandlerFunc) (baseURL string, captured *capturedRequest) {
	t.Helper()
	captured = &capturedRequest{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		captured.auth = r.Header.Get("Authorization")
		captured.body, _ = io.ReadAll(r.Body)
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/v1", captured
}

type capturedRequest struct {
	auth string
	body []byte
}

func TestForward_NonStreaming_ReturnsTextAndUsage(t *testing.T) {
	base, cap := newFakeUpstream(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"model":"fake-model",
			"choices":[{"message":{"content":"hello world"}}],
			"usage":{"prompt_tokens":7,"completion_tokens":3}
		}`))
	})

	res, err := NewForwarder(nil).ForwardJSON(context.Background(), ForwardCall{
		BaseURL:    base,
		APIKey:     "sk-fake",
		AuthScheme: "bearer",
		Request: &LLMRequest{
			Model:    "fake-model",
			Messages: []LLMMessage{{Role: "user", Content: "hi"}},
		},
	})
	if err != nil {
		t.Fatalf("forward: %v", err)
	}
	if res.Text != "hello world" {
		t.Fatalf("text = %q", res.Text)
	}
	if res.Usage.InputTokens != 7 || res.Usage.OutputTokens != 3 {
		t.Fatalf("usage = %+v", res.Usage)
	}
	if cap.auth != "Bearer sk-fake" {
		t.Fatalf("auth header = %q", cap.auth)
	}
}

func TestForward_NonStreaming_UpstreamErrorReturnsTyped(t *testing.T) {
	base, _ := newFakeUpstream(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad model"}`))
	})

	_, err := NewForwarder(nil).ForwardJSON(context.Background(), ForwardCall{
		BaseURL: base, APIKey: "k", AuthScheme: "bearer",
		Request: &LLMRequest{Model: "x"},
	})
	var upErr *UpstreamError
	if !errors.As(err, &upErr) {
		t.Fatalf("expected *UpstreamError, got %v", err)
	}
	if upErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d", upErr.StatusCode)
	}
}

const upstreamSSE = `data: {"model":"m","choices":[{"delta":{"content":"hello "}}]}

data: {"model":"m","choices":[{"delta":{"content":"world"}}]}

data: {"model":"m","choices":[{"delta":{}}],"usage":{"prompt_tokens":4,"completion_tokens":2}}

data: [DONE]

`

func TestForward_Streaming_RewrapsToHiveloopShape(t *testing.T) {
	base, cap := newFakeUpstream(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, upstreamSSE)
	})

	rw := newFlushRecorder()
	res, err := NewForwarder(nil).ForwardStream(context.Background(), ForwardCall{
		BaseURL: base, APIKey: "sk", AuthScheme: "bearer",
		Request: &LLMRequest{Model: "m", Stream: true},
		Stream:  true,
	}, rw)
	if err != nil {
		t.Fatalf("forward: %v", err)
	}
	if cap.auth != "Bearer sk" {
		t.Fatalf("auth header = %q", cap.auth)
	}
	if res.Text != "hello world" {
		t.Fatalf("buffered text = %q (want 'hello world')", res.Text)
	}
	if res.Usage.InputTokens != 4 || res.Usage.OutputTokens != 2 {
		t.Fatalf("usage = %+v", res.Usage)
	}
	if rw.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("missing SSE Content-Type")
	}

	deltas, done := parseHiveloopSSE(t, rw.body.String())
	if len(deltas) != 2 || deltas[0] != "hello " || deltas[1] != "world" {
		t.Fatalf("deltas = %q", deltas)
	}
	if !done.Done {
		t.Fatalf("done frame missing or not done:true")
	}
	if done.Usage.OutputTokens != 2 {
		t.Fatalf("done.usage = %+v", done.Usage)
	}
	if rw.flushCount < 3 {
		t.Fatalf("expected at least 3 flushes (2 deltas + done), got %d", rw.flushCount)
	}
}

func TestEmitCachedSSE_OneDeltaPlusCachedDone(t *testing.T) {
	rw := newFlushRecorder()
	cached := &CompletionResult{
		Text:  "the moon is bright",
		Usage: Usage{InputTokens: 5, OutputTokens: 4},
	}
	if err := EmitCachedSSE(rw, cached); err != nil {
		t.Fatalf("emit: %v", err)
	}
	deltas, done := parseHiveloopSSE(t, rw.body.String())
	if len(deltas) != 1 || deltas[0] != "the moon is bright" {
		t.Fatalf("deltas = %q", deltas)
	}
	if !done.Done || !done.Cached {
		t.Fatalf("done frame: %+v (Done+Cached must both be true)", done)
	}
}

// flushRecorder wraps httptest.ResponseRecorder with a Flush counter so
// streaming tests can assert chunks were flushed individually.
type flushRecorder struct {
	*httptest.ResponseRecorder
	body       *strings.Builder
	flushCount int
}

func newFlushRecorder() *flushRecorder {
	return &flushRecorder{
		ResponseRecorder: httptest.NewRecorder(),
		body:             &strings.Builder{},
	}
}

func (r *flushRecorder) Write(p []byte) (int, error) {
	n, err := r.ResponseRecorder.Write(p)
	r.body.Write(p[:n])
	return n, err
}

func (r *flushRecorder) Flush() {
	r.flushCount++
}

type parsedDone struct {
	Done   bool  `json:"done"`
	Usage  Usage `json:"usage"`
	Cached bool  `json:"cached"`
}

func parseHiveloopSSE(t *testing.T, body string) ([]string, parsedDone) {
	t.Helper()
	var deltas []string
	var done parsedDone
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}
		var d sseDelta
		if json.Unmarshal([]byte(payload), &d) == nil && d.Delta != "" {
			deltas = append(deltas, d.Delta)
			continue
		}
		_ = json.Unmarshal([]byte(payload), &done)
	}
	return deltas, done
}
