package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/ziraloop/ziraloop/internal/mcp/catalog"
	"github.com/ziraloop/ziraloop/internal/middleware"
	"github.com/ziraloop/ziraloop/internal/model"
	"github.com/ziraloop/ziraloop/internal/nango"
	"github.com/ziraloop/ziraloop/internal/token"
)

// --------------------------------------------------------------------------
// Helpers — create integration + connection in the e2e harness
// --------------------------------------------------------------------------

// createNangoIntegration creates an integration via the management API (POST /v1/integrations).
// The integration exists in both our DB and Nango (real round-trip).
// Uses an API_KEY provider by default (no credentials needed).
func (h *testHarness) createNangoIntegration(t *testing.T, org model.Org, displayName string) model.Integration {
	t.Helper()
	var provider string
	for _, p := range h.nangoClient.GetProviders() {
		if p.AuthMode == "API_KEY" {
			if _, ok := h.catalog.GetProvider(p.Name); ok {
				provider = p.Name
				break
			}
		}
	}
	if provider == "" {
		t.Fatal("no API_KEY provider with action definitions found")
	}
	body := fmt.Sprintf(`{"provider":%q,"display_name":%q}`, provider, displayName)
	req := httptest.NewRequest(http.MethodPost, "/v1/integrations", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("createNangoIntegration: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	var integ model.Integration
	if err := h.db.Where("id = ?", resp["id"]).First(&integ).Error; err != nil {
		t.Fatalf("createNangoIntegration: lookup failed: %v", err)
	}
	return integ
}

// createNangoIntegrationForProvider creates an integration for a specific provider via the management API.
// For OAUTH2/OAUTH1/TBA providers, passes dummy credentials.
func (h *testHarness) createNangoIntegrationForProvider(t *testing.T, org model.Org, providerName, displayName string) model.Integration {
	t.Helper()
	provider, found := h.nangoClient.GetProvider(providerName)
	if !found {
		t.Fatalf("provider %q not found", providerName)
	}
	var credsJSON string
	switch provider.AuthMode {
	case "OAUTH2", "OAUTH1", "TBA":
		credsJSON = fmt.Sprintf(`,"credentials":{"type":%q,"client_id":"test-id","client_secret":"test-secret"}`, provider.AuthMode)
	case "APP":
		credsJSON = `,"credentials":{"type":"APP","app_id":"test-app","app_link":"https://example.com/app","private_key":"test-key"}`
	}
	body := fmt.Sprintf(`{"provider":%q,"display_name":%q%s}`, providerName, displayName, credsJSON)
	req := httptest.NewRequest(http.MethodPost, "/v1/integrations", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("createNangoIntegrationForProvider(%s): expected 201, got %d: %s", providerName, rr.Code, rr.Body.String())
	}
	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	var integ model.Integration
	if err := h.db.Where("id = ?", resp["id"]).First(&integ).Error; err != nil {
		t.Fatalf("createNangoIntegrationForProvider: lookup failed: %v", err)
	}
	return integ
}

// createNangoConnection creates a real connection in Nango via nangoClient.CreateConnection(),
// then stores the reference via the management API (POST /v1/integrations/{id}/connections).
// Only works for API_KEY providers.
func (h *testHarness) createNangoConnection(t *testing.T, org model.Org, integ model.Integration) string {
	t.Helper()
	nangoProviderConfigKey := fmt.Sprintf("%s_%s", org.ID.String(), integ.UniqueKey)
	nangoConnID := fmt.Sprintf("test-conn-%s", uuid.New().String()[:8])

	// Create real connection in Nango
	if err := h.nangoClient.CreateConnection(context.Background(), nango.CreateConnectionRequest{
		ProviderConfigKey: nangoProviderConfigKey,
		ConnectionID:      nangoConnID,
		APIKey:            "test-api-key-e2e",
	}); err != nil {
		t.Fatalf("createNangoConnection: Nango create failed: %v", err)
	}

	// Store reference via management API
	body := fmt.Sprintf(`{"nango_connection_id":%q}`, nangoConnID)
	req := httptest.NewRequest(http.MethodPost, "/v1/integrations/"+integ.ID.String()+"/connections", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("createNangoConnection: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	return resp["id"].(string)
}

// createLocalConnection stores a connection reference via the management API.
// The nango_connection_id is NOT validated against Nango — use only for tests
// that exercise local-only logic (scoped tokens, scope hashes).
// For tests that need a real Nango round-trip, use createNangoConnection.
func (h *testHarness) createLocalConnection(t *testing.T, org model.Org, integID uuid.UUID, nangoConnID string) string {
	t.Helper()
	body := fmt.Sprintf(`{"nango_connection_id":%q}`, nangoConnID)
	req := httptest.NewRequest(http.MethodPost, "/v1/integrations/"+integID.String()+"/connections", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("createLocalConnection: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	return resp["id"].(string)
}

// mintScopedToken mints a token with scopes via the API handler, returning the
// raw response recorder so callers can inspect status code and body.
func (h *testHarness) mintScopedToken(t *testing.T, org model.Org, credID uuid.UUID, scopesJSON string) *httptest.ResponseRecorder {
	t.Helper()
	body := fmt.Sprintf(`{"credential_id":%q,"ttl":"1h","scopes":%s}`, credID.String(), scopesJSON)
	req := httptest.NewRequest(http.MethodPost, "/v1/tokens", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

// --------------------------------------------------------------------------
// E2E: Connection CRUD lifecycle (real Nango round-trip)
// --------------------------------------------------------------------------

func TestE2E_Connection_CRUD(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)
	integ := h.createNangoIntegration(t, org, "CRUD Test")

	// 1. Create connection (real Nango connection)
	connID := h.createNangoConnection(t, org, integ)
	if connID == "" {
		t.Fatal("connection ID is empty")
	}

	// 2. Get connection
	req := httptest.NewRequest(http.MethodGet, "/v1/connections/"+connID, nil)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("get connection: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var getResp map[string]any
	json.NewDecoder(rr.Body).Decode(&getResp)
	if getResp["id"] != connID {
		t.Fatalf("get returned wrong id: %v", getResp["id"])
	}
	if getResp["integration_id"] != integ.ID.String() {
		t.Fatalf("wrong integration_id: %v", getResp["integration_id"])
	}

	// Get the nango_connection_id for later verification
	nangoConnID := getResp["nango_connection_id"].(string)

	// 3. List connections for integration
	req = httptest.NewRequest(http.MethodGet, "/v1/integrations/"+integ.ID.String()+"/connections", nil)
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list connections: expected 200, got %d", rr.Code)
	}
	list := decodePaginatedList(t, rr)
	if len(list) != 1 {
		t.Fatalf("expected 1 connection, got %d", len(list))
	}
	if list[0]["id"] != connID {
		t.Fatalf("list returned wrong connection")
	}

	// 4. Revoke connection (should also delete from Nango)
	req = httptest.NewRequest(http.MethodDelete, "/v1/connections/"+connID, nil)
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("revoke: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// 5. Verify revoked connection is not returned by GET
	req = httptest.NewRequest(http.MethodGet, "/v1/connections/"+connID, nil)
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after revoke, got %d", rr.Code)
	}

	// 6. Verify revoked connection not in list
	req = httptest.NewRequest(http.MethodGet, "/v1/integrations/"+integ.ID.String()+"/connections", nil)
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list after revoke: expected 200, got %d", rr.Code)
	}
	list = decodePaginatedList(t, rr)
	if len(list) != 0 {
		t.Fatalf("expected 0 connections after revoke, got %d", len(list))
	}

	// 7. Verify Nango connection is gone
	nangoProviderConfigKey := fmt.Sprintf("%s_%s", org.ID.String(), integ.UniqueKey)
	_, err := h.nangoClient.GetConnection(context.Background(), nangoConnID, nangoProviderConfigKey)
	if err == nil {
		t.Fatal("connection should be gone from Nango after revoke")
	}
}

// --------------------------------------------------------------------------
// E2E: Connection with identity
// --------------------------------------------------------------------------

func TestE2E_Connection_WithIdentity(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)
	integ := h.createNangoIntegration(t, org, "Identity Test")

	// Create identity first
	identBody := `{"external_id":"user-456","meta":{"name":"Test User"}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/identities", strings.NewReader(identBody))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create identity: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var identResp map[string]any
	json.NewDecoder(rr.Body).Decode(&identResp)
	identID := identResp["id"].(string)

	// Create real Nango connection first
	nangoProviderConfigKey := fmt.Sprintf("%s_%s", org.ID.String(), integ.UniqueKey)
	nangoConnID := fmt.Sprintf("test-conn-%s", uuid.New().String()[:8])
	if err := h.nangoClient.CreateConnection(context.Background(), nango.CreateConnectionRequest{
		ProviderConfigKey: nangoProviderConfigKey,
		ConnectionID:      nangoConnID,
		APIKey:            "test-api-key-e2e",
	}); err != nil {
		t.Fatalf("create Nango connection: %v", err)
	}

	// Create connection with identity via management API
	body := fmt.Sprintf(`{"nango_connection_id":%q,"identity_id":%q}`, nangoConnID, identID)
	req = httptest.NewRequest(http.MethodPost, "/v1/integrations/"+integ.ID.String()+"/connections", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create connection with identity: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var connResp map[string]any
	json.NewDecoder(rr.Body).Decode(&connResp)
	if connResp["identity_id"] != identID {
		t.Fatalf("expected identity_id=%s, got %v", identID, connResp["identity_id"])
	}
}

// --------------------------------------------------------------------------
// E2E: Connection tenant isolation
// --------------------------------------------------------------------------

func TestE2E_Connection_TenantIsolation(t *testing.T) {
	h := newHarness(t)
	org1 := h.createOrg(t)
	org2 := h.createOrg(t)

	integ := h.createNangoIntegration(t, org1, "Org1 Isolation")
	connID := h.createNangoConnection(t, org1, integ)

	// org2 should NOT see the connection via GET
	req := httptest.NewRequest(http.MethodGet, "/v1/connections/"+connID, nil)
	req = middleware.WithOrg(req, &org2)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("org2 GET: expected 404, got %d", rr.Code)
	}

	// org2 should NOT be able to revoke it
	req = httptest.NewRequest(http.MethodDelete, "/v1/connections/"+connID, nil)
	req = middleware.WithOrg(req, &org2)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("org2 revoke: expected 404, got %d", rr.Code)
	}
}

// --------------------------------------------------------------------------
// E2E: Connection to deleted integration fails
// --------------------------------------------------------------------------

func TestE2E_Connection_DeletedIntegration(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)
	integ := h.createNangoIntegration(t, org, "Soon Deleted")

	// Soft-delete the integration locally (keep in Nango)
	h.db.Model(&integ).Update("deleted_at", "2026-01-01")

	// Attempt to create connection on deleted integration
	body := `{"nango_connection_id":"nango-deleted"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/integrations/"+integ.ID.String()+"/connections", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for deleted integration, got %d: %s", rr.Code, rr.Body.String())
	}
}

// --------------------------------------------------------------------------
// E2E: Scoped token mint — valid scopes
// --------------------------------------------------------------------------

func TestE2E_ScopedToken_ValidScopes(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)
	cred := h.storeCredential(t, org, "https://api.example.com", "bearer", "sk-fake-key")

	// Create integration (Slack — has curated actions) and local connection
	integ := h.createNangoIntegrationForProvider(t, org, "slack", "Slack Scoped")
	connID := h.createLocalConnection(t, org, integ.ID, "nango-scoped-1")

	// Mint token with valid scopes
	scopesJSON := fmt.Sprintf(`[{"connection_id":%q,"actions":["conversations_list","conversations_history"],"resources":{"channel":["C123","C456"]}}]`, connID)
	rr := h.mintScopedToken(t, org, cred.ID, scopesJSON)
	if rr.Code != http.StatusCreated {
		t.Fatalf("mint scoped token: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	tok := resp["token"].(string)
	if !strings.HasPrefix(tok, "ptok_") {
		t.Fatalf("expected ptok_ prefix, got %s", tok[:10])
	}

	// Verify JWT has scope_hash
	jwtStr := strings.TrimPrefix(tok, "ptok_")
	claims, err := token.Validate(h.signingKey, jwtStr)
	if err != nil {
		t.Fatalf("validate token: %v", err)
	}
	if claims.ScopeHash == "" {
		t.Fatal("expected scope_hash in JWT claims")
	}

	// Verify scopes are stored in the token record
	var tokenRecord model.Token
	if err := h.db.Where("jti = ?", claims.ID).First(&tokenRecord).Error; err != nil {
		t.Fatalf("lookup token: %v", err)
	}
	if tokenRecord.Scopes == nil {
		t.Fatal("expected scopes to be stored in token record")
	}
}

// --------------------------------------------------------------------------
// E2E: Scoped token — multiple connections (Slack + GitHub)
// --------------------------------------------------------------------------

func TestE2E_ScopedToken_MultipleConnections(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)
	cred := h.storeCredential(t, org, "https://api.example.com", "bearer", "sk-fake-key")

	// Create Slack integration + local connection
	slackInteg := h.createNangoIntegrationForProvider(t, org, "slack", "Slack Multi")
	slackConnID := h.createLocalConnection(t, org, slackInteg.ID, "nango-slack-multi")

	// Create Notion integration + local connection (OAUTH2, has catalog actions)
	notionInteg := h.createNangoIntegrationForProvider(t, org, "notion", "Notion Multi")
	notionConnID := h.createLocalConnection(t, org, notionInteg.ID, "nango-notion-multi")

	// Mint with both scopes
	scopesJSON := fmt.Sprintf(`[
		{"connection_id":%q,"actions":["conversations_list","chat_post_message"],"resources":{"channel":["C001"]}},
		{"connection_id":%q,"actions":["search","get_page"]}
	]`, slackConnID, notionConnID)

	rr := h.mintScopedToken(t, org, cred.ID, scopesJSON)
	if rr.Code != http.StatusCreated {
		t.Fatalf("mint multi-scope token: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	tok := resp["token"].(string)

	// Validate JWT
	jwtStr := strings.TrimPrefix(tok, "ptok_")
	claims, err := token.Validate(h.signingKey, jwtStr)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if claims.ScopeHash == "" {
		t.Fatal("expected scope_hash for multi-scope token")
	}
}

// --------------------------------------------------------------------------
// E2E: Scoped token — invalid action key returns 400
// --------------------------------------------------------------------------

func TestE2E_ScopedToken_InvalidAction(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)
	cred := h.storeCredential(t, org, "https://api.example.com", "bearer", "sk-fake-key")

	integ := h.createNangoIntegrationForProvider(t, org, "slack", "Slack Invalid Action")
	connID := h.createLocalConnection(t, org, integ.ID, "nango-invalid-action")

	// Mint with nonexistent action
	scopesJSON := fmt.Sprintf(`[{"connection_id":%q,"actions":["nonexistent_action"]}]`, connID)
	rr := h.mintScopedToken(t, org, cred.ID, scopesJSON)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("invalid action: expected 400, got %d: %s", rr.Code, rr.Body.String())
	}

	var errResp map[string]string
	json.NewDecoder(rr.Body).Decode(&errResp)
	if !strings.Contains(errResp["error"], "nonexistent_action") {
		t.Fatalf("error should mention action name, got: %s", errResp["error"])
	}
}

// --------------------------------------------------------------------------
// E2E: Scoped token — wildcard actions rejected
// --------------------------------------------------------------------------

func TestE2E_ScopedToken_WildcardRejected(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)
	cred := h.storeCredential(t, org, "https://api.example.com", "bearer", "sk-fake-key")

	integ := h.createNangoIntegrationForProvider(t, org, "slack", "Slack Wildcard")
	connID := h.createLocalConnection(t, org, integ.ID, "nango-wildcard")

	// Mint with wildcard — must be rejected
	scopesJSON := fmt.Sprintf(`[{"connection_id":%q,"actions":["*"]}]`, connID)
	rr := h.mintScopedToken(t, org, cred.ID, scopesJSON)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("wildcard: expected 400, got %d: %s", rr.Code, rr.Body.String())
	}

	var errResp map[string]string
	json.NewDecoder(rr.Body).Decode(&errResp)
	if !strings.Contains(errResp["error"], "wildcard") {
		t.Fatalf("error should mention wildcard, got: %s", errResp["error"])
	}
}

// --------------------------------------------------------------------------
// E2E: Scoped token — invalid connection_id returns 400
// --------------------------------------------------------------------------

func TestE2E_ScopedToken_InvalidConnection(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)
	cred := h.storeCredential(t, org, "https://api.example.com", "bearer", "sk-fake-key")

	fakeConnID := uuid.New().String()
	scopesJSON := fmt.Sprintf(`[{"connection_id":%q,"actions":["conversations_list"]}]`, fakeConnID)
	rr := h.mintScopedToken(t, org, cred.ID, scopesJSON)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("invalid connection: expected 400, got %d: %s", rr.Code, rr.Body.String())
	}

	var errResp map[string]string
	json.NewDecoder(rr.Body).Decode(&errResp)
	if !strings.Contains(errResp["error"], "not found") {
		t.Fatalf("error should mention not found, got: %s", errResp["error"])
	}
}

