package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/google/uuid"
	"github.com/llmvault/llmvault/internal/counter"
	"github.com/llmvault/llmvault/internal/handler"
	"github.com/llmvault/llmvault/internal/mcp/catalog"
	"github.com/llmvault/llmvault/internal/middleware"
	"github.com/llmvault/llmvault/internal/model"
	"github.com/llmvault/llmvault/internal/token"
)

// mcpTestHarness extends testHarness with MCP server infrastructure.
type mcpTestHarness struct {
	*testHarness
	mcpHandler *handler.MCPHandler
	mcpRouter  *chi.Mux
}

// newMCPHarness creates a full test harness with MCP server routing.
func newMCPHarness(t *testing.T) *mcpTestHarness {
	t.Helper()
	h := newHarness(t)
	actionsCatalog := catalog.Global()

	// Counter is optional — pass nil to disable request cap checking in MCP tests.
	var ctr *counter.Counter

	mcpH := handler.NewMCPHandler(h.db, h.signingKey, actionsCatalog, h.nangoClient, ctr)

	// Re-create token handler WITH mcpBaseURL + serverCache so mcp_endpoint is returned.
	// Registered as /v1/mcp-tokens so it doesn't conflict with the harness's /v1/tokens.
	tokenHandler := handler.NewTokenHandler(h.db, h.signingKey, h.cacheManager, ctr, actionsCatalog, "http://mcp.test", mcpH.ServerCache)
	h.router.Route("/v1/mcp-tokens", func(r chi.Router) {
		r.Post("/", tokenHandler.Mint)
		r.Delete("/{jti}", tokenHandler.Revoke)
	})

	// Register available-scopes on the harness router
	h.router.Get("/v1/connections/available-scopes", handler.NewConnectionHandler(h.db, h.nangoClient, actionsCatalog).AvailableScopes)

	// Build MCP chi router (same structure as main.go)
	mcpRouter := chi.NewRouter()
	mcpRouter.Route("/{jti}", func(r chi.Router) {
		r.Use(middleware.TokenAuth(h.signingKey, h.db))
		r.Use(mcpH.ValidateJTIMatch)
		r.Use(mcpH.ValidateHasScopes)
		r.Handle("/*", mcpH.StreamableHTTPHandler())
	})
	mcpRouter.Route("/sse/{jti}", func(r chi.Router) {
		r.Use(middleware.TokenAuth(h.signingKey, h.db))
		r.Use(mcpH.ValidateJTIMatch)
		r.Use(mcpH.ValidateHasScopes)
		r.Handle("/*", mcpH.SSEHandler())
	})

	return &mcpTestHarness{
		testHarness: h,
		mcpHandler:  mcpH,
		mcpRouter:   mcpRouter,
	}
}

// mintScopedTokenWithEndpoint mints a token via the /v1/mcp-tokens endpoint
// (which has mcpBaseURL configured) and returns the full response.
func (h *mcpTestHarness) mintScopedTokenWithEndpoint(t *testing.T, org model.Org, credID uuid.UUID, scopesJSON string) (tokenStr, jti, mcpEndpoint string) {
	t.Helper()
	body := fmt.Sprintf(`{"credential_id":%q,"ttl":"1h","scopes":%s}`, credID.String(), scopesJSON)
	req := httptest.NewRequest(http.MethodPost, "/v1/mcp-tokens/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("mintScopedTokenWithEndpoint: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Token       string  `json:"token"`
		JTI         string  `json:"jti"`
		MCPEndpoint *string `json:"mcp_endpoint"`
	}
	json.NewDecoder(rr.Body).Decode(&resp)
	ep := ""
	if resp.MCPEndpoint != nil {
		ep = *resp.MCPEndpoint
	}
	return resp.Token, resp.JTI, ep
}

// authRoundTripper injects an Authorization header on every request.
type authRoundTripper struct {
	token string
	base  http.RoundTripper
}

func (a *authRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+a.token)
	return a.base.RoundTrip(req)
}

// httpClientWithAuth returns an http.Client that injects the given bearer token.
func httpClientWithAuth(tok string) *http.Client {
	return &http.Client{
		Transport: &authRoundTripper{token: tok, base: http.DefaultTransport},
	}
}

// mcpRequest sends a raw MCP JSON-RPC POST to the MCP test server.
func mcpRequest(t *testing.T, mcpServer *httptest.Server, jti, tokenStr string, rpcBody any) *http.Response {
	t.Helper()
	bodyBytes, err := json.Marshal(rpcBody)
	if err != nil {
		t.Fatalf("marshal rpc body: %v", err)
	}
	url := mcpServer.URL + "/" + jti
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(string(bodyBytes)))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("mcp request failed: %v", err)
	}
	return resp
}

