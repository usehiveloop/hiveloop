package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/forge"
	"github.com/ziraloop/ziraloop/internal/model"
)

// forgeTestDB opens a connection to the test Postgres database and runs
// migrations. The connection is closed when the test finishes.
func forgeTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	loadEnv(t)

	dsn := envOr("DATABASE_URL", testDBURL)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("cannot connect to Postgres: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(3)
	sqlDB.SetMaxIdleConns(1)
	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("Postgres not reachable: %v", err)
	}
	if err := model.AutoMigrate(db); err != nil {
		t.Fatalf("migration failed: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	return db
}

// forgeTestScaffold creates the prerequisite Org, Identity, Credential, and
// Agent records that ForgeRun requires via foreign key constraints. Returns
// the IDs needed to construct a ForgeRun.
type forgeIDs struct {
	orgID        uuid.UUID
	identityID   uuid.UUID
	credentialID uuid.UUID
	agentID      uuid.UUID
}

func createForgeScaffold(t *testing.T, db *gorm.DB) forgeIDs {
	t.Helper()
	suffix := uuid.New().String()[:8]

	org := model.Org{Name: fmt.Sprintf("forge-test-org-%s", suffix)}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}

	identity := model.Identity{
		OrgID:      org.ID,
		ExternalID: fmt.Sprintf("forge-test-identity-%s", suffix),
	}
	if err := db.Create(&identity).Error; err != nil {
		t.Fatalf("create identity: %v", err)
	}

	cred := model.Credential{
		OrgID:        org.ID,
		Label:        "forge-test-cred",
		BaseURL:      "https://api.example.com",
		AuthScheme:   "bearer",
		EncryptedKey: []byte("fake-encrypted-key"),
		WrappedDEK:   []byte("fake-wrapped-dek"),
		ProviderID:   "openai",
	}
	if err := db.Create(&cred).Error; err != nil {
		t.Fatalf("create credential: %v", err)
	}

	agent := model.Agent{
		OrgID:        &org.ID,
		IdentityID:   &identity.ID,
		Name:         fmt.Sprintf("forge-test-agent-%s", suffix),
		CredentialID: &cred.ID,
		SandboxType:  "shared",
		SystemPrompt: "You are a test agent.",
		Model:        "gpt-4o",
		Tools:        model.JSON{},
		McpServers:   model.JSON{},
		Skills:       model.JSON{},
		Integrations: model.JSON{},
		Subagents:    model.JSON{},
		AgentConfig:  model.JSON{},
		Permissions:  model.JSON{},
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}

	return forgeIDs{
		orgID:        org.ID,
		identityID:   identity.ID,
		credentialID: cred.ID,
		agentID:      agent.ID,
	}
}

// forgeMCPRequest creates an HTTP request with the required MCP Streamable HTTP headers.
func forgeMCPRequest(t *testing.T, method, url, body string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	return req
}

// extractSSEData extracts the JSON payload from an SSE response body.
// SSE format: "event: message\ndata: {json}\n\n"
func extractSSEData(t *testing.T, body string) []byte {
	t.Helper()
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data: ") {
			return []byte(strings.TrimPrefix(line, "data: "))
		}
	}
	// If not SSE, assume the body is raw JSON.
	return []byte(body)
}