// --------------------------------------------------------------------------
// E2E: Scoped token — revoked connection returns 400
// --------------------------------------------------------------------------

func TestE2E_ScopedToken_RevokedConnection(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)
	cred := h.storeCredential(t, org, "https://api.example.com", "bearer", "sk-fake-key")

	integ := h.createNangoIntegration(t, org, "Revoked Conn")
	connID := h.createNangoConnection(t, org, integ)

	// Revoke the connection (also deletes from Nango)
	req := httptest.NewRequest(http.MethodDelete, "/v1/connections/"+connID, nil)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("revoke: expected 200, got %d", rr.Code)
	}

	// Attempt to mint with revoked connection
	scopesJSON := fmt.Sprintf(`[{"connection_id":%q,"actions":["conversations_list"]}]`, connID)
	rr = h.mintScopedToken(t, org, cred.ID, scopesJSON)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("revoked connection: expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

// --------------------------------------------------------------------------
// E2E: Scoped token — connection from another org returns 400
// --------------------------------------------------------------------------

func TestE2E_ScopedToken_CrossOrgConnection(t *testing.T) {
	h := newHarness(t)
	org1 := h.createOrg(t)
	org2 := h.createOrg(t)

	cred := h.storeCredential(t, org2, "https://api.example.com", "bearer", "sk-fake-key")

	// Create connection in org1
	integ := h.createNangoIntegrationForProvider(t, org1, "slack", "Slack Org1")
	connID := h.createLocalConnection(t, org1, integ.ID, "nango-crossorg")

	// Try to mint in org2 using org1's connection
	scopesJSON := fmt.Sprintf(`[{"connection_id":%q,"actions":["conversations_list"]}]`, connID)
	rr := h.mintScopedToken(t, org2, cred.ID, scopesJSON)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("cross-org: expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

// --------------------------------------------------------------------------
// E2E: Scoped token — invalid resource type returns 400
// --------------------------------------------------------------------------

func TestE2E_ScopedToken_InvalidResourceType(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)
	cred := h.storeCredential(t, org, "https://api.example.com", "bearer", "sk-fake-key")

	integ := h.createNangoIntegrationForProvider(t, org, "slack", "Slack Bad Resource")
	connID := h.createLocalConnection(t, org, integ.ID, "nango-bad-resource")

	// conversations_list has resource_type="channel" — providing a "repo" resource is invalid
	scopesJSON := fmt.Sprintf(`[{"connection_id":%q,"actions":["conversations_list"],"resources":{"repo":["org/repo"]}}]`, connID)
	rr := h.mintScopedToken(t, org, cred.ID, scopesJSON)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("invalid resource type: expected 400, got %d: %s", rr.Code, rr.Body.String())
	}

	var errResp map[string]string
	json.NewDecoder(rr.Body).Decode(&errResp)
	if !strings.Contains(errResp["error"], "repo") {
		t.Fatalf("error should mention resource type, got: %s", errResp["error"])
	}
}

// --------------------------------------------------------------------------
// E2E: Scoped token — resource scoping with valid resource type
// --------------------------------------------------------------------------

func TestE2E_ScopedToken_ValidResourceScoping(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)
	cred := h.storeCredential(t, org, "https://api.example.com", "bearer", "sk-fake-key")

	integ := h.createNangoIntegrationForProvider(t, org, "slack", "Slack Resource Scope")
	connID := h.createLocalConnection(t, org, integ.ID, "nango-resource-scope")

	// conversations_history has resource_type="channel" — providing channel resources is valid
	scopesJSON := fmt.Sprintf(`[{"connection_id":%q,"actions":["conversations_history"],"resources":{"channel":["C001","C002"]}}]`, connID)
	rr := h.mintScopedToken(t, org, cred.ID, scopesJSON)
	if rr.Code != http.StatusCreated {
		t.Fatalf("valid resource scoping: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
}

// --------------------------------------------------------------------------
// E2E: Token without scopes — backward compatible
// --------------------------------------------------------------------------

func TestE2E_ScopedToken_BackwardCompatible(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)
	cred := h.storeCredential(t, org, "https://api.example.com", "bearer", "sk-fake-key")

	// Mint without scopes — should work exactly as before
	tok := h.mintToken(t, org, cred.ID)
	if !strings.HasPrefix(tok, "ptok_") {
		t.Fatalf("expected ptok_ prefix, got %s", tok[:10])
	}

	// Verify JWT has NO scope_hash
	jwtStr := strings.TrimPrefix(tok, "ptok_")
	claims, err := token.Validate(h.signingKey, jwtStr)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if claims.ScopeHash != "" {
		t.Fatalf("expected empty scope_hash for unscoped token, got %s", claims.ScopeHash)
	}

	// Verify token record has no scopes (nil or empty)
	var tokenRecord model.Token
	if err := h.db.Where("jti = ?", claims.ID).First(&tokenRecord).Error; err != nil {
		t.Fatalf("lookup: %v", err)
	}
	// Scopes should either be nil or an empty map — no "scopes" key
	if tokenRecord.Scopes != nil {
		if _, hasScopeKey := tokenRecord.Scopes["scopes"]; hasScopeKey {
			t.Fatal("expected no scopes key in token record for unscoped token")
		}
	}
}

// --------------------------------------------------------------------------
// E2E: Scoped token — empty actions array returns 400
// --------------------------------------------------------------------------

func TestE2E_ScopedToken_EmptyActions(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)
	cred := h.storeCredential(t, org, "https://api.example.com", "bearer", "sk-fake-key")

	integ := h.createNangoIntegrationForProvider(t, org, "slack", "Slack Empty Actions")
	connID := h.createLocalConnection(t, org, integ.ID, "nango-empty-actions")

	scopesJSON := fmt.Sprintf(`[{"connection_id":%q,"actions":[]}]`, connID)
	rr := h.mintScopedToken(t, org, cred.ID, scopesJSON)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("empty actions: expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

// --------------------------------------------------------------------------
// E2E: Scoped token — provider with no actions defined returns 400
// --------------------------------------------------------------------------

func TestE2E_ScopedToken_ProviderWithNoActions(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)
	cred := h.storeCredential(t, org, "https://api.example.com", "bearer", "sk-fake-key")

	// asana has no actions defined in the catalog — use API_KEY provider
	integ := h.createNangoIntegration(t, org, "Asana No Actions")
	connID := h.createLocalConnection(t, org, integ.ID, "nango-asana")

	scopesJSON := fmt.Sprintf(`[{"connection_id":%q,"actions":["list_tasks"]}]`, connID)
	rr := h.mintScopedToken(t, org, cred.ID, scopesJSON)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("no actions provider: expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

// --------------------------------------------------------------------------
// E2E: Scoped token — deleted integration's connection returns 400
// --------------------------------------------------------------------------

func TestE2E_ScopedToken_DeletedIntegration(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)
	cred := h.storeCredential(t, org, "https://api.example.com", "bearer", "sk-fake-key")

	integ := h.createNangoIntegration(t, org, "Deleted Integ")
	connID := h.createLocalConnection(t, org, integ.ID, "nango-deleted-integ")

	// Soft-delete the integration
	h.db.Model(&integ).Update("deleted_at", "2026-01-01")

	// Minting with a connection whose integration is deleted should fail
	scopesJSON := fmt.Sprintf(`[{"connection_id":%q,"actions":["conversations_list"]}]`, connID)
	rr := h.mintScopedToken(t, org, cred.ID, scopesJSON)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("deleted integration: expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

// --------------------------------------------------------------------------
// E2E: Actions catalog basic checks
// --------------------------------------------------------------------------

func TestE2E_ActionsCatalog(t *testing.T) {
	cat := catalog.Global()

	// Verify curated providers exist
	for _, provider := range []string{"slack", "github", "notion"} {
		p, ok := cat.GetProvider(provider)
		if !ok {
			t.Fatalf("expected provider %q in catalog", provider)
		}
		if len(p.Actions) == 0 {
			t.Fatalf("expected actions for provider %q", provider)
		}
	}

	// Verify providers with no actions
	for _, provider := range []string{"salesforce"} {
		p, ok := cat.GetProvider(provider)
		if !ok {
			t.Fatalf("expected provider %q in catalog", provider)
		}
		if len(p.Actions) != 0 {
			t.Fatalf("expected no actions for provider %q, got %d", provider, len(p.Actions))
		}
	}

	// Verify providers that now have full action sets
	for _, provider := range []string{"asana", "jira"} {
		p, ok := cat.GetProvider(provider)
		if !ok {
			t.Fatalf("expected provider %q in catalog", provider)
		}
		if len(p.Actions) == 0 {
			t.Fatalf("expected actions for provider %q", provider)
		}
	}

	// Verify specific Slack actions
	for _, action := range []string{"conversations_list", "conversations_history", "chat_post_message"} {
		a, ok := cat.GetAction("slack", action)
		if !ok {
			t.Fatalf("expected action %q for slack", action)
		}
		if a.DisplayName == "" {
			t.Fatalf("expected display_name for slack.%s", action)
		}
	}

	// Verify resource types — all three are channel-scoped
	for _, action := range []string{"conversations_history", "conversations_list", "chat_post_message"} {
		a, _ := cat.GetAction("slack", action)
		if a.ResourceType != "channel" {
			t.Fatalf("expected resource_type=channel for slack.%s, got %q", action, a.ResourceType)
		}
	}

	// Verify ValidateActions rejects wildcard
	err := cat.ValidateActions("slack", []string{"*"})
	if err == nil {
		t.Fatal("expected error for wildcard action")
	}

	// Verify ValidateActions rejects unknown action
	err = cat.ValidateActions("slack", []string{"nonexistent"})
	if err == nil {
		t.Fatal("expected error for unknown action")
	}

	// Verify ValidateActions accepts valid actions
	err = cat.ValidateActions("slack", []string{"conversations_list", "conversations_history"})
	if err != nil {
		t.Fatalf("unexpected error for valid actions: %v", err)
	}

	// Verify ListProviders
	providers := cat.ListProviders()
	if len(providers) == 0 {
		t.Fatal("expected providers in catalog")
	}
}

// --------------------------------------------------------------------------
// E2E: Scoped token — scope hash determinism
// --------------------------------------------------------------------------

func TestE2E_ScopedToken_ScopeHashDeterminism(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)
	cred := h.storeCredential(t, org, "https://api.example.com", "bearer", "sk-fake-key")

	integ := h.createNangoIntegrationForProvider(t, org, "slack", "Slack Determinism")
	connID := h.createLocalConnection(t, org, integ.ID, "nango-determinism")

	scopesJSON := fmt.Sprintf(`[{"connection_id":%q,"actions":["conversations_list","conversations_history"],"resources":{"channel":["C001"]}}]`, connID)

	// Mint twice with identical scopes
	rr1 := h.mintScopedToken(t, org, cred.ID, scopesJSON)
	if rr1.Code != http.StatusCreated {
		t.Fatalf("mint 1: expected 201, got %d: %s", rr1.Code, rr1.Body.String())
	}
	rr2 := h.mintScopedToken(t, org, cred.ID, scopesJSON)
	if rr2.Code != http.StatusCreated {
		t.Fatalf("mint 2: expected 201, got %d: %s", rr2.Code, rr2.Body.String())
	}

	// Extract scope hashes
	var resp1, resp2 map[string]any
	json.NewDecoder(rr1.Body).Decode(&resp1)
	json.NewDecoder(rr2.Body).Decode(&resp2)

	tok1 := strings.TrimPrefix(resp1["token"].(string), "ptok_")
	tok2 := strings.TrimPrefix(resp2["token"].(string), "ptok_")

	claims1, _ := token.Validate(h.signingKey, tok1)
	claims2, _ := token.Validate(h.signingKey, tok2)

	if claims1.ScopeHash != claims2.ScopeHash {
		t.Fatalf("scope hashes differ for identical scopes: %s vs %s", claims1.ScopeHash, claims2.ScopeHash)
	}
}

// --------------------------------------------------------------------------
// E2E: Scoped token — missing connection_id returns 400
// --------------------------------------------------------------------------

func TestE2E_ScopedToken_MissingConnectionID(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)
	cred := h.storeCredential(t, org, "https://api.example.com", "bearer", "sk-fake-key")

	scopesJSON := `[{"connection_id":"","actions":["conversations_list"]}]`
	rr := h.mintScopedToken(t, org, cred.ID, scopesJSON)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("missing connection_id: expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

// --------------------------------------------------------------------------
// E2E: Connection — multiple connections per integration
// --------------------------------------------------------------------------

func TestE2E_Connection_MultiplePerIntegration(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)
	integ := h.createNangoIntegration(t, org, "Multi Conn")

	// Create 3 real Nango connections
	var connIDs []string
	for i := 0; i < 3; i++ {
		connID := h.createNangoConnection(t, org, integ)
		connIDs = append(connIDs, connID)
	}

	// List — should return all 3
	req := httptest.NewRequest(http.MethodGet, "/v1/integrations/"+integ.ID.String()+"/connections", nil)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", rr.Code)
	}
	list := decodePaginatedList(t, rr)
	if len(list) != 3 {
		t.Fatalf("expected 3 connections, got %d", len(list))
	}

	// Revoke one
	req = httptest.NewRequest(http.MethodDelete, "/v1/connections/"+connIDs[0], nil)
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("revoke: expected 200, got %d", rr.Code)
	}

	// List — should return 2
	req = httptest.NewRequest(http.MethodGet, "/v1/integrations/"+integ.ID.String()+"/connections", nil)
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	list = decodePaginatedList(t, rr)
	if len(list) != 2 {
		t.Fatalf("expected 2 connections after revoke, got %d", len(list))
	}
}
