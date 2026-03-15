package e2e

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/llmvault/llmvault/internal/middleware"
	"github.com/llmvault/llmvault/internal/model"
)

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

func (h *testHarness) createConnectSession(t *testing.T, org model.Org, body string) (sessionToken string, sessionID string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/connect/sessions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create connect session: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		ID           string `json:"id"`
		SessionToken string `json:"session_token"`
	}
	json.NewDecoder(rr.Body).Decode(&resp)
	return resp.SessionToken, resp.ID
}

func (h *testHarness) connectRequest(t *testing.T, method, path, sessionToken string, body io.Reader) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+sessionToken)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

// --------------------------------------------------------------------------
// E2E: Connect session lifecycle
// --------------------------------------------------------------------------

func TestE2E_ConnectSession_Lifecycle(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)

	body := `{"external_id":"user_123","ttl":"15m"}`
	token, _ := h.createConnectSession(t, org, body)

	// Verify token format
	if !strings.HasPrefix(token, "csess_") {
		t.Fatalf("expected csess_ prefix, got %s", token[:10])
	}
	if len(token) != 70 { // csess_ (6) + 64 hex chars
		t.Fatalf("expected token length 70, got %d", len(token))
	}

	// Use session to get session info
	rr := h.connectRequest(t, http.MethodGet, "/v1/widget/session", token, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("session info: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var info struct {
		ID         string  `json:"id"`
		IdentityID *string `json:"identity_id"`
		ExternalID string  `json:"external_id"`
		ExpiresAt  string  `json:"expires_at"`
	}
	json.NewDecoder(rr.Body).Decode(&info)

	if info.ExternalID != "user_123" {
		t.Errorf("expected external_id user_123, got %s", info.ExternalID)
	}
	if info.IdentityID == nil {
		t.Error("expected identity_id to be set (auto-upserted)")
	}
	if info.ExpiresAt == "" {
		t.Error("expected expires_at to be set")
	}

	t.Logf("Session created: id=%s, identity=%s", info.ID, *info.IdentityID)
}

// --------------------------------------------------------------------------
// E2E: Connect session with explicit identity_id
// --------------------------------------------------------------------------

func TestE2E_ConnectSession_ExplicitIdentity(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)

	// Create identity first
	identReq := httptest.NewRequest(http.MethodPost, "/v1/identities",
		strings.NewReader(`{"external_id":"explicit_user"}`))
	identReq.Header.Set("Content-Type", "application/json")
	identReq = middleware.WithOrg(identReq, &org)
	identRR := httptest.NewRecorder()
	h.router.ServeHTTP(identRR, identReq)
	if identRR.Code != http.StatusCreated {
		t.Fatalf("create identity: %d: %s", identRR.Code, identRR.Body.String())
	}
	var identResp struct {
		ID string `json:"id"`
	}
	json.NewDecoder(identRR.Body).Decode(&identResp)

	// Create session with explicit identity_id
	body := fmt.Sprintf(`{"identity_id":%q}`, identResp.ID)
	token, _ := h.createConnectSession(t, org, body)

	rr := h.connectRequest(t, http.MethodGet, "/v1/widget/session", token, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var info struct {
		IdentityID *string `json:"identity_id"`
	}
	json.NewDecoder(rr.Body).Decode(&info)
	if info.IdentityID == nil || *info.IdentityID != identResp.ID {
		t.Errorf("expected identity_id %s", identResp.ID)
	}
}

// --------------------------------------------------------------------------
// E2E: Connect session expiry
// --------------------------------------------------------------------------

func TestE2E_ConnectSession_Expiry(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)

	token, sessionID := h.createConnectSession(t, org, `{"external_id":"exp_user","ttl":"15m"}`)

	// Manually expire the session in DB
	h.db.Model(&model.ConnectSession{}).Where("id = ?", sessionID).
		Update("expires_at", "2020-01-01 00:00:00")

	rr := h.connectRequest(t, http.MethodGet, "/v1/widget/session", token, nil)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for expired session, got %d", rr.Code)
	}
}

