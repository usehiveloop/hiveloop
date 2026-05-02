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

// TestPusherDeadFeatures_NotEmittedOnWire is an invariant test:
//   - Insert an Agent with legacy AgentConfig JSONB fields populated as if
//     the row was migrated from the pre-Wave-2 schema (immortal,
//     history_strip, tool_requirements, verifier).
//   - Push it through buildAgentDefinition + UpsertAgent.
//   - Assert NONE of those keys appear in the wire-format JSON.
//
// This catches regressions where someone re-adds an apply* helper that
// silently re-emits a dropped field.
func TestPusherDeadFeatures_NotEmittedOnWire(t *testing.T) {
	db := setupPusherTestDB(t)
	encKey := testPusherEncKey(t)
	signingKey := []byte("test-signing-key-for-dead-features")

	org := model.Org{ID: uuid.New(), Name: "dead-org-" + uuid.New().String()[:8], Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", org.ID).Delete(&model.Org{}) })

	encrypted, err := encKey.EncryptString("sk-dead-key")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	cred := model.Credential{
		ID: uuid.New(), OrgID: org.ID,
		ProviderID: "anthropic", Label: "Dead Anthropic",
		EncryptedKey: encrypted, WrappedDEK: []byte("test"),
		BaseURL: "https://api.anthropic.com", AuthScheme: "bearer",
	}
	if err := db.Create(&cred).Error; err != nil {
		t.Fatalf("create cred: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", cred.ID).Delete(&model.Credential{}) })

	// Simulate an old DB row that still carries pre-Wave-2 fields.
	legacyConfig := model.JSON{
		"max_tokens":  4096,
		"max_turns":   100,
		"temperature": 0.7,
		// Dead fields below — must NOT round-trip onto the wire.
		"immortal": map[string]any{
			"enabled":          true,
			"token_budget":     50000,
			"expose_journal":   true,
			"retention_window": 10,
		},
		"history_strip": map[string]any{
			"enabled":          true,
			"pin_recent_count": 5,
			"age_threshold":    3,
			"pin_errors":       true,
		},
		"tool_requirements": map[string]any{
			"memory_recall": map[string]any{"cadence": 10, "enforcement": "warn"},
		},
		"verifier": map[string]any{
			"enabled": true,
			"model":   "gpt-4o-mini",
		},
		"max_tasks_per_conversation":       50,
		"max_concurrent_conversations":     5,
		"subagent_timeout_foreground_secs": 300,
		"subagent_timeout_background_secs": 1800,
	}

	agent := model.Agent{
		ID: uuid.New(), OrgID: &org.ID, CredentialID: &cred.ID,
		Name: "Dead Features Agent-" + uuid.New().String()[:8],
		Model: "claude-sonnet-4-5",
		SystemPrompt: "test", Status: "active", AgentType: "agent",
		Tools: model.JSON{}, McpServers: model.JSON{}, Skills: model.JSON{},
		Integrations: model.JSON{}, AgentConfig: legacyConfig, Permissions: model.JSON{},
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", agent.ID).Delete(&model.Agent{}) })

	cfg := &config.Config{ProxyHost: "proxy.dead.test", MCPBaseURL: "https://mcp.dead.test"}
	pusher := NewPusher(db, nil, signingKey, cfg, nil)

	def := pusher.buildAgentDefinition(t.Context(), &agent, &cred, "ptok_dead", uuid.New().String())

	var captured []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/push/agents/") {
			body, _ := io.ReadAll(r.Body)
			captured = body
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	client := bridgepkg.NewBridgeClient(srv.URL, "test-key")
	if err := client.UpsertAgent(context.Background(), agent.ID.String(), def); err != nil {
		t.Fatalf("UpsertAgent: %v", err)
	}

	var body map[string]any
	if err := json.Unmarshal(captured, &body); err != nil {
		t.Fatalf("decode body: %v\n--- raw ---\n%s", err, captured)
	}

	deadTopLevel := []string{
		"tools",
		"subagents",
		"immortal",
		"history_strip",
		"tool_requirements",
		"verifier",
	}
	for _, dead := range deadTopLevel {
		if _, present := body[dead]; present {
			t.Errorf("upsert body contains forbidden top-level field %q (raw=%s)", dead, captured)
		}
	}

	cfgRaw, _ := body["config"].(map[string]any)
	deadInConfig := []string{
		"immortal",
		"history_strip",
		"tool_requirements",
		"verifier",
		"max_tasks_per_conversation",
		"max_concurrent_conversations",
		"subagent_timeout_foreground_secs",
		"subagent_timeout_background_secs",
	}
	for _, dead := range deadInConfig {
		if _, present := cfgRaw[dead]; present {
			t.Errorf("upsert body config contains forbidden field %q (raw=%s)", dead, captured)
		}
	}
}
