//go:build llm

package e2e

import (
	"encoding/json"
	"fmt"
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

// --------------------------------------------------------------------------
// Fireworks: Non-streaming — Llama 3.3 70B
// --------------------------------------------------------------------------

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
	json.NewDecoder(rr.Body).Decode(&resp)
	content := extractNonStreamContent(t, resp)
	if content == "" {
		t.Fatal("empty response")
	}
	t.Logf("Llama 70B response: %s", content)
}

// --------------------------------------------------------------------------
// Fireworks: SSE streaming — Qwen3 8B
// --------------------------------------------------------------------------

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

// --------------------------------------------------------------------------
// Fireworks: Tool/function calling — Llama 3.3 70B
// --------------------------------------------------------------------------

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
	json.NewDecoder(rr.Body).Decode(&resp)
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

// --------------------------------------------------------------------------
// Fireworks: Streaming tool calls — Llama 3.3 70B
// --------------------------------------------------------------------------

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

// --------------------------------------------------------------------------
// Fireworks: DeepSeek V3.1 streaming
// --------------------------------------------------------------------------

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

// --------------------------------------------------------------------------
// Fireworks: Multi-turn conversation — Kimi K2
// --------------------------------------------------------------------------

func TestE2E_Fireworks_KimiK2_MultiTurn(t *testing.T) {
	apiKey := requireFireworksKey(t)
	h, tok, proxyPath := fireworksSetup(t, apiKey)

	// Turn 1
	payload1 := `{
		"model": "accounts/fireworks/models/kimi-k2-instruct-0905",
		"messages": [{"role": "user", "content": "The secret code is GAMMA-42. Remember it."}],
		"stream": false,
		"max_tokens": 50
	}`
	rr1 := h.proxyRequest(t, "POST", proxyPath, tok, strings.NewReader(payload1))
	if rr1.Code != 200 {
		t.Fatalf("turn 1: expected 200, got %d: %s", rr1.Code, rr1.Body.String())
	}
	var resp1 map[string]any
	json.NewDecoder(rr1.Body).Decode(&resp1)
	assistant1 := extractNonStreamContent(t, resp1)

	// Turn 2
	payload2 := fmt.Sprintf(`{
		"model": "accounts/fireworks/models/kimi-k2-instruct-0905",
		"messages": [
			{"role": "user", "content": "The secret code is GAMMA-42. Remember it."},
			{"role": "assistant", "content": %q},
			{"role": "user", "content": "What is the secret code? Reply with just the code."}
		],
		"stream": false,
		"max_tokens": 30
	}`, assistant1)

	rr2 := h.proxyRequest(t, "POST", proxyPath, tok, strings.NewReader(payload2))
	if rr2.Code != 200 {
		t.Fatalf("turn 2: expected 200, got %d: %s", rr2.Code, rr2.Body.String())
	}
	var resp2 map[string]any
	json.NewDecoder(rr2.Body).Decode(&resp2)
	answer := extractNonStreamContent(t, resp2)

	if !strings.Contains(strings.ToUpper(answer), "GAMMA-42") {
		t.Fatalf("expected GAMMA-42 in response, got: %s", answer)
	}
	t.Logf("Multi-turn verified: %s", answer)
}

// --------------------------------------------------------------------------
// Fireworks: DeepSeek V3 non-streaming
// --------------------------------------------------------------------------

func TestE2E_Fireworks_DeepSeek_NonStreaming(t *testing.T) {
	apiKey := requireFireworksKey(t)
	h, tok, proxyPath := fireworksSetup(t, apiKey)

	payload := `{
		"model": "accounts/fireworks/models/deepseek-v3p2",
		"messages": [{"role": "user", "content": "What is the capital of Japan? Reply with just the city name."}],
		"stream": false,
		"max_tokens": 100
	}`

	rr := h.proxyRequest(t, "POST", proxyPath, tok, strings.NewReader(payload))
	if rr.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	content := extractNonStreamContent(t, resp)
	if content == "" {
		t.Fatal("empty response from DeepSeek")
	}
	if !strings.Contains(strings.ToLower(content), "tokyo") {
		t.Logf("warning: expected 'Tokyo' in response, got: %s", content)
	}
	t.Logf("DeepSeek V3 response: %s", content)
}

// --------------------------------------------------------------------------
// Fireworks: Kimi K2.5 streaming (reasoning_content format)
// --------------------------------------------------------------------------

func TestE2E_Fireworks_KimiK2p5_Streaming(t *testing.T) {
	apiKey := requireFireworksKey(t)
	h, tok, proxyPath := fireworksSetup(t, apiKey)

	payload := `{
		"model": "accounts/fireworks/models/kimi-k2p5",
		"messages": [{"role": "user", "content": "What is 2+2? Reply with just the number."}],
		"stream": true,
		"max_tokens": 50
	}`

	rr := h.proxyRequest(t, "POST", proxyPath, tok, strings.NewReader(payload))
	if rr.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	chunks := parseSSEChunks(t, rr.Body.Bytes())
	if len(chunks) == 0 {
		t.Fatal("no SSE chunks from Kimi K2.5")
	}

	// Kimi K2.5 uses reasoning_content in delta; extract both content and reasoning_content
	var contentBuilder, reasoningBuilder strings.Builder
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
		if c, ok := delta["content"].(string); ok {
			contentBuilder.WriteString(c)
		}
		if r, ok := delta["reasoning_content"].(string); ok {
			reasoningBuilder.WriteString(r)
		}
	}

	content := contentBuilder.String()
	reasoning := reasoningBuilder.String()
	combined := content + reasoning
	if combined == "" {
		t.Fatal("no content or reasoning_content in stream from Kimi K2.5")
	}
	t.Logf("Kimi K2.5 streaming (%d chunks): content=%q reasoning=%q", len(chunks), content, reasoning)
}