// --------------------------------------------------------------------------
// E2E: Connect session TTL validation
// --------------------------------------------------------------------------

func TestE2E_ConnectSession_TTLValidation(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)

	// TTL > 30m should fail
	req := httptest.NewRequest(http.MethodPost, "/v1/connect/sessions",
		strings.NewReader(`{"external_id":"ttl_user","ttl":"1h"}`))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for TTL > 30m, got %d: %s", rr.Code, rr.Body.String())
	}
}

// --------------------------------------------------------------------------
// E2E: Connect session origin validation
// --------------------------------------------------------------------------

func TestE2E_ConnectSession_OriginValidation(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)

	token, _ := h.createConnectSession(t, org,
		`{"external_id":"origin_user","allowed_origins":["https://app.example.com"]}`)

	// Request with matching origin — should succeed
	req := httptest.NewRequest(http.MethodGet, "/v1/widget/session", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Origin", "https://app.example.com")
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("matching origin: expected 200, got %d", rr.Code)
	}

	// Request with non-matching origin — should be rejected
	req = httptest.NewRequest(http.MethodGet, "/v1/widget/session", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Origin", "https://evil.com")
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("wrong origin: expected 403, got %d", rr.Code)
	}
}

// --------------------------------------------------------------------------
// E2E: Connect provider catalog filtering
// --------------------------------------------------------------------------

func TestE2E_Connect_ProviderFiltering(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)

	token, _ := h.createConnectSession(t, org,
		`{"external_id":"provider_user","allowed_providers":["openai","anthropic"]}`)

	rr := h.connectRequest(t, http.MethodGet, "/v1/widget/providers", token, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var providers []struct {
		ID string `json:"id"`
	}
	json.NewDecoder(rr.Body).Decode(&providers)

	if len(providers) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(providers))
	}

	ids := map[string]bool{}
	for _, p := range providers {
		ids[p.ID] = true
	}
	if !ids["openai"] || !ids["anthropic"] {
		t.Errorf("expected openai and anthropic, got %v", ids)
	}
}

// --------------------------------------------------------------------------
// E2E: Connect provider catalog — no filter returns all
// --------------------------------------------------------------------------

func TestE2E_Connect_ProviderNoFilter(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)

	token, _ := h.createConnectSession(t, org, `{"external_id":"all_providers"}`)

	rr := h.connectRequest(t, http.MethodGet, "/v1/widget/providers", token, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var providers []struct {
		ID string `json:"id"`
	}
	json.NewDecoder(rr.Body).Decode(&providers)

	if len(providers) < 50 {
		t.Fatalf("expected 50+ providers with no filter, got %d", len(providers))
	}
}

// --------------------------------------------------------------------------
// E2E: Connect connection CRUD
// --------------------------------------------------------------------------

