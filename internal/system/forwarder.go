package system

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ForwardCall holds everything the forwarder needs for one upstream call.
// All fields are required; the forwarder doesn't reach into a wider
// dependency graph — the handler resolves credential + model upfront.
type ForwardCall struct {
	BaseURL    string // e.g. "https://api.openai.com/v1"
	APIKey     string // bare key (already decrypted by caller)
	AuthScheme string // "bearer" / "x-api-key" / "api-key"
	Request    *LLMRequest
	Stream     bool
}

// Forwarder issues the upstream HTTP call. Single instance is safe for
// concurrent use.
type Forwarder struct {
	HTTPClient *http.Client
}

// NewForwarder builds a forwarder with sensible defaults. Pass a nil client
// to use a fresh one; pass a wrapped one (e.g. proxy.CaptureTransport) to
// keep usage capture working.
func NewForwarder(c *http.Client) *Forwarder {
	if c == nil {
		c = &http.Client{Timeout: 60 * time.Second}
	}
	return &Forwarder{HTTPClient: c}
}

// ForwardJSON runs a non-streaming call and returns the parsed result. The
// caller is responsible for writing the response (cache write, JSON encode
// for the client, etc.).
func (f *Forwarder) ForwardJSON(ctx context.Context, call ForwardCall) (*CompletionResult, error) {
	if call.Stream {
		return nil, errors.New("ForwardJSON: call.Stream must be false")
	}
	resp, err := f.do(ctx, call)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, &UpstreamError{
			StatusCode: resp.StatusCode,
			Body:       string(body),
		}
	}

	var raw openAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode upstream response: %w", err)
	}
	if len(raw.Choices) == 0 {
		return nil, errors.New("upstream returned zero choices")
	}
	return &CompletionResult{
		Text:  raw.Choices[0].Message.Content,
		Model: raw.Model,
		Usage: Usage{
			InputTokens:  raw.Usage.PromptTokens,
			OutputTokens: raw.Usage.CompletionTokens,
		},
	}, nil
}

// ForwardStream runs a streaming call. As upstream OpenAI SSE chunks arrive
// the forwarder rewrites them to Hiveloop's envelope and writes them to w
// (which must implement http.Flusher), and tees the deltas into a buffer
// that becomes the cached result on clean completion. The returned
// CompletionResult is only populated when the upstream stream finished
// cleanly (a non-nil error means the cache MUST NOT be written).
func (f *Forwarder) ForwardStream(ctx context.Context, call ForwardCall, w http.ResponseWriter) (*CompletionResult, error) {
	if !call.Stream {
		return nil, errors.New("ForwardStream: call.Stream must be true")
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, errors.New("ResponseWriter does not implement http.Flusher")
	}

	resp, err := f.do(ctx, call)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, &UpstreamError{
			StatusCode: resp.StatusCode,
			Body:       string(body),
		}
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	var (
		textBuf  strings.Builder
		usage    Usage
		modelOut string
	)

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)

	for scanner.Scan() {
		line := scanner.Text()
		// OpenAI SSE: each event is `data: <json>` followed by a blank line.
		// We don't care about empty lines or comment lines (`:`).
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}
		if payload == "[DONE]" {
			break
		}
		var chunk openAIStreamChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			// Skip malformed chunks rather than abort the whole stream.
			// OpenAI sends an occasional comment that we already filter,
			// but third-party providers vary.
			continue
		}
		if chunk.Model != "" {
			modelOut = chunk.Model
		}
		if chunk.Usage != nil {
			usage = Usage{
				InputTokens:  chunk.Usage.PromptTokens,
				OutputTokens: chunk.Usage.CompletionTokens,
			}
		}
		if len(chunk.Choices) > 0 {
			delta := chunk.Choices[0].Delta.Content
			if delta != "" {
				textBuf.WriteString(delta)
				if err := writeSSE(w, sseDelta{Delta: delta}); err != nil {
					return nil, err
				}
				flusher.Flush()
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read upstream stream: %w", err)
	}

	if err := writeSSE(w, sseDone{
		Done:  true,
		Usage: usage,
	}); err != nil {
		return nil, err
	}
	flusher.Flush()

	return &CompletionResult{
		Text:  textBuf.String(),
		Model: modelOut,
		Usage: usage,
	}, nil
}

// EmitCachedSSE writes a Hiveloop-shaped SSE response from a cached result.
// One delta chunk + one done chunk with cached:true. Used by the handler
// when a streaming request hits the cache.
func EmitCachedSSE(w http.ResponseWriter, cached *CompletionResult) error {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return errors.New("ResponseWriter does not implement http.Flusher")
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	if err := writeSSE(w, sseDelta{Delta: cached.Text}); err != nil {
		return err
	}
	flusher.Flush()
	if err := writeSSE(w, sseDone{
		Done:   true,
		Usage:  cached.Usage,
		Cached: true,
	}); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

func (f *Forwarder) do(ctx context.Context, call ForwardCall) (*http.Response, error) {
	body, err := json.Marshal(call.Request)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	url := strings.TrimRight(call.BaseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if call.Stream {
		req.Header.Set("Accept", "text/event-stream")
	}
	attachAuth(req, call.AuthScheme, call.APIKey)
	return f.HTTPClient.Do(req)
}

func attachAuth(req *http.Request, scheme, apiKey string) {
	switch scheme {
	case "x-api-key":
		req.Header.Set("x-api-key", apiKey)
	case "api-key":
		req.Header.Set("api-key", apiKey)
	default: // bearer is the default
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
}

// UpstreamError signals an upstream non-2xx. The handler maps it to a 502
// with the body preserved for debugging.
type UpstreamError struct {
	StatusCode int
	Body       string
}

func (e *UpstreamError) Error() string {
	return fmt.Sprintf("upstream %d: %s", e.StatusCode, truncate(e.Body, 300))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// --- OpenAI wire types ---

type openAIResponse struct {
	Model   string         `json:"model"`
	Choices []openAIChoice `json:"choices"`
	Usage   openAIUsage    `json:"usage"`
}

type openAIChoice struct {
	Message openAIMessage `json:"message"`
}

type openAIMessage struct {
	Content string `json:"content"`
}

type openAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

type openAIStreamChunk struct {
	Model   string               `json:"model"`
	Choices []openAIStreamChoice `json:"choices"`
	Usage   *openAIUsage         `json:"usage,omitempty"`
}

type openAIStreamChoice struct {
	Delta openAIStreamDelta `json:"delta"`
}

type openAIStreamDelta struct {
	Content string `json:"content"`
}

// --- Hiveloop SSE envelope ---

type sseDelta struct {
	Delta string `json:"delta"`
}

type sseDone struct {
	Done   bool  `json:"done"`
	Usage  Usage `json:"usage"`
	Cached bool  `json:"cached,omitempty"`
}

func writeSSE(w io.Writer, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "data: %s\n\n", raw)
	return err
}
