//go:build llm

package e2e

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// openRouterKeyCache memoises the validation result across tests within a run
// so we only hit OpenRouter once, not once per test.
var (
	openRouterKeyValidated bool
	openRouterKeyValid     bool
	openRouterValidatedKey string
)

func requireOpenRouterKey(t *testing.T) string {
	t.Helper()
	loadEnv(t)
	key := os.Getenv("OPENROUTER_API_KEY")
	if key == "" {
		t.Skip("OPENROUTER_API_KEY not set — skipping OpenRouter-dependent test")
	}

	if !openRouterKeyValidated || openRouterValidatedKey != key {
		openRouterValidatedKey = key
		openRouterKeyValidated = true
		openRouterKeyValid = validateOpenRouterKey(key)
	}
	if !openRouterKeyValid {
		t.Skip("OPENROUTER_API_KEY rejected by OpenRouter (rotate CI secret) — skipping")
	}
	return key
}

// validateOpenRouterKey hits OpenRouter's /auth/key endpoint with a short
// timeout. Any non-2xx response means the key is not usable for the rest of
// the suite.
func validateOpenRouterKey(key string) bool {
	req, err := http.NewRequest(http.MethodGet, "https://openrouter.ai/api/v1/auth/key", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+key)
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

func parseSSEChunks(t *testing.T, data []byte) []string {
	t.Helper()
	var chunks []string
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			chunk := strings.TrimPrefix(line, "data: ")
			chunk = strings.TrimSpace(chunk)
			if chunk != "" {
				chunks = append(chunks, chunk)
			}
		}
	}
	return chunks
}

// extractNonStreamContent safely extracts content from a non-streaming chat completion response.
func extractNonStreamContent(t *testing.T, resp map[string]any) string {
	t.Helper()
	choices, ok := resp["choices"].([]any)
	if !ok || len(choices) == 0 {
		t.Fatalf("no choices in response: %v", resp)
	}
	choice, ok := choices[0].(map[string]any)
	if !ok {
		t.Fatalf("invalid choice format: %v", choices[0])
	}
	msg, ok := choice["message"].(map[string]any)
	if !ok {
		t.Fatalf("no message in choice: %v", choice)
	}
	content, _ := msg["content"].(string)
	return content
}

func extractStreamContent(chunks []string) string {
	var sb strings.Builder
	for _, chunk := range chunks {
		if chunk == "[DONE]" {
			continue
		}
		var event map[string]any
		if err := json.Unmarshal([]byte(chunk), &event); err != nil {
			continue
		}
		choices, ok := event["choices"].([]any)
		if !ok || len(choices) == 0 {
			continue
		}
		delta, ok := choices[0].(map[string]any)["delta"].(map[string]any)
		if !ok {
			continue
		}
		if content, ok := delta["content"].(string); ok {
			sb.WriteString(content)
		}
	}
	return sb.String()
}