func TestE2E_Connect_ConnectionCRUD(t *testing.T) {
	apiKey := requireOpenRouterKey(t)
	h := newHarness(t)
	org := h.createOrg(t)

	token, _ := h.createConnectSession(t, org, `{"external_id":"crud_user"}`)

	// Create connection
	createBody := fmt.Sprintf(`{"provider_id":"openrouter","api_key":%q,"label":"My OpenRouter"}`, apiKey)
	rr := h.connectRequest(t, http.MethodPost, "/v1/widget/connections", token, strings.NewReader(createBody))
	if rr.Code != http.StatusCreated {
		t.Fatalf("create connection: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var createResp struct {
		ID           string `json:"id"`
		Label        string `json:"label"`
		ProviderID   string `json:"provider_id"`
		ProviderName string `json:"provider_name"`
		BaseURL      string `json:"base_url"`
		AuthScheme   string `json:"auth_scheme"`
	}
	json.NewDecoder(rr.Body).Decode(&createResp)

	if createResp.ProviderID != "openrouter" {
		t.Errorf("expected provider_id openrouter, got %s", createResp.ProviderID)
	}
	if createResp.ProviderName == "" {
		t.Error("expected provider_name to be set")
	}
	if createResp.Label != "My OpenRouter" {
		t.Errorf("expected label 'My OpenRouter', got %s", createResp.Label)
	}
	if createResp.BaseURL == "" {
		t.Error("expected base_url to be auto-resolved")
	}
	if createResp.AuthScheme != "bearer" {
		t.Errorf("expected auth_scheme bearer, got %s", createResp.AuthScheme)
	}

	// List connections
	rr = h.connectRequest(t, http.MethodGet, "/v1/widget/connections", token, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", rr.Code)
	}

	var connPage struct {
		Data []struct {
			ID         string `json:"id"`
			ProviderID string `json:"provider_id"`
		} `json:"data"`
		HasMore bool `json:"has_more"`
	}
	json.NewDecoder(rr.Body).Decode(&connPage)
	if len(connPage.Data) != 1 {
		t.Fatalf("expected 1 connection, got %d", len(connPage.Data))
	}
	if connPage.Data[0].ID != createResp.ID {
		t.Error("listed connection ID doesn't match created")
	}

	// Delete connection
	rr = h.connectRequest(t, http.MethodDelete, "/v1/widget/connections/"+createResp.ID, token, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("delete: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify connection is gone
	rr = h.connectRequest(t, http.MethodGet, "/v1/widget/connections", token, nil)
	json.NewDecoder(rr.Body).Decode(&connPage)
	if len(connPage.Data) != 0 {
		t.Fatalf("expected 0 connections after delete, got %d", len(connPage.Data))
	}
}

// --------------------------------------------------------------------------
// E2E: Connect — API key never returned in responses
// --------------------------------------------------------------------------

func TestE2E_Connect_APIKeyNeverReturned(t *testing.T) {
	apiKey := requireOpenRouterKey(t)
	h := newHarness(t)
	org := h.createOrg(t)

	token, _ := h.createConnectSession(t, org, `{"external_id":"secret_user"}`)

	// Create connection
	body := fmt.Sprintf(`{"provider_id":"openrouter","api_key":%q}`, apiKey)
	rr := h.connectRequest(t, http.MethodPost, "/v1/widget/connections", token,
		strings.NewReader(body))
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	// Check create response doesn't contain the key
	respBody := rr.Body.String()
	if strings.Contains(respBody, apiKey) {
		t.Fatal("API key leaked in create response!")
	}

	// Check list response doesn't contain the key
	rr = h.connectRequest(t, http.MethodGet, "/v1/widget/connections", token, nil)
	respBody = rr.Body.String()
	if strings.Contains(respBody, apiKey) {
		t.Fatal("API key leaked in list response!")
	}
}

// --------------------------------------------------------------------------
// E2E: Connect — provider auto-resolution
// --------------------------------------------------------------------------

func TestE2E_Connect_ProviderAutoResolution(t *testing.T) {
	apiKey := requireOpenRouterKey(t)
	h := newHarness(t)
	org := h.createOrg(t)

	token, _ := h.createConnectSession(t, org, `{"external_id":"auto_user"}`)

	// Test auto-resolution with openrouter (logic is provider-agnostic)
	body := fmt.Sprintf(`{"provider_id":"openrouter","api_key":%q}`, apiKey)
	rr := h.connectRequest(t, http.MethodPost, "/v1/widget/connections", token,
		strings.NewReader(body))
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		BaseURL    string `json:"base_url"`
		AuthScheme string `json:"auth_scheme"`
	}
	json.NewDecoder(rr.Body).Decode(&resp)

	if !strings.Contains(resp.BaseURL, "openrouter.ai") {
		t.Errorf("base_url: got %q, want substring %q", resp.BaseURL, "openrouter.ai")
	}
	if resp.AuthScheme != "bearer" {
		t.Errorf("auth_scheme: got %q, want %q", resp.AuthScheme, "bearer")
	}
}

// --------------------------------------------------------------------------
// E2E: Connect — org isolation
// --------------------------------------------------------------------------

func TestE2E_Connect_OrgIsolation(t *testing.T) {
	apiKey := requireOpenRouterKey(t)
	h := newHarness(t)
	org1 := h.createOrg(t)
	org2 := h.createOrg(t)

	// Create connection via org1's session
	token1, _ := h.createConnectSession(t, org1, `{"external_id":"iso_user1"}`)
	body := fmt.Sprintf(`{"provider_id":"openrouter","api_key":%q}`, apiKey)
	rr := h.connectRequest(t, http.MethodPost, "/v1/widget/connections", token1,
		strings.NewReader(body))
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", rr.Code)
	}

	// org2's session should not see org1's connections
	token2, _ := h.createConnectSession(t, org2, `{"external_id":"iso_user2"}`)
	rr = h.connectRequest(t, http.MethodGet, "/v1/widget/connections", token2, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", rr.Code)
	}

	var connPage struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	json.NewDecoder(rr.Body).Decode(&connPage)
	if len(connPage.Data) != 0 {
		t.Fatalf("org2 should see 0 connections, got %d", len(connPage.Data))
	}
}

// --------------------------------------------------------------------------
// E2E: Connect — invalid session token rejected
// --------------------------------------------------------------------------

func TestE2E_Connect_InvalidToken(t *testing.T) {
	h := newHarness(t)

	rr := h.connectRequest(t, http.MethodGet, "/v1/widget/session", "csess_invalid_token", nil)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

// --------------------------------------------------------------------------
// E2E: Connect — non-csess token rejected
// --------------------------------------------------------------------------

func TestE2E_Connect_WrongTokenFormat(t *testing.T) {
	h := newHarness(t)

	rr := h.connectRequest(t, http.MethodGet, "/v1/widget/session", "ptok_some_jwt_token", nil)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

// --------------------------------------------------------------------------
// E2E: Connect — permission enforcement
// --------------------------------------------------------------------------

func TestE2E_Connect_PermissionEnforcement(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)

	// Create session with only "list" permission
	token, _ := h.createConnectSession(t, org,
		`{"external_id":"perm_user","permissions":["list"]}`)

	// List should work
	rr := h.connectRequest(t, http.MethodGet, "/v1/widget/connections", token, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("list with permission: expected 200, got %d", rr.Code)
	}

	// Create should be forbidden
	rr = h.connectRequest(t, http.MethodPost, "/v1/widget/connections", token,
		strings.NewReader(`{"provider_id":"openai","api_key":"sk-test"}`))
	if rr.Code != http.StatusForbidden {
		t.Fatalf("create without permission: expected 403, got %d", rr.Code)
	}
}

// --------------------------------------------------------------------------
// E2E: Connect — no permission field means all permissions
// --------------------------------------------------------------------------

func TestE2E_Connect_NoPermissionsMeansAll(t *testing.T) {
	apiKey := requireOpenRouterKey(t)
	h := newHarness(t)
	org := h.createOrg(t)

	// No permissions field = all allowed
	token, _ := h.createConnectSession(t, org, `{"external_id":"all_perm_user"}`)

	// Create should work
	body := fmt.Sprintf(`{"provider_id":"openrouter","api_key":%q}`, apiKey)
	rr := h.connectRequest(t, http.MethodPost, "/v1/widget/connections", token,
		strings.NewReader(body))
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
}

// --------------------------------------------------------------------------
// E2E: Connect — disallowed provider rejected
// --------------------------------------------------------------------------

func TestE2E_Connect_DisallowedProvider(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)

	// Only allow openai
	token, _ := h.createConnectSession(t, org,
		`{"external_id":"restricted_user","allowed_providers":["openai"]}`)

	// Try to create anthropic connection
	rr := h.connectRequest(t, http.MethodPost, "/v1/widget/connections", token,
		strings.NewReader(`{"provider_id":"anthropic","api_key":"sk-test"}`))
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for disallowed provider, got %d: %s", rr.Code, rr.Body.String())
	}
}

