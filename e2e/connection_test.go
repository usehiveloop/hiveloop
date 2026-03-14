package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/llmvault/llmvault/internal/mcp/catalog"
	"github.com/llmvault/llmvault/internal/middleware"
	"github.com/llmvault/llmvault/internal/model"
	"github.com/llmvault/llmvault/internal/token"
)

// --------------------------------------------------------------------------
// Helpers — create integration + connection in the e2e harness
// --------------------------------------------------------------------------

// createIntegration creates a Slack integration (no Nango call in e2e, provider
// validation is skipped when nangoClient is nil).
func (h *testHarness) createIntegration(t *testing.T, org model.Org, provider, displayName string) model.Integration {
	t.Helper()
	integ := model.Integration{
		ID:          uuid.New(),
		OrgID:       org.ID,
		UniqueKey:   fmt.Sprintf("%s-%s", provider, uuid.New().String()[:8]),
		Provider:    provider,
		DisplayName: displayName,
	}
	if err := h.db.Create(&integ).Error; err != nil {
		t.Fatalf("create integration: %v", err)
	}
	return integ
}

// createConnection creates a connection via the API handler.
func (h *testHarness) createConnection(t *testing.T, org model.Org, integID uuid.UUID, nangoConnID string) string {
	t.Helper()
	body := fmt.Sprintf(`{"nango_connection_id":%q}`, nangoConnID)
	req := httptest.NewRequest(http.MethodPost, "/v1/integrations/"+integID.String()+"/connections", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create connection: expected 201, got %d: %s", rr.Code, rr.Body.String())
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
// E2E: Connection CRUD lifecycle
// --------------------------------------------------------------------------

func TestE2E_Connection_CRUD(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)
	integ := h.createIntegration(t, org, "slack", "Slack Test")

	// 1. Create connection
	connID := h.createConnection(t, org, integ.ID, "nango-conn-123")
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
	if getResp["nango_connection_id"] != "nango-conn-123" {
		t.Fatalf("wrong nango_connection_id: %v", getResp["nango_connection_id"])
	}

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

	// 4. Revoke connection
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
}

// --------------------------------------------------------------------------
// E2E: Connection with identity
// --------------------------------------------------------------------------

func TestE2E_Connection_WithIdentity(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)
	integ := h.createIntegration(t, org, "github", "GitHub Test")

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

	// Create connection with identity
	body := fmt.Sprintf(`{"nango_connection_id":"nango-gh-789","identity_id":%q}`, identID)
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

	integ := h.createIntegration(t, org1, "slack", "Org1 Slack")
	connID := h.createConnection(t, org1, integ.ID, "nango-isolated")

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
	integ := h.createIntegration(t, org, "slack", "Slack Soon Deleted")

	// Soft-delete the integration
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

	// Create integration and connection (Slack — has curated actions)
	integ := h.createIntegration(t, org, "slack", "Slack Scoped")
	connID := h.createConnection(t, org, integ.ID, "nango-scoped-1")

	// Mint token with valid scopes
	scopesJSON := fmt.Sprintf(`[{"connection_id":%q,"actions":["list_channels","read_messages"],"resources":{"channel":["C123","C456"]}}]`, connID)
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

	// Create Slack integration + connection
	slackInteg := h.createIntegration(t, org, "slack", "Slack Multi")
	slackConnID := h.createConnection(t, org, slackInteg.ID, "nango-slack-multi")

	// Create GitHub integration + connection
	githubInteg := h.createIntegration(t, org, "github", "GitHub Multi")
	githubConnID := h.createConnection(t, org, githubInteg.ID, "nango-github-multi")

	// Mint with both scopes
	scopesJSON := fmt.Sprintf(`[
		{"connection_id":%q,"actions":["list_channels","send_message"],"resources":{"channel":["C001"]}},
		{"connection_id":%q,"actions":["list_repos","list_issues"],"resources":{"repo":["org/repo-a"]}}
	]`, slackConnID, githubConnID)

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

	integ := h.createIntegration(t, org, "slack", "Slack Invalid Action")
	connID := h.createConnection(t, org, integ.ID, "nango-invalid-action")

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

	integ := h.createIntegration(t, org, "slack", "Slack Wildcard")
	connID := h.createConnection(t, org, integ.ID, "nango-wildcard")

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
	scopesJSON := fmt.Sprintf(`[{"connection_id":%q,"actions":["list_channels"]}]`, fakeConnID)
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

	integ := h.createIntegration(t, org, "slack", "Slack Revoked Conn")
	connID := h.createConnection(t, org, integ.ID, "nango-revoked")

	// Revoke the connection
	req := httptest.NewRequest(http.MethodDelete, "/v1/connections/"+connID, nil)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("revoke: expected 200, got %d", rr.Code)
	}

	// Attempt to mint with revoked connection
	scopesJSON := fmt.Sprintf(`[{"connection_id":%q,"actions":["list_channels"]}]`, connID)
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
	integ := h.createIntegration(t, org1, "slack", "Slack Org1")
	connID := h.createConnection(t, org1, integ.ID, "nango-crossorg")

	// Try to mint in org2 using org1's connection
	scopesJSON := fmt.Sprintf(`[{"connection_id":%q,"actions":["list_channels"]}]`, connID)
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

	integ := h.createIntegration(t, org, "slack", "Slack Bad Resource")
	connID := h.createConnection(t, org, integ.ID, "nango-bad-resource")

	// list_channels has resource_type="" — providing a "repo" resource is invalid
	scopesJSON := fmt.Sprintf(`[{"connection_id":%q,"actions":["list_channels"],"resources":{"repo":["org/repo"]}}]`, connID)
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

	integ := h.createIntegration(t, org, "slack", "Slack Resource Scope")
	connID := h.createConnection(t, org, integ.ID, "nango-resource-scope")

	// read_messages has resource_type="channel" — providing channel resources is valid
	scopesJSON := fmt.Sprintf(`[{"connection_id":%q,"actions":["read_messages"],"resources":{"channel":["C001","C002"]}}]`, connID)
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

	integ := h.createIntegration(t, org, "slack", "Slack Empty Actions")
	connID := h.createConnection(t, org, integ.ID, "nango-empty-actions")

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

	// asana has no actions defined in the catalog
	integ := h.createIntegration(t, org, "asana", "Asana No Actions")
	connID := h.createConnection(t, org, integ.ID, "nango-asana")

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

	integ := h.createIntegration(t, org, "slack", "Slack Deleted Integ")
	connID := h.createConnection(t, org, integ.ID, "nango-deleted-integ")

	// Soft-delete the integration
	h.db.Model(&integ).Update("deleted_at", "2026-01-01")

	// Minting with a connection whose integration is deleted should fail
	scopesJSON := fmt.Sprintf(`[{"connection_id":%q,"actions":["list_channels"]}]`, connID)
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
	for _, provider := range []string{"slack", "github", "google", "notion", "linear"} {
		p, ok := cat.GetProvider(provider)
		if !ok {
			t.Fatalf("expected provider %q in catalog", provider)
		}
		if len(p.Actions) == 0 {
			t.Fatalf("expected actions for provider %q", provider)
		}
	}

	// Verify skeleton providers exist but have no actions
	for _, provider := range []string{"asana", "jira", "salesforce"} {
		p, ok := cat.GetProvider(provider)
		if !ok {
			t.Fatalf("expected skeleton provider %q in catalog", provider)
		}
		if len(p.Actions) != 0 {
			t.Fatalf("expected no actions for skeleton provider %q, got %d", provider, len(p.Actions))
		}
	}

	// Verify specific Slack actions
	for _, action := range []string{"list_channels", "read_messages", "send_message"} {
		a, ok := cat.GetAction("slack", action)
		if !ok {
			t.Fatalf("expected action %q for slack", action)
		}
		if a.DisplayName == "" {
			t.Fatalf("expected display_name for slack.%s", action)
		}
	}

	// Verify resource types
	readMsg, _ := cat.GetAction("slack", "read_messages")
	if readMsg.ResourceType != "channel" {
		t.Fatalf("expected resource_type=channel for slack.read_messages, got %q", readMsg.ResourceType)
	}
	listChannels, _ := cat.GetAction("slack", "list_channels")
	if listChannels.ResourceType != "" {
		t.Fatalf("expected empty resource_type for slack.list_channels, got %q", listChannels.ResourceType)
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
	err = cat.ValidateActions("slack", []string{"list_channels", "read_messages"})
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

	integ := h.createIntegration(t, org, "slack", "Slack Determinism")
	connID := h.createConnection(t, org, integ.ID, "nango-determinism")

	scopesJSON := fmt.Sprintf(`[{"connection_id":%q,"actions":["list_channels","read_messages"],"resources":{"channel":["C001"]}}]`, connID)

	// Mint twice with identical scopes
	rr1 := h.mintScopedToken(t, org, cred.ID, scopesJSON)
	if rr1.Code != http.StatusCreated {
		t.Fatalf("mint 1: expected 201, got %d", rr1.Code)
	}
	rr2 := h.mintScopedToken(t, org, cred.ID, scopesJSON)
	if rr2.Code != http.StatusCreated {
		t.Fatalf("mint 2: expected 201, got %d", rr2.Code)
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

	scopesJSON := `[{"connection_id":"","actions":["list_channels"]}]`
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
	integ := h.createIntegration(t, org, "slack", "Slack Multi Conn")

	// Create 3 connections
	var connIDs []string
	for i := 0; i < 3; i++ {
		connID := h.createConnection(t, org, integ.ID, fmt.Sprintf("nango-multi-%d", i))
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