func TestForgeMCP_ServerFactory_WithRunningEval(t *testing.T) {
	db := forgeTestDB(t)
	ids := createForgeScaffold(t, db)

	// Create ForgeRun with current_iteration = 1.
	run := model.ForgeRun{
		OrgID:                    ids.orgID,
		AgentID:                  ids.agentID,
		ArchitectCredentialID:    ids.credentialID,
		ArchitectModel:           "gpt-4o",
		EvalDesignerCredentialID: ids.credentialID,
		EvalDesignerModel:        "gpt-4o",
		JudgeCredentialID:        ids.credentialID,
		JudgeModel:               "gpt-4o",
		Status:                   model.ForgeStatusRunning,
		CurrentIteration:         1,
	}
	if err := db.Create(&run).Error; err != nil {
		t.Fatalf("create forge run: %v", err)
	}

	// Create ForgeIteration with tool definitions.
	toolDefs := []map[string]any{
		{
			"name":        "search_orders",
			"description": "Search for customer orders",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"order_id": map[string]any{"type": "string"},
				},
			},
		},
		{
			"name":        "process_refund",
			"description": "Process a refund for an order",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"order_id": map[string]any{"type": "string"},
					"amount":   map[string]any{"type": "number"},
				},
			},
		},
	}
	toolsJSON, _ := json.Marshal(toolDefs)

	iter := model.ForgeIteration{
		ForgeRunID: run.ID,
		Iteration:  1,
		Phase:      model.ForgePhaseEvaluating,
		Tools:      model.RawJSON(toolsJSON),
	}
	if err := db.Create(&iter).Error; err != nil {
		t.Fatalf("create forge iteration: %v", err)
	}

	// Create ForgeEvalCase with tool mocks.
	toolMocks := map[string][]forge.MockSample{
		"search_orders": {
			{Match: map[string]any{"order_id": "123"}, Response: map[string]any{"status": "delivered", "total": 49.99}},
			{Match: map[string]any{}, Response: map[string]any{"error": "order not found"}},
		},
		"process_refund": {
			{Match: map[string]any{}, Response: map[string]any{"refund_id": "REF-001", "status": "processed"}},
		},
	}
	toolMocksJSON, _ := json.Marshal(toolMocks)

	evalCase := model.ForgeEvalCase{
		ForgeRunID:       run.ID,
		TestName:         "refund_happy_path",
		Category:         "happy_path",
		Tier:             model.ForgeEvalTierBasic,
		RequirementType:  model.ForgeRequirementHard,
		SampleCount:      3,
		TestPrompt:       "I want a refund for order 123",
		ExpectedBehavior: "Agent should look up the order and process a refund",
		ToolMocks:        model.RawJSON(toolMocksJSON),
	}
	if err := db.Create(&evalCase).Error; err != nil {
		t.Fatalf("create forge eval case: %v", err)
	}

	// Create ForgeEvalResult with status = running.
	evalResult := model.ForgeEvalResult{
		ForgeEvalCaseID:  evalCase.ID,
		ForgeIterationID: iter.ID,
		Status:           model.ForgeEvalRunning,
	}
	if err := db.Create(&evalResult).Error; err != nil {
		t.Fatalf("create forge eval result: %v", err)
	}

	// Build the MCP handler and chi router.
	mcpHandler := forge.NewForgeMCPHandler(db)

	router := chi.NewRouter()
	router.Route("/forge/{forgeRunID}", func(r chi.Router) {
		r.Handle("/*", mcpHandler.StreamableHTTPHandler())
	})

	// Issue an MCP initialize request to verify the server has the correct tools.
	// MCP Streamable HTTP uses JSON-RPC 2.0 over HTTP POST.
	initBody := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
	w := httptest.NewRecorder()
	router.ServeHTTP(w, forgeMCPRequest(t, http.MethodPost, fmt.Sprintf("/forge/%s/mcp", run.ID), initBody))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// After initialize, list the tools.
	listBody := `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, forgeMCPRequest(t, http.MethodPost, fmt.Sprintf("/forge/%s/mcp", run.ID), listBody))

	if w2.Code != http.StatusOK {
		t.Fatalf("tools/list expected 200, got %d: %s", w2.Code, w2.Body.String())
	}

	// Parse the JSON-RPC response to verify tools.
	var rpcResp struct {
		Result struct {
			Tools []struct {
				Name        string `json:"name"`
				Description string `json:"description"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(extractSSEData(t, w2.Body.String()), &rpcResp); err != nil {
		t.Fatalf("unmarshal tools/list response: %v (body: %s)", err, w2.Body.String())
	}

	if len(rpcResp.Result.Tools) != 2 {
		t.Fatalf("expected 2 tools, got %d (body: %s)", len(rpcResp.Result.Tools), w2.Body.String())
	}

	toolNames := map[string]bool{}
	for _, tool := range rpcResp.Result.Tools {
		toolNames[tool.Name] = true
	}
	if !toolNames["search_orders"] {
		t.Error("expected search_orders tool to be registered")
	}
	if !toolNames["process_refund"] {
		t.Error("expected process_refund tool to be registered")
	}
}

