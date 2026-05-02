package sandbox

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	bridgepkg "github.com/usehiveloop/hiveloop/internal/bridge"
	"github.com/usehiveloop/hiveloop/internal/config"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// TestBridgeCodegenSmoke proves that the regenerated bridge client emits the
// new ACP-harness wire shape — i.e. the pusher's buildAgentDefinition output
// serializes WITHOUT the dead Wave-1 fields (`tools`, `subagents`,
// `immortal`, `history_strip`) and WITH the new required `harness` enum.
//
// We hit two endpoints with a real httptest server:
//   - PUT  /push/agents/{id}            (UpsertAgent wire shape)
//   - POST /agents/{id}/conversations   (CreateConversationRequest shape)
//
// The first uses a definition built by the same buildAgentDefinition that
// pushAgentToSandbox uses in production, against a real Postgres-backed
// agent fixture. This is the actual proof that the migration's wire format
// is correct end-to-end.
func TestBridgeCodegenSmoke_NewWireShape(t *testing.T) {
	db := setupPusherTestDB(t)
	encKey := testPusherEncKey(t)
	signingKey := []byte("test-signing-key-for-codegen-smoke")

	org := model.Org{ID: uuid.New(), Name: "smoke-org-" + uuid.New().String()[:8], Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", org.ID).Delete(&model.Org{}) })

	encrypted, err := encKey.EncryptString("sk-smoke-key")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	cred := model.Credential{
		ID: uuid.New(), OrgID: org.ID,
		ProviderID: "anthropic", Label: "Smoke Anthropic",
		EncryptedKey: encrypted, WrappedDEK: []byte("test"),
		BaseURL: "https://api.anthropic.com", AuthScheme: "bearer",
	}
	if err := db.Create(&cred).Error; err != nil {
		t.Fatalf("create cred: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", cred.ID).Delete(&model.Credential{}) })

	agent := model.Agent{
		ID: uuid.New(), OrgID: &org.ID, CredentialID: &cred.ID,
		Name: "Smoke Agent", Model: "claude-sonnet-4-5",
		SystemPrompt: "You are a smoke-test agent.",
		Status:       "active",
		Permissions: model.JSON{"Read": "allow"},
		Tools:       model.JSON{}, McpServers: model.JSON{}, Skills: model.JSON{},
		Integrations: model.JSON{}, AgentConfig: model.JSON{},
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", agent.ID).Delete(&model.Agent{}) })

	cfg := &config.Config{
		ProxyHost:  "proxy.smoke.test",
		MCPBaseURL: "https://mcp.smoke.test",
	}
	pusher := NewPusher(db, nil, signingKey, cfg, nil)

	def := pusher.buildAgentDefinition(t.Context(), &agent, &cred, "ptok_smoke", uuid.New().String())

	// --- httptest bridge server ----------------------------------------
	var (
		gotUpsertBody []byte
		gotConvBody   []byte
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/push/agents/"):
			body, _ := io.ReadAll(r.Body)
			gotUpsertBody = body
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/conversations") && strings.HasPrefix(r.URL.Path, "/agents/"):
			body, _ := io.ReadAll(r.Body)
			gotConvBody = body
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"conversation_id":"conv-smoke","stream_url":"http://x/stream"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	client := bridgepkg.NewBridgeClient(srv.URL, "test-key")

	// --- exercise UpsertAgent ------------------------------------------
	if err := client.UpsertAgent(context.Background(), agent.ID.String(), def); err != nil {
		t.Fatalf("UpsertAgent: %v", err)
	}
	if len(gotUpsertBody) == 0 {
		t.Fatal("server did not receive UpsertAgent body")
	}

	var upsertJSON map[string]any
	if err := json.Unmarshal(gotUpsertBody, &upsertJSON); err != nil {
		t.Fatalf("decode upsert body: %v\n--- raw ---\n%s", err, gotUpsertBody)
	}

	// Required new field: harness MUST be present and equal "claude" in Wave 1.
	harness, ok := upsertJSON["harness"]
	if !ok {
		t.Errorf("upsert body missing required `harness` field; got keys: %v", mapKeys(upsertJSON))
	}
	if harness != "claude" {
		t.Errorf("harness = %v, want \"claude\"", harness)
	}

	// Dead fields that MUST NOT appear in the new wire shape.
	for _, dead := range []string{
		"tools",
		"subagents",
		"immortal",
		"history_strip",
		"tool_requirements",
		"verifier",
		"artifacts",
		"integrations",
	} {
		if _, present := upsertJSON[dead]; present {
			t.Errorf("upsert body contains forbidden top-level field %q (raw=%s)", dead, gotUpsertBody)
		}
	}
	// Same check inside `config` — the immortal/history_strip/tool_requirements/verifier
	// fields used to live nested under config.
	if cfgRaw, ok := upsertJSON["config"].(map[string]any); ok {
		for _, dead := range []string{
			"immortal",
			"history_strip",
			"tool_requirements",
			"verifier",
			"max_tasks_per_conversation",
			"max_concurrent_conversations",
			"subagent_timeout_foreground_secs",
			"subagent_timeout_background_secs",
			"system_reminder_refresh_turns",
			"tool_calls_only",
			"rate_limit_rpm",
			"json_schema",
		} {
			if _, present := cfgRaw[dead]; present {
				t.Errorf("upsert body config contains forbidden field %q (raw=%s)", dead, gotUpsertBody)
			}
		}
	}

	// Sanity: required fields the new schema mandates.
	for _, required := range []string{"id", "name", "harness", "system_prompt", "provider"} {
		if _, present := upsertJSON[required]; !present {
			t.Errorf("upsert body missing required field %q", required)
		}
	}

	// --- exercise CreateConversation -----------------------------------
	provider := bridgepkg.ProviderConfig{
		ProviderType: bridgepkg.Anthropic,
		Model:        "claude-sonnet-4-5",
		ApiKey:       "sk-conv",
	}
	if _, err := client.CreateConversationWithOptions(context.Background(), agent.ID.String(), bridgepkg.CreateConversationRequest{
		Provider: &provider,
	}); err != nil {
		t.Fatalf("CreateConversationWithOptions: %v", err)
	}
	if len(gotConvBody) == 0 {
		t.Fatal("server did not receive CreateConversation body")
	}

	var convJSON map[string]any
	if err := json.Unmarshal(gotConvBody, &convJSON); err != nil {
		t.Fatalf("decode conv body: %v\n--- raw ---\n%s", err, gotConvBody)
	}

	// Old per-conversation override fields removed in the new wire shape.
	for _, dead := range []string{"tool_names", "mcp_server_names", "subagent_api_keys"} {
		if _, present := convJSON[dead]; present {
			t.Errorf("conv body contains forbidden field %q (raw=%s)", dead, gotConvBody)
		}
	}

	// Provider override should be inline, not wrapped in a oneOf union.
	provRaw, ok := convJSON["provider"].(map[string]any)
	if !ok {
		t.Fatalf("conv body provider should be an inline object, got %T (raw=%s)", convJSON["provider"], gotConvBody)
	}
	if provRaw["provider_type"] != "anthropic" {
		t.Errorf("conv provider.provider_type = %v, want \"anthropic\"", provRaw["provider_type"])
	}
}

func mapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