// --------------------------------------------------------------------------
// E2E: Connect — session without identity can't create connections
// --------------------------------------------------------------------------

func TestE2E_Connect_NoIdentity(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)

	// Creating a session without identity_id or external_id should fail
	req := httptest.NewRequest(http.MethodPost, "/v1/connect/sessions", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing identity, got %d: %s", rr.Code, rr.Body.String())
	}
}

// --------------------------------------------------------------------------
// E2E: Connect — settings CRUD
// --------------------------------------------------------------------------

func TestE2E_Connect_SettingsCRUD(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)

	// GET initial settings (empty)
	req := httptest.NewRequest(http.MethodGet, "/v1/settings/connect", nil)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("get settings: expected 200, got %d", rr.Code)
	}

	var settings struct {
		AllowedOrigins []string `json:"allowed_origins"`
	}
	json.NewDecoder(rr.Body).Decode(&settings)
	if len(settings.AllowedOrigins) != 0 {
		t.Fatalf("expected empty allowed_origins, got %v", settings.AllowedOrigins)
	}

	// PUT new settings
	req = httptest.NewRequest(http.MethodPut, "/v1/settings/connect",
		strings.NewReader(`{"allowed_origins":["https://app.example.com","http://localhost:3000"]}`))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("put settings: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// GET updated settings
	req = httptest.NewRequest(http.MethodGet, "/v1/settings/connect", nil)
	// Need to reload org from DB since settings update modifies the org
	var updatedOrg model.Org
	h.db.Where("id = ?", org.ID).First(&updatedOrg)
	req = middleware.WithOrg(req, &updatedOrg)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	json.NewDecoder(rr.Body).Decode(&settings)
	if len(settings.AllowedOrigins) != 2 {
		t.Fatalf("expected 2 allowed_origins, got %d", len(settings.AllowedOrigins))
	}
}

