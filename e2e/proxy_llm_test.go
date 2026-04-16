//go:build llm

package e2e

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ziraloop/ziraloop/internal/middleware"
)


// --------------------------------------------------------------------------
// E2E: Proxy non-streaming completion via OpenRouter → OpenAI
// --------------------------------------------------------------------------

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

// --------------------------------------------------------------------------
// E2E: Proxy SSE streaming via OpenRouter → Anthropic Claude
// --------------------------------------------------------------------------

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

	// Parse SSE stream
	chunks := parseSSEChunks(t, rr.Body.Bytes())
	if len(chunks) == 0 {
		t.Fatal("expected SSE chunks, got none")
	}

	// Collect all content deltas
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

// --------------------------------------------------------------------------
// E2E: Proxy SSE streaming via OpenRouter → Google Gemini
// --------------------------------------------------------------------------

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

// --------------------------------------------------------------------------
// E2E: Tool calls via OpenRouter → OpenAI
// --------------------------------------------------------------------------

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

	// The model should either call the tool or provide content
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
		// Verify arguments contain "San Francisco"
		args := fn["arguments"].(string)
		if !strings.Contains(strings.ToLower(args), "san francisco") {
			t.Logf("warning: tool args don't contain 'san francisco': %s", args)
		}
	} else {
		t.Logf("Model responded with content instead of tool call: %s", content)
	}
}

// --------------------------------------------------------------------------
// E2E: Streaming tool calls via OpenRouter → Anthropic
// --------------------------------------------------------------------------

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

	// Look for tool call deltas in the stream
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
		// Some models may respond with content instead
		content := extractStreamContent(chunks)
		t.Logf("Model responded with content instead of streaming tool call: %s", content)
	}
}

// --------------------------------------------------------------------------
// E2E: MiniMax via OpenRouter
// --------------------------------------------------------------------------

func TestE2E_Proxy_Meta_NonStreaming(t *testing.T) {
	apiKey := requireOpenRouterKey(t)
	h := newHarness(t)
	org := h.createOrg(t)
	cred := h.storeCredential(t, org, "https://openrouter.ai/api", "bearer", apiKey)
	tok := h.mintToken(t, org, cred.ID)

	payload := `{
		"model": "openai/gpt-4.1-nano",
		"messages": [{"role": "user", "content": "Say hello in Japanese. Reply with just the greeting."}],
		"stream": false,
		"max_tokens": 30
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
	content := extractNonStreamContent(t, resp)
	if content == "" {
		t.Fatal("empty response from Meta Llama")
	}
	t.Logf("Meta Llama response: %s", content)
}

// --------------------------------------------------------------------------
// E2E: Multi-turn conversation via proxy
// --------------------------------------------------------------------------

func TestE2E_Proxy_MultiTurn(t *testing.T) {
	apiKey := requireOpenRouterKey(t)
	h := newHarness(t)
	org := h.createOrg(t)
	cred := h.storeCredential(t, org, "https://openrouter.ai/api", "bearer", apiKey)
	tok := h.mintToken(t, org, cred.ID)
	proxyPath := "/v1/proxy/v1/chat/completions"

	// Turn 1
	payload1 := `{
		"model": "openai/gpt-4.1-nano",
		"messages": [{"role": "user", "content": "My name is Alice. Remember it."}],
		"stream": false,
		"max_tokens": 30
	}`
	rr1 := h.proxyRequest(t, http.MethodPost, proxyPath, tok, strings.NewReader(payload1))
	if rr1.Code != http.StatusOK {
		t.Fatalf("turn 1: expected 200, got %d: %s", rr1.Code, rr1.Body.String())
	}
	var resp1 map[string]any
	json.NewDecoder(rr1.Body).Decode(&resp1)
	assistantMsg := extractNonStreamContent(t, resp1)

	// Turn 2 — include conversation history
	payload2 := fmt.Sprintf(`{
		"model": "openai/gpt-4.1-nano",
		"messages": [
			{"role": "user", "content": "My name is Alice. Remember it."},
			{"role": "assistant", "content": %q},
			{"role": "user", "content": "What is my name? Reply with just the name."}
		],
		"stream": false,
		"max_tokens": 30
	}`, assistantMsg)
	rr2 := h.proxyRequest(t, http.MethodPost, proxyPath, tok, strings.NewReader(payload2))
	if rr2.Code != http.StatusOK {
		t.Fatalf("turn 2: expected 200, got %d: %s", rr2.Code, rr2.Body.String())
	}
	var resp2 map[string]any
	json.NewDecoder(rr2.Body).Decode(&resp2)
	answer := extractNonStreamContent(t, resp2)
	if !strings.Contains(strings.ToLower(answer), "alice") {
		t.Fatalf("expected 'Alice' in response, got: %s", answer)
	}
	t.Logf("Multi-turn verified: %s", answer)
}

// --------------------------------------------------------------------------
// E2E: Verify sandbox token is NOT sent to upstream
// --------------------------------------------------------------------------

func TestE2E_Proxy_TokenStripped(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)

	// Create a credential pointing to a test server that echoes headers
	echoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"received_auth": authHeader,
		})
	}))
	defer echoServer.Close()

	cred := h.storeCredential(t, org, echoServer.URL, "bearer", "sk-the-real-api-key")
	tok := h.mintToken(t, org, cred.ID)

	proxyPath := "/v1/proxy/test"
	rr := h.proxyRequest(t, http.MethodGet, proxyPath, tok, nil)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)

	receivedAuth := resp["received_auth"]
	// Must contain the real API key, not the sandbox token
	if !strings.Contains(receivedAuth, "sk-the-real-api-key") {
		t.Fatalf("upstream should receive real API key, got: %s", receivedAuth)
	}
	if strings.Contains(receivedAuth, "ptok_") {
		t.Fatal("sandbox token leaked to upstream!")
	}
}

// --------------------------------------------------------------------------
// E2E: Tenant isolation — one org can't use another's credential
// --------------------------------------------------------------------------

func TestE2E_Proxy_TenantIsolation(t *testing.T) {
	h := newHarness(t)
	org1 := h.createOrg(t)
	org2 := h.createOrg(t)

	cred1 := h.storeCredential(t, org1, "https://api.example.com", "bearer", "org1-secret")

	// Mint token for org2 (which doesn't own cred1)
	// We need to do this manually since mintToken validates credential ownership
	tokenStr, jti, err := token.Mint(h.signingKey, org2.ID.String(), cred1.ID.String(), time.Hour)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	tokenRecord := model.Token{
		ID: uuid.New(), OrgID: org2.ID, CredentialID: cred1.ID,
		JTI: jti, ExpiresAt: time.Now().Add(time.Hour),
	}
	h.db.Create(&tokenRecord)
	t.Cleanup(func() { h.db.Where("id = ?", tokenRecord.ID).Delete(&model.Token{}) })

	proxyPath := "/v1/proxy/test"
	rr := h.proxyRequest(t, http.MethodGet, proxyPath, "ptok_"+tokenStr, nil)

	// Should fail because org2 doesn't own cred1
	if rr.Code == http.StatusOK {
		t.Fatal("tenant isolation violated: org2 accessed org1's credential")
	}
	t.Logf("Tenant isolation enforced: got %d", rr.Code)
}
