package system

import (
	"encoding/json"
	"fmt"
	"io"
)

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