// --------------------------------------------------------------------------
// E2E: Connect — settings origin validation enforced on session creation
// --------------------------------------------------------------------------

func TestE2E_Connect_SettingsEnforcedOnSession(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)

	// Set org allowed origins
	req := httptest.NewRequest(http.MethodPut, "/v1/settings/connect",
		strings.NewReader(`{"allowed_origins":["https://allowed.com"]}`))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("set settings: %d", rr.Code)
	}

	// Reload org
	h.db.Where("id = ?", org.ID).First(&org)

	// Creating session with allowed origin should work
	req = httptest.NewRequest(http.MethodPost, "/v1/connect/sessions",
		strings.NewReader(`{"external_id":"enforced_user","allowed_origins":["https://allowed.com"]}`))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	// Creating session with non-allowed origin should fail
	req = httptest.NewRequest(http.MethodPost, "/v1/connect/sessions",
		strings.NewReader(`{"external_id":"enforced_user2","allowed_origins":["https://not-allowed.com"]}`))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for non-allowed origin, got %d: %s", rr.Code, rr.Body.String())
	}
}

// --------------------------------------------------------------------------
// E2E: Connect — verify connection with real OpenRouter key
// --------------------------------------------------------------------------

func TestE2E_Connect_VerifyConnection_Live(t *testing.T) {
	apiKey := requireOpenRouterKey(t)
	h := newHarness(t)
	org := h.createOrg(t)

	token, _ := h.createConnectSession(t, org,
		`{"external_id":"verify_user","permissions":["create","list","verify"]}`)

	// Create connection with real key
	body := fmt.Sprintf(`{"provider_id":"openrouter","api_key":%q,"label":"Real OR Key"}`, apiKey)
	rr := h.connectRequest(t, http.MethodPost, "/v1/widget/connections", token,
		strings.NewReader(body))
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var createResp struct {
		ID string `json:"id"`
	}
	json.NewDecoder(rr.Body).Decode(&createResp)

	// Verify connection
	rr = h.connectRequest(t, http.MethodPost,
		"/v1/widget/connections/"+createResp.ID+"/verify", token, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("verify: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var verifyResp struct {
		Valid bool   `json:"valid"`
		Error string `json:"error"`
	}
	json.NewDecoder(rr.Body).Decode(&verifyResp)

	if !verifyResp.Valid {
		t.Fatalf("expected valid=true, got error: %s", verifyResp.Error)
	}
	t.Log("Connection verified successfully with real OpenRouter key")
}

// --------------------------------------------------------------------------
// E2E: Connect — invalid key rejected at creation
// --------------------------------------------------------------------------

func TestE2E_Connect_CreateConnection_InvalidKey_Rejected(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)

	token, _ := h.createConnectSession(t, org,
		`{"external_id":"bad_key_user","permissions":["create","list"]}`)

	// Create connection with fake key — should be rejected with 422
	rr := h.connectRequest(t, http.MethodPost, "/v1/widget/connections", token,
		strings.NewReader(`{"provider_id":"openai","api_key":"sk-invalid-key-12345"}`))
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", rr.Code, rr.Body.String())
	}

	var errResp struct {
		Error string `json:"error"`
	}
	json.NewDecoder(rr.Body).Decode(&errResp)

	if !strings.Contains(errResp.Error, "api key verification failed") {
		t.Fatalf("expected error to contain 'api key verification failed', got: %s", errResp.Error)
	}
	t.Logf("Correctly rejected invalid key at creation: %s", errResp.Error)
}

