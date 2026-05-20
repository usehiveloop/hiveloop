//go:build llm

package e2e

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func requireFireworksKey(t *testing.T) string {
	t.Helper()
	loadEnv(t)
	key := os.Getenv("FIREWORKS_API_KEY")
	if key == "" {
		t.Fatal("FIREWORKS_API_KEY must be set")
	}
	return key
}

// fireworksSetup creates a harness with a Fireworks credential and token.
func fireworksSetup(t *testing.T, apiKey string) (*testHarness, string, string) {
	t.Helper()
	h := newHarness(t)
	org := h.createOrg(t)
	cred := h.storeCredential(t, org, "https://api.fireworks.ai/inference", "bearer", apiKey)
	tok := h.mintToken(t, org, cred.ID)
	proxyPath := "/v1/proxy/v1/chat/completions"
	return h, tok, proxyPath
}

func TestE2E_Fireworks_Llama70B_NonStreaming(t *testing.T) {
	apiKey := requireFireworksKey(t)
	h, tok, proxyPath := fireworksSetup(t, apiKey)

	payload := `{
		"model": "accounts/fireworks/models/llama-v3p3-70b-instruct",
		"messages": [{"role": "user", "content": "Reply with exactly: hello from fireworks"}],
		"stream": false,
		"max_tokens": 20
	}`

	rr := h.proxyRequest(t, "POST", proxyPath, tok, strings.NewReader(payload))
	if rr.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	content := extractNonStreamContent(t, resp)
	if content == "" {
		t.Fatal("empty response")
	}
	t.Logf("Llama 70B response: %s", content)
}

func TestE2E_Fireworks_Qwen3_Streaming(t *testing.T) {
	apiKey := requireFireworksKey(t)
	h, tok, proxyPath := fireworksSetup(t, apiKey)

	payload := `{
		"model": "accounts/fireworks/models/qwen3-8b",
		"messages": [{"role": "user", "content": "Count from 1 to 5, one number per line. No extra text."}],
		"stream": true,
		"max_tokens": 50
	}`

	rr := h.proxyRequest(t, "POST", proxyPath, tok, strings.NewReader(payload))
	if rr.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	chunks := parseSSEChunks(t, rr.Body.Bytes())
	if len(chunks) == 0 {
		t.Fatal("no SSE chunks received")
	}

	content := extractStreamContent(chunks)
	if content == "" {
		t.Fatal("no content in stream")
	}
	t.Logf("Qwen3 8B streaming (%d chunks): %s", len(chunks), content)
}

func TestE2E_Fireworks_Llama70B_ToolCalls(t *testing.T) {
	apiKey := requireFireworksKey(t)
	h, tok, proxyPath := fireworksSetup(t, apiKey)

	payload := `{
		"model": "accounts/fireworks/models/llama-v3p3-70b-instruct",
		"messages": [{"role": "user", "content": "What is the current temperature in Paris?"}],
		"tools": [{
			"type": "function",
			"function": {
				"name": "get_temperature",
				"description": "Get the current temperature for a city",
				"parameters": {
					"type": "object",
					"properties": {
						"city": {"type": "string", "description": "The city name"}
					},
					"required": ["city"]
				}
			}
		}],
		"stream": false,
		"max_tokens": 150
	}`

	rr := h.proxyRequest(t, "POST", proxyPath, tok, strings.NewReader(payload))
	if rr.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	choices := resp["choices"].([]any)
	msg := choices[0].(map[string]any)["message"].(map[string]any)

	toolCalls, hasTools := msg["tool_calls"].([]any)
	content, _ := msg["content"].(string)

	if hasTools && len(toolCalls) > 0 {
		tc := toolCalls[0].(map[string]any)
		fn := tc["function"].(map[string]any)
		t.Logf("Tool call: %s(%s)", fn["name"], fn["arguments"])
		if fn["name"] != "get_temperature" {
			t.Fatalf("expected get_temperature, got %s", fn["name"])
		}
		args := fn["arguments"].(string)
		if !strings.Contains(strings.ToLower(args), "paris") {
			t.Logf("warning: args don't mention Paris: %s", args)
		}
	} else if content != "" {
		t.Logf("Model responded with content instead of tool call: %s", content)
	} else {
		t.Fatal("no tool calls and no content in response")
	}
}

func TestE2E_Fireworks_Llama70B_StreamingToolCalls(t *testing.T) {
	apiKey := requireFireworksKey(t)
	h, tok, proxyPath := fireworksSetup(t, apiKey)

	payload := `{
		"model": "accounts/fireworks/models/llama-v3p3-70b-instruct",
		"messages": [{"role": "user", "content": "Look up the population of Berlin."}],
		"tools": [{
			"type": "function",
			"function": {
				"name": "lookup_population",
				"description": "Look up the population of a city",
				"parameters": {
					"type": "object",
					"properties": {
						"city": {"type": "string", "description": "City name"}
					},
					"required": ["city"]
				}
			}
		}],
		"stream": true,
		"max_tokens": 150
	}`

	rr := h.proxyRequest(t, "POST", proxyPath, tok, strings.NewReader(payload))
	if rr.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	chunks := parseSSEChunks(t, rr.Body.Bytes())
	if len(chunks) == 0 {
		t.Fatal("no SSE chunks")
	}

	var toolName string
	var toolArgs strings.Builder
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
		if tcs, ok := delta["tool_calls"].([]any); ok && len(tcs) > 0 {
			tc := tcs[0].(map[string]any)
			if fn, ok := tc["function"].(map[string]any); ok {
				if name, ok := fn["name"].(string); ok && name != "" {
					toolName = name
				}
				if args, ok := fn["arguments"].(string); ok {
					toolArgs.WriteString(args)
				}
			}
		}
	}

	if toolName != "" {
		t.Logf("Streaming tool call: %s(%s)", toolName, toolArgs.String())
		if toolName != "lookup_population" {
			t.Fatalf("expected lookup_population, got %s", toolName)
		}
	} else {
		content := extractStreamContent(chunks)
		t.Logf("Model responded with content instead of streaming tool call: %s", content)
	}
}

func TestE2E_Fireworks_DeepSeekV3p1_Streaming(t *testing.T) {
	apiKey := requireFireworksKey(t)
	h, tok, proxyPath := fireworksSetup(t, apiKey)

	payload := `{
		"model": "accounts/fireworks/models/deepseek-v3p1",
		"messages": [{"role": "user", "content": "What is 7 * 8? Reply with just the number."}],
		"stream": true,
		"max_tokens": 30
	}`

	rr := h.proxyRequest(t, "POST", proxyPath, tok, strings.NewReader(payload))
	if rr.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	chunks := parseSSEChunks(t, rr.Body.Bytes())
	content := extractStreamContent(chunks)
	if content == "" {
		t.Skip("empty stream from DeepSeek V3.1 — model may be temporarily unavailable")
	}
	t.Logf("DeepSeek V3.1 streaming (%d chunks): %s", len(chunks), content)
}
