//go:build llm

package e2e

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestE2E_Fireworks_KimiK2_MultiTurn(t *testing.T) {
	apiKey := requireFireworksKey(t)
	h, tok, proxyPath := fireworksSetup(t, apiKey)

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
	_ = json.NewDecoder(rr1.Body).Decode(&resp1)
	assistant1 := extractNonStreamContent(t, resp1)

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
	_ = json.NewDecoder(rr2.Body).Decode(&resp2)
	answer := extractNonStreamContent(t, resp2)

	if !strings.Contains(strings.ToUpper(answer), "GAMMA-42") {
		t.Fatalf("expected GAMMA-42 in response, got: %s", answer)
	}
	t.Logf("Multi-turn verified: %s", answer)
}

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
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	content := extractNonStreamContent(t, resp)
	if content == "" {
		t.Fatal("empty response from DeepSeek")
	}
	if !strings.Contains(strings.ToLower(content), "tokyo") {
		t.Logf("warning: expected 'Tokyo' in response, got: %s", content)
	}
	t.Logf("DeepSeek V3 response: %s", content)
}

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