// --------------------------------------------------------------------------
// Test 1: Minting a scoped token returns mcp_endpoint in response
// --------------------------------------------------------------------------

func TestE2E_MCP_MintScopedToken_ReturnsMCPEndpoint(t *testing.T) {
	h := newMCPHarness(t)
	org := h.createOrg(t)
	cred := h.storeCredential(t, org, "https://api.example.com", "bearer", "sk-fake")

	integ := h.createNangoIntegrationForProvider(t, org, "slack", "Slack MCP Test")
	connID := h.createLocalConnection(t, org, integ.ID, "nango-mcp-1")

	scopesJSON := fmt.Sprintf(`[{"connection_id":%q,"actions":["send_message","list_channels"]}]`, connID)
	tok, jti, mcpEndpoint := h.mintScopedTokenWithEndpoint(t, org, cred.ID, scopesJSON)

	if tok == "" {
		t.Fatal("token is empty")
	}
	if jti == "" {
		t.Fatal("jti is empty")
	}
	if mcpEndpoint == "" {
		t.Fatal("expected mcp_endpoint in response, got empty")
	}
	expected := "http://mcp.test/" + jti
	if mcpEndpoint != expected {
		t.Fatalf("expected mcp_endpoint=%q, got %q", expected, mcpEndpoint)
	}
}

// --------------------------------------------------------------------------
// Test 2: MCP client connects and tools/list returns correct tools
// --------------------------------------------------------------------------

func TestE2E_MCP_ToolsList_ReturnsCorrectTools(t *testing.T) {
	h := newMCPHarness(t)
	org := h.createOrg(t)
	cred := h.storeCredential(t, org, "https://api.example.com", "bearer", "sk-fake")

	integ := h.createNangoIntegrationForProvider(t, org, "slack", "Slack Tools")
	connID := h.createLocalConnection(t, org, integ.ID, "nango-mcp-tools")

	scopesJSON := fmt.Sprintf(`[{"connection_id":%q,"actions":["send_message","read_messages","list_channels"]}]`, connID)
	tok, jti, _ := h.mintScopedTokenWithEndpoint(t, org, cred.ID, scopesJSON)

	mcpSrv := httptest.NewServer(h.mcpRouter)
	defer mcpSrv.Close()

	transport := &mcpsdk.StreamableClientTransport{
		Endpoint:             mcpSrv.URL + "/" + jti,
		HTTPClient:           httpClientWithAuth(tok),
		DisableStandaloneSSE: true,
	}

	client := mcpsdk.NewClient(&mcpsdk.Implementation{
		Name:    "test-client",
		Version: "v0.0.1",
	}, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		t.Fatalf("MCP connect failed: %v", err)
	}
	defer session.Close()

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("tools/list failed: %v", err)
	}

	if len(tools.Tools) != 3 {
		t.Fatalf("expected 3 tools, got %d: %+v", len(tools.Tools), tools.Tools)
	}

	toolNames := make(map[string]bool)
	for _, tool := range tools.Tools {
		toolNames[tool.Name] = true
	}
	for _, expected := range []string{"slack_send_message", "slack_read_messages", "slack_list_channels"} {
		if !toolNames[expected] {
			t.Fatalf("expected tool %q not found in %v", expected, toolNames)
		}
	}
}

// --------------------------------------------------------------------------
// Test 3: Calling a tool proxies through to Nango
// --------------------------------------------------------------------------

func TestE2E_MCP_CallTool_ProxiesToNango(t *testing.T) {
	h := newMCPHarness(t)
	org := h.createOrg(t)
	cred := h.storeCredential(t, org, "https://api.example.com", "bearer", "sk-fake")

	integ := h.createNangoIntegrationForProvider(t, org, "slack", "Slack Call")
	connID := h.createLocalConnection(t, org, integ.ID, "nango-mcp-call")

	scopesJSON := fmt.Sprintf(`[{"connection_id":%q,"actions":["list_channels"]}]`, connID)
	tok, jti, _ := h.mintScopedTokenWithEndpoint(t, org, cred.ID, scopesJSON)

	mcpSrv := httptest.NewServer(h.mcpRouter)
	defer mcpSrv.Close()

	transport := &mcpsdk.StreamableClientTransport{
		Endpoint:             mcpSrv.URL + "/" + jti,
		HTTPClient:           httpClientWithAuth(tok),
		DisableStandaloneSSE: true,
	}

	client := mcpsdk.NewClient(&mcpsdk.Implementation{
		Name:    "test-client",
		Version: "v0.0.1",
	}, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		t.Fatalf("MCP connect failed: %v", err)
	}
	defer session.Close()

	// Call the tool — Nango will likely return an error since the connection
	// is local-only, but the tool handler runs and the MCP protocol returns
	// a valid CallToolResult (with IsError=true for the Nango error).
	result, err := session.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "slack_list_channels",
		Arguments: json.RawMessage(`{"limit":10}`),
	})
	if err != nil {
		t.Fatalf("CallTool protocol error: %v", err)
	}

	// We expect the tool to return content (possibly an error from Nango proxy)
	if len(result.Content) == 0 {
		t.Fatal("expected content in tool result")
	}

	t.Logf("tool result: isError=%v, content=%d items", result.IsError, len(result.Content))
}

