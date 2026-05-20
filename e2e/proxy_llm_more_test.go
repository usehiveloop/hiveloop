//go:build llm

package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/token"
)

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

func TestE2E_Proxy_MultiTurn(t *testing.T) {
	apiKey := requireOpenRouterKey(t)
	h := newHarness(t)
	org := h.createOrg(t)
	cred := h.storeCredential(t, org, "https://openrouter.ai/api", "bearer", apiKey)
	tok := h.mintToken(t, org, cred.ID)
	proxyPath := "/v1/proxy/v1/chat/completions"

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
	_ = json.NewDecoder(rr1.Body).Decode(&resp1)
	assistantMsg := extractNonStreamContent(t, resp1)

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
	_ = json.NewDecoder(rr2.Body).Decode(&resp2)
	answer := extractNonStreamContent(t, resp2)
	if !strings.Contains(strings.ToLower(answer), "alice") {
		t.Fatalf("expected 'Alice' in response, got: %s", answer)
	}
	t.Logf("Multi-turn verified: %s", answer)
}

func TestE2E_Proxy_TokenStripped(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)

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
	_ = json.NewDecoder(rr.Body).Decode(&resp)

	receivedAuth := resp["received_auth"]

	if !strings.Contains(receivedAuth, "sk-the-real-api-key") {
		t.Fatalf("upstream should receive real API key, got: %s", receivedAuth)
	}
	if strings.Contains(receivedAuth, "ptok_") {
		t.Fatal("sandbox token leaked to upstream!")
	}
}

func TestE2E_Proxy_TenantIsolation(t *testing.T) {
	h := newHarness(t)
	org1 := h.createOrg(t)
	org2 := h.createOrg(t)

	cred1 := h.storeCredential(t, org1, "https://api.example.com", "bearer", "org1-secret")

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

	if rr.Code == http.StatusOK {
		t.Fatal("tenant isolation violated: org2 accessed org1's credential")
	}
	t.Logf("Tenant isolation enforced: got %d", rr.Code)
}