// --------------------------------------------------------------------------
// E2E: Connect — unknown provider rejected
// --------------------------------------------------------------------------

func TestE2E_Connect_UnknownProvider(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)

	token, _ := h.createConnectSession(t, org, `{"external_id":"unknown_provider_user"}`)

	rr := h.connectRequest(t, http.MethodPost, "/v1/widget/connections", token,
		strings.NewReader(`{"provider_id":"totally-fake-provider","api_key":"sk-test"}`))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown provider, got %d", rr.Code)
	}
}

// --------------------------------------------------------------------------
// E2E: Connect — default label from provider name
// --------------------------------------------------------------------------

func TestE2E_Connect_DefaultLabel(t *testing.T) {
	apiKey := requireOpenRouterKey(t)
	h := newHarness(t)
	org := h.createOrg(t)

	token, _ := h.createConnectSession(t, org, `{"external_id":"label_user"}`)

	// Create without label — should default to provider name
	body := fmt.Sprintf(`{"provider_id":"openrouter","api_key":%q}`, apiKey)
	rr := h.connectRequest(t, http.MethodPost, "/v1/widget/connections", token,
		strings.NewReader(body))
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rr.Code)
	}

	var resp struct {
		Label string `json:"label"`
	}
	json.NewDecoder(rr.Body).Decode(&resp)

	if resp.Label == "" {
		t.Error("expected default label from provider name")
	}
	t.Logf("Default label: %s", resp.Label)
}

// --------------------------------------------------------------------------
// E2E: Connect — delete non-existent connection returns 404
// --------------------------------------------------------------------------

func TestE2E_Connect_DeleteNotFound(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)

	token, _ := h.createConnectSession(t, org, `{"external_id":"del_user"}`)

	rr := h.connectRequest(t, http.MethodDelete,
		"/v1/widget/connections/00000000-0000-0000-0000-000000000000", token, nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

// --------------------------------------------------------------------------
// E2E: Connect — session activation tracking
// --------------------------------------------------------------------------

func TestE2E_Connect_SessionActivation(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)

	_, sessionID := h.createConnectSession(t, org, `{"external_id":"activate_user"}`)

	// Before any use, activated_at should be nil
	var sess model.ConnectSession
	h.db.Where("id = ?", sessionID).First(&sess)
	if sess.ActivatedAt != nil {
		t.Fatal("expected activated_at to be nil before first use")
	}

	// Use the session
	token := sess.SessionToken
	rr := h.connectRequest(t, http.MethodGet, "/v1/widget/session", token, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	// After use, activated_at should be set
	h.db.Where("id = ?", sessionID).First(&sess)
	if sess.ActivatedAt == nil {
		t.Fatal("expected activated_at to be set after first use")
	}
}