// --------------------------------------------------------------------------
// Test 4: Token without scopes → MCP endpoint returns 403
// --------------------------------------------------------------------------

func TestE2E_MCP_NoScopes_Returns403(t *testing.T) {
	h := newMCPHarness(t)
	org := h.createOrg(t)
	cred := h.storeCredential(t, org, "https://api.example.com", "bearer", "sk-fake")

	// Mint token WITHOUT scopes (via the standard endpoint, not /v1/mcp-tokens)
	tok := h.mintToken(t, org, cred.ID)
	jwtStr := strings.TrimPrefix(tok, "ptok_")
	claims, err := token.Validate(h.signingKey, jwtStr)
	if err != nil {
		t.Fatalf("validate token: %v", err)
	}

	mcpSrv := httptest.NewServer(h.mcpRouter)
	defer mcpSrv.Close()

	rpcBody := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params":  map[string]any{},
	}
	resp := mcpRequest(t, mcpSrv, claims.ID, tok, rpcBody)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for token without scopes, got %d", resp.StatusCode)
	}
}

// --------------------------------------------------------------------------
// Test 5: Revoked token → MCP endpoint returns 401
// --------------------------------------------------------------------------

func TestE2E_MCP_RevokedToken_Returns401(t *testing.T) {
	h := newMCPHarness(t)
	org := h.createOrg(t)
	cred := h.storeCredential(t, org, "https://api.example.com", "bearer", "sk-fake")

	integ := h.createNangoIntegrationForProvider(t, org, "slack", "Slack Revoke")
	connID := h.createLocalConnection(t, org, integ.ID, "nango-mcp-revoke")

	scopesJSON := fmt.Sprintf(`[{"connection_id":%q,"actions":["send_message"]}]`, connID)
	tok, jti, _ := h.mintScopedTokenWithEndpoint(t, org, cred.ID, scopesJSON)

	mcpSrv := httptest.NewServer(h.mcpRouter)
	defer mcpSrv.Close()

	// Revoke the token
	req := httptest.NewRequest(http.MethodDelete, "/v1/mcp-tokens/"+jti, nil)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("revoke: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Try to use revoked token on MCP endpoint
	rpcBody := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params":  map[string]any{},
	}
	resp := mcpRequest(t, mcpSrv, jti, tok, rpcBody)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for revoked token, got %d", resp.StatusCode)
	}
}

// --------------------------------------------------------------------------
// Test 6: Token revocation evicts cached MCP server
// --------------------------------------------------------------------------

func TestE2E_MCP_Revocation_EvictsCache(t *testing.T) {
	h := newMCPHarness(t)
	org := h.createOrg(t)
	cred := h.storeCredential(t, org, "https://api.example.com", "bearer", "sk-fake")

	integ := h.createNangoIntegrationForProvider(t, org, "slack", "Slack Cache")
	connID := h.createLocalConnection(t, org, integ.ID, "nango-mcp-cache")

	scopesJSON := fmt.Sprintf(`[{"connection_id":%q,"actions":["list_channels"]}]`, connID)
	tok, jti, _ := h.mintScopedTokenWithEndpoint(t, org, cred.ID, scopesJSON)

	mcpSrv := httptest.NewServer(h.mcpRouter)
	defer mcpSrv.Close()

	// Step 1: connect and list tools to populate the server cache
	transport := &mcpsdk.StreamableClientTransport{
		Endpoint:             mcpSrv.URL + "/" + jti,
		HTTPClient:           httpClientWithAuth(tok),
		DisableStandaloneSSE: true,
	}

	client := mcpsdk.NewClient(&mcpsdk.Implementation{
		Name:    "test-client",
		Version: "v0.0.1",
	}, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		t.Fatalf("initial MCP connect: %v", err)
	}

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("initial tools/list: %v", err)
	}
	if len(tools.Tools) == 0 {
		t.Fatal("expected tools in initial list")
	}
	session.Close()

	// Step 2: Revoke the token — this should evict the server cache
	req := httptest.NewRequest(http.MethodDelete, "/v1/mcp-tokens/"+jti, nil)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("revoke: expected 200, got %d", rr.Code)
	}

	// Step 3: attempt to use — should be rejected (401) since token is revoked in DB
	rpcBody := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params":  map[string]any{},
	}
	resp := mcpRequest(t, mcpSrv, jti, tok, rpcBody)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 after revocation+cache eviction, got %d", resp.StatusCode)
	}
}