func TestForgeMCP_ServerFactory_NoRunningEval(t *testing.T) {
	db := forgeTestDB(t)
	ids := createForgeScaffold(t, db)

	// Create a ForgeRun with current_iteration = 0 (no active iteration).
	run := model.ForgeRun{
		OrgID:                    ids.orgID,
		AgentID:                  ids.agentID,
		ArchitectCredentialID:    ids.credentialID,
		ArchitectModel:           "gpt-4o",
		EvalDesignerCredentialID: ids.credentialID,
		EvalDesignerModel:        "gpt-4o",
		JudgeCredentialID:        ids.credentialID,
		JudgeModel:               "gpt-4o",
		Status:                   model.ForgeStatusQueued,
		CurrentIteration:         0,
	}
	if err := db.Create(&run).Error; err != nil {
		t.Fatalf("create forge run: %v", err)
	}

	mcpHandler := forge.NewForgeMCPHandler(db)

	router := chi.NewRouter()
	router.Route("/forge/{forgeRunID}", func(r chi.Router) {
		r.Handle("/*", mcpHandler.StreamableHTTPHandler())
	})

	// Initialize — should get an empty server (no tools).
	initBody := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
	w := httptest.NewRecorder()
	router.ServeHTTP(w, forgeMCPRequest(t, http.MethodPost, fmt.Sprintf("/forge/%s/mcp", run.ID), initBody))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// List tools — should be empty.
	listBody := `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, forgeMCPRequest(t, http.MethodPost, fmt.Sprintf("/forge/%s/mcp", run.ID), listBody))

	if w2.Code != http.StatusOK {
		t.Fatalf("tools/list expected 200, got %d: %s", w2.Code, w2.Body.String())
	}

	var rpcResp struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(extractSSEData(t, w2.Body.String()), &rpcResp); err != nil {
		t.Fatalf("unmarshal tools/list response: %v (body: %s)", err, w2.Body.String())
	}

	if len(rpcResp.Result.Tools) != 0 {
		t.Errorf("expected 0 tools for empty server, got %d", len(rpcResp.Result.Tools))
	}
}

func TestForgeMCP_ServerFactory_NonexistentRun(t *testing.T) {
	db := forgeTestDB(t)

	mcpHandler := forge.NewForgeMCPHandler(db)

	router := chi.NewRouter()
	router.Route("/forge/{forgeRunID}", func(r chi.Router) {
		r.Handle("/*", mcpHandler.StreamableHTTPHandler())
	})

	// Use a random UUID that does not exist.
	fakeID := uuid.New()

	initBody := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
	w := httptest.NewRecorder()
	router.ServeHTTP(w, forgeMCPRequest(t, http.MethodPost, fmt.Sprintf("/forge/%s/mcp", fakeID), initBody))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// List tools — should be empty (empty server returned for missing run).
	listBody := `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, forgeMCPRequest(t, http.MethodPost, fmt.Sprintf("/forge/%s/mcp", fakeID), listBody))

	if w2.Code != http.StatusOK {
		t.Fatalf("tools/list expected 200, got %d: %s", w2.Code, w2.Body.String())
	}

	var rpcResp struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(extractSSEData(t, w2.Body.String()), &rpcResp); err != nil {
		t.Fatalf("unmarshal tools/list response: %v (body: %s)", err, w2.Body.String())
	}

	if len(rpcResp.Result.Tools) != 0 {
		t.Errorf("expected 0 tools for nonexistent run, got %d", len(rpcResp.Result.Tools))
	}
}
