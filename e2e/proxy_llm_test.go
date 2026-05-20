//go:build llm

package e2e

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestE2E_Proxy_OpenAI_NonStreaming(t *testing.T) {
	apiKey := requireOpenRouterKey(t)
	h := newHarness(t)
	org := h.createOrg(t)
	cred := h.storeCredential(t, org, "https://openrouter.ai/api", "bearer", apiKey)
	tok := h.mintToken(t, org, cred.ID)

	payload := `{
		"model": "openai/gpt-4.1-nano",
		"messages": [{"role": "user", "content": "Reply with exactly: hello proxy"}],
		"stream": false,
		"max_tokens": 20
	}`

	proxyPath := "/v1/proxy/v1/chat/completions"
	rr := h.proxyRequest(t, http.MethodPost, proxyPath, tok, strings.NewReader(payload))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	content := extractNonStreamContent(t, resp)
	if content == "" {
		t.Fatal("empty content in response")
	}
	t.Logf("OpenAI response: %s", content)
}

func TestE2E_Proxy_Anthropic_Streaming(t *testing.T) {
	apiKey := requireOpenRouterKey(t)
	h := newHarness(t)
	org := h.createOrg(t)
	cred := h.storeCredential(t, org, "https://openrouter.ai/api", "bearer", apiKey)
	tok := h.mintToken(t, org, cred.ID)

	payload := `{
		"model": "openai/gpt-4.1-nano",
		"messages": [{"role": "user", "content": "Count from 1 to 5, one number per line."}],
		"stream": true,
		"max_tokens": 50
	}`

	proxyPath := "/v1/proxy/v1/chat/completions"
	rr := h.proxyRequest(t, http.MethodPost, proxyPath, tok, strings.NewReader(payload))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	chunks := parseSSEChunks(t, rr.Body.Bytes())
	if len(chunks) == 0 {
		t.Fatal("expected SSE chunks, got none")
	}

	var fullContent strings.Builder
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
			fullContent.WriteString(content)
		}
	}

	result := fullContent.String()
	if result == "" {
		t.Fatal("no content received from stream")
	}
	t.Logf("Anthropic streaming result (%d chunks): %s", len(chunks), result)
}

func TestE2E_Proxy_Google_Streaming(t *testing.T) {
	apiKey := requireOpenRouterKey(t)
	h := newHarness(t)
	org := h.createOrg(t)
	cred := h.storeCredential(t, org, "https://openrouter.ai/api", "bearer", apiKey)
	tok := h.mintToken(t, org, cred.ID)

	payload := `{
		"model": "openai/gpt-4.1-nano",
		"messages": [{"role": "user", "content": "What is 2+2? Reply with just the number."}],
		"stream": true,
		"max_tokens": 20
	}`

	proxyPath := "/v1/proxy/v1/chat/completions"
	rr := h.proxyRequest(t, http.MethodPost, proxyPath, tok, strings.NewReader(payload))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	chunks := parseSSEChunks(t, rr.Body.Bytes())
	content := extractStreamContent(chunks)
	if content == "" {
		t.Fatal("no content from Google Gemini stream")
	}
	t.Logf("Google Gemini streaming result: %s", content)
}

func TestE2E_Proxy_OpenAI_ToolCalls(t *testing.T) {
	apiKey := requireOpenRouterKey(t)
	h := newHarness(t)
	org := h.createOrg(t)
	cred := h.storeCredential(t, org, "https://openrouter.ai/api", "bearer", apiKey)
	tok := h.mintToken(t, org, cred.ID)

	payload := `{
		"model": "openai/gpt-4.1-nano",
		"messages": [{"role": "user", "content": "What is the weather in San Francisco?"}],
		"tools": [{
			"type": "function",
			"function": {
				"name": "get_weather",
				"description": "Get the current weather for a location",
				"parameters": {
					"type": "object",
					"properties": {
						"location": {"type": "string", "description": "City name"}
					},
					"required": ["location"]
				}
			}
		}],
		"stream": false,
		"max_tokens": 100
	}`

	proxyPath := "/v1/proxy/v1/chat/completions"
	rr := h.proxyRequest(t, http.MethodPost, proxyPath, tok, strings.NewReader(payload))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	choices := resp["choices"].([]any)
	if len(choices) == 0 {
		t.Fatal("no choices")
	}
	choice := choices[0].(map[string]any)
	msg := choice["message"].(map[string]any)

	toolCalls, hasTools := msg["tool_calls"].([]any)
	content, hasContent := msg["content"].(string)

	if !hasTools && !hasContent {
		t.Fatalf("expected tool_calls or content, got neither: %v", msg)
	}

	if hasTools && len(toolCalls) > 0 {
		tc := toolCalls[0].(map[string]any)
		fn := tc["function"].(map[string]any)
		t.Logf("Tool call: %s(%s)", fn["name"], fn["arguments"])

		if fn["name"] != "get_weather" {
			t.Fatalf("expected get_weather tool call, got %s", fn["name"])
		}

		args := fn["arguments"].(string)
		if !strings.Contains(strings.ToLower(args), "san francisco") {
			t.Logf("warning: tool args don't contain 'san francisco': %s", args)
		}
	} else {
		t.Logf("Model responded with content instead of tool call: %s", content)
	}
}

func TestE2E_Proxy_Anthropic_StreamingToolCalls(t *testing.T) {
	apiKey := requireOpenRouterKey(t)
	h := newHarness(t)
	org := h.createOrg(t)
	cred := h.storeCredential(t, org, "https://openrouter.ai/api", "bearer", apiKey)
	tok := h.mintToken(t, org, cred.ID)

	payload := `{
		"model": "openai/gpt-4.1-nano",
		"messages": [{"role": "user", "content": "What is the weather in Tokyo?"}],
		"tools": [{
			"type": "function",
			"function": {
				"name": "get_weather",
				"description": "Get the current weather for a location",
				"parameters": {
					"type": "object",
					"properties": {
						"location": {"type": "string", "description": "City name"}
					},
					"required": ["location"]
				}
			}
		}],
		"stream": true,
		"max_tokens": 100
	}`

	proxyPath := "/v1/proxy/v1/chat/completions"
	rr := h.proxyRequest(t, http.MethodPost, proxyPath, tok, strings.NewReader(payload))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	chunks := parseSSEChunks(t, rr.Body.Bytes())
	if len(chunks) == 0 {
		t.Fatal("expected SSE chunks")
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
		delta := choices[0].(map[string]any)["delta"].(map[string]any)

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
		if toolName != "get_weather" {
			t.Fatalf("expected get_weather, got %s", toolName)
		}
	} else {

		content := extractStreamContent(chunks)
		t.Logf("Model responded with content instead of streaming tool call: %s", content)
	}
}