// --------------------------------------------------------------------------
// Test 7: GET /v1/connections/available-scopes returns enriched connections
// --------------------------------------------------------------------------

func TestE2E_MCP_AvailableScopes(t *testing.T) {
	h := newMCPHarness(t)
	org := h.createOrg(t)

	// Create Slack integration + connection
	slackInteg := h.createNangoIntegrationForProvider(t, org, "slack", "Slack Scopes")
	h.createLocalConnection(t, org, slackInteg.ID, "nango-scopes-slack")

	// Create Notion integration + connection
	notionInteg := h.createNangoIntegrationForProvider(t, org, "notion", "Notion Scopes")
	h.createLocalConnection(t, org, notionInteg.ID, "nango-scopes-notion")

	// Create asana integration (no catalog actions) + connection
	asanaInteg := h.createNangoIntegrationForProvider(t, org, "asana", "Asana Scopes")
	h.createLocalConnection(t, org, asanaInteg.ID, "nango-scopes-asana")

	// GET available-scopes
	req := httptest.NewRequest(http.MethodGet, "/v1/connections/available-scopes", nil)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("available-scopes: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var result []map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	// Slack and Notion should be included; Asana excluded (no catalog actions)
	if len(result) < 2 {
		t.Fatalf("expected at least 2 connections with actions, got %d", len(result))
	}

	providers := make(map[string]map[string]any)
	for _, conn := range result {
		p := conn["provider"].(string)
		providers[p] = conn
	}

	// Verify Slack present with actions
	slackConn, ok := providers["slack"]
	if !ok {
		t.Fatal("expected Slack in available-scopes")
	}
	slackActions := slackConn["actions"].([]any)
	if len(slackActions) == 0 {
		t.Fatal("expected Slack actions non-empty")
	}
	firstAction := slackActions[0].(map[string]any)
	if firstAction["key"] == nil || firstAction["key"] == "" {
		t.Fatal("action missing key")
	}
	if firstAction["display_name"] == nil || firstAction["display_name"] == "" {
		t.Fatal("action missing display_name")
	}

	// Verify Notion present
	if _, ok := providers["notion"]; !ok {
		t.Fatal("expected Notion in available-scopes")
	}

	// Verify Asana excluded
	if _, ok := providers["asana"]; ok {
		t.Fatal("Asana should not be in available-scopes (no catalog actions)")
	}

	t.Logf("available-scopes returned %d connections", len(result))
}

// --------------------------------------------------------------------------
// Test 8: SSE transport at /sse/{jti}
// --------------------------------------------------------------------------

func TestE2E_MCP_SSE_Transport(t *testing.T) {
	h := newMCPHarness(t)
	org := h.createOrg(t)
	cred := h.storeCredential(t, org, "https://api.example.com", "bearer", "sk-fake")

	integ := h.createNangoIntegrationForProvider(t, org, "slack", "Slack SSE")
	connID := h.createLocalConnection(t, org, integ.ID, "nango-mcp-sse")

	scopesJSON := fmt.Sprintf(`[{"connection_id":%q,"actions":["send_message","list_channels"]}]`, connID)
	tok, jti, _ := h.mintScopedTokenWithEndpoint(t, org, cred.ID, scopesJSON)

	mcpSrv := httptest.NewServer(h.mcpRouter)
	defer mcpSrv.Close()

	// Connect via SSE transport — use custom HTTP client to inject auth header
	sseURL := mcpSrv.URL + "/sse/" + jti
	transport := &mcpsdk.SSEClientTransport{
		Endpoint:   sseURL,
		HTTPClient: httpClientWithAuth(tok),
	}

	client := mcpsdk.NewClient(&mcpsdk.Implementation{
		Name:    "test-sse-client",
		Version: "v0.0.1",
	}, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		t.Fatalf("SSE connect failed: %v", err)
	}
	defer session.Close()

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("SSE tools/list failed: %v", err)
	}

	if len(tools.Tools) != 2 {
		t.Fatalf("expected 2 tools via SSE, got %d", len(tools.Tools))
	}

	toolNames := make(map[string]bool)
	for _, tool := range tools.Tools {
		toolNames[tool.Name] = true
	}
	for _, expected := range []string{"slack_send_message", "slack_list_channels"} {
		if !toolNames[expected] {
			t.Fatalf("expected tool %q not found via SSE in %v", expected, toolNames)
		}
	}

	t.Logf("SSE transport returned %d tools", len(tools.Tools))
}
