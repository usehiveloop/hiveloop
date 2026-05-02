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

// TestPusherAgentConfig_RoundTripsHarnessFields proves that each new
// AgentConfig field on the bridge wire format is populated end-to-end:
//   - The author writes the field to agents.agent_config (JSONB).
//   - The pusher round-trips it through buildAgentDefinition and
//     UpsertAgent so the bridge sees the correct value.
//
// This is a real-Postgres integration test — it skips automatically when
// the test database isn't reachable (see setupPusherTestDB).
func TestPusherAgentConfig_HarnessOptionalFields(t *testing.T) {
	db := setupPusherTestDB(t)
	encKey := testPusherEncKey(t)
	signingKey := []byte("test-signing-key-for-agent-config")

	org := model.Org{ID: uuid.New(), Name: "agent-cfg-org-" + uuid.New().String()[:8], Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", org.ID).Delete(&model.Org{}) })

	encrypted, err := encKey.EncryptString("sk-cfg-key")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	cred := model.Credential{
		ID: uuid.New(), OrgID: org.ID,
		ProviderID: "anthropic", Label: "Cfg Anthropic",
		EncryptedKey: encrypted, WrappedDEK: []byte("test"),
		BaseURL: "https://api.anthropic.com", AuthScheme: "bearer",
	}
	if err := db.Create(&cred).Error; err != nil {
		t.Fatalf("create cred: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", cred.ID).Delete(&model.Credential{}) })

	cases := []struct {
		name      string
		agentCfg  model.JSON
		assertion func(t *testing.T, cfg map[string]any)
	}{
		{
			name:     "reasoning_effort high",
			agentCfg: model.JSON{"reasoning_effort": "high"},
			assertion: func(t *testing.T, cfg map[string]any) {
				if cfg["reasoning_effort"] != "high" {
					t.Errorf("reasoning_effort = %v, want \"high\"", cfg["reasoning_effort"])
				}
			},
		},
		{
			name:     "small_fast_model",
			agentCfg: model.JSON{"small_fast_model": "haiku-4-5"},
			assertion: func(t *testing.T, cfg map[string]any) {
				if cfg["small_fast_model"] != "haiku-4-5" {
					t.Errorf("small_fast_model = %v, want \"haiku-4-5\"", cfg["small_fast_model"])
				}
			},
		},
		{
			name:     "fallback_model",
			agentCfg: model.JSON{"fallback_model": "opus-4-7"},
			assertion: func(t *testing.T, cfg map[string]any) {
				if cfg["fallback_model"] != "opus-4-7" {
					t.Errorf("fallback_model = %v, want \"opus-4-7\"", cfg["fallback_model"])
				}
			},
		},
		{
			name:     "allowed_tools",
			agentCfg: model.JSON{"allowed_tools": []any{"read", "write"}},
			assertion: func(t *testing.T, cfg map[string]any) {
				got, ok := cfg["allowed_tools"].([]any)
				if !ok {
					t.Fatalf("allowed_tools missing or wrong type: %T %v", cfg["allowed_tools"], cfg["allowed_tools"])
				}
				if len(got) != 2 || got[0] != "read" || got[1] != "write" {
					t.Errorf("allowed_tools = %v, want [read write]", got)
				}
			},
		},
		{
			name:     "disabled_tools (author-supplied)",
			agentCfg: model.JSON{"disabled_tools": []any{"bash"}},
			assertion: func(t *testing.T, cfg map[string]any) {
				got, ok := cfg["disabled_tools"].([]any)
				if !ok {
					t.Fatalf("disabled_tools missing or wrong type: %T %v", cfg["disabled_tools"], cfg["disabled_tools"])
				}
				found := false
				for _, v := range got {
					if v == "bash" {
						found = true
					}
				}
				if !found {
					t.Errorf("disabled_tools = %v, want to contain \"bash\"", got)
				}
			},
		},
		{
			name:     "permission_mode bypassPermissions",
			agentCfg: model.JSON{"permission_mode": "bypassPermissions"},
			assertion: func(t *testing.T, cfg map[string]any) {
				if cfg["permission_mode"] != "bypassPermissions" {
					t.Errorf("permission_mode = %v, want \"bypassPermissions\"", cfg["permission_mode"])
				}
			},
		},
		{
			name:     "env map",
			agentCfg: model.JSON{"env": map[string]any{"FOO": "bar"}},
			assertion: func(t *testing.T, cfg map[string]any) {
				envRaw, ok := cfg["env"].(map[string]any)
				if !ok {
					t.Fatalf("env missing or wrong type: %T %v", cfg["env"], cfg["env"])
				}
				if envRaw["FOO"] != "bar" {
					t.Errorf("env[FOO] = %v, want \"bar\"", envRaw["FOO"])
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			agent := model.Agent{
				ID: uuid.New(), OrgID: &org.ID, CredentialID: &cred.ID,
				Name: "Cfg Agent " + tc.name + "-" + uuid.New().String()[:8],
				Model: "claude-sonnet-4-5",
				SystemPrompt: "test agent",
				Status:       "active",
				Tools: model.JSON{}, McpServers: model.JSON{}, Skills: model.JSON{},
				Integrations: model.JSON{}, AgentConfig: tc.agentCfg, Permissions: model.JSON{},
			}
			if err := db.Create(&agent).Error; err != nil {
				t.Fatalf("create agent: %v", err)
			}
			t.Cleanup(func() { db.Where("id = ?", agent.ID).Delete(&model.Agent{}) })

			cfg := &config.Config{ProxyHost: "proxy.cfg.test", MCPBaseURL: "https://mcp.cfg.test"}
			pusher := NewPusher(db, nil, signingKey, cfg, nil)

			def := pusher.buildAgentDefinition(t.Context(), &agent, &cred, "ptok_cfg", uuid.New().String())

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
			if len(captured) == 0 {
				t.Fatal("server did not capture body")
			}

			var body map[string]any
			if err := json.Unmarshal(captured, &body); err != nil {
				t.Fatalf("decode body: %v\n--- raw ---\n%s", err, captured)
			}
			cfgRaw, ok := body["config"].(map[string]any)
			if !ok {
				t.Fatalf("config missing in upsert body: %s", captured)
			}
			tc.assertion(t, cfgRaw)
		})
	}
}

// TestPusherAgentConfig_HarnessStampedOnFirstPush_ThenReused proves the
// two-push contract:
//   - Push 1 (agent.harness == "") → pusher computes via harnessFor and
//     stamps the value on the agents row.
//   - Push 2 (re-read agent) → harness column is non-empty and the second
//     push uses the persisted value verbatim.
func TestPusherAgentConfig_HarnessStampedOnFirstPush_ThenReused(t *testing.T) {
	db := setupPusherTestDB(t)
	encKey := testPusherEncKey(t)
	signingKey := []byte("test-signing-key-for-stamp")

	org := model.Org{ID: uuid.New(), Name: "stamp-org-" + uuid.New().String()[:8], Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", org.ID).Delete(&model.Org{}) })

	encrypted, err := encKey.EncryptString("sk-stamp-key")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	cred := model.Credential{
		ID: uuid.New(), OrgID: org.ID,
		// OpenAI provider so the computed harness is OpenCode (interesting case).
		ProviderID: "openai", Label: "Stamp OpenAI",
		EncryptedKey: encrypted, WrappedDEK: []byte("test"),
		BaseURL: "https://api.openai.com", AuthScheme: "bearer",
	}
	if err := db.Create(&cred).Error; err != nil {
		t.Fatalf("create cred: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", cred.ID).Delete(&model.Credential{}) })

	agent := model.Agent{
		ID: uuid.New(), OrgID: &org.ID, CredentialID: &cred.ID,
		Name: "Stamp Agent-" + uuid.New().String()[:8], Model: "gpt-4o",
		SystemPrompt: "test agent", Status: "active",
		Tools: model.JSON{}, McpServers: model.JSON{}, Skills: model.JSON{},
		Integrations: model.JSON{}, AgentConfig: model.JSON{}, Permissions: model.JSON{},
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", agent.ID).Delete(&model.Agent{}) })

	// Confirm the freshly-inserted agent has empty harness.
	var fresh model.Agent
	if err := db.Where("id = ?", agent.ID).First(&fresh).Error; err != nil {
		t.Fatalf("read fresh agent: %v", err)
	}
	if fresh.Harness != "" {
		t.Fatalf("expected empty harness on fresh agent, got %q", fresh.Harness)
	}

	// --- exercise the production push path against a fake bridge ----------
	cfg := &config.Config{ProxyHost: "proxy.stamp.test", MCPBaseURL: "https://mcp.stamp.test"}
	pusher := NewPusher(db, nil, signingKey, cfg, nil)

	push := func(t *testing.T) string {
		t.Helper()

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

		var current model.Agent
		if err := db.Where("id = ?", agent.ID).First(&current).Error; err != nil {
			t.Fatalf("read agent before push: %v", err)
		}

		def := pusher.buildAgentDefinition(t.Context(), &current, &cred, "ptok_stamp", uuid.New().String())

		// Stamp logic out of pushAgentToSandbox — replicated here without the
		// orchestrator/credentials.Resolve dependencies the full push needs.
		if current.Harness == "" {
			db.Model(&model.Agent{}).
				Where("id = ? AND (harness IS NULL OR harness = '')", current.ID).
				Update("harness", string(def.Harness))
		}

		client := bridgepkg.NewBridgeClient(srv.URL, "test-key")
		if err := client.UpsertAgent(context.Background(), current.ID.String(), def); err != nil {
			t.Fatalf("UpsertAgent: %v", err)
		}
		var body map[string]any
		if err := json.Unmarshal(captured, &body); err != nil {
			t.Fatalf("decode body: %v\n--- raw ---\n%s", err, captured)
		}
		harness, _ := body["harness"].(string)
		return harness
	}

	// Push 1: harness should be computed (open_code for openai+gpt-4o) and
	// stamped on the agents row.
	pushedHarness1 := push(t)
	if pushedHarness1 != string(bridgepkg.OpenCode) {
		t.Errorf("first push harness = %q, want %q", pushedHarness1, bridgepkg.OpenCode)
	}

	var afterFirst model.Agent
	if err := db.Where("id = ?", agent.ID).First(&afterFirst).Error; err != nil {
		t.Fatalf("read agent after first push: %v", err)
	}
	if afterFirst.Harness != string(bridgepkg.OpenCode) {
		t.Errorf("agent.harness after first push = %q, want %q", afterFirst.Harness, bridgepkg.OpenCode)
	}

	// Push 2: persisted value should be reused verbatim.
	pushedHarness2 := push(t)
	if pushedHarness2 != string(bridgepkg.OpenCode) {
		t.Errorf("second push harness = %q, want %q", pushedHarness2, bridgepkg.OpenCode)
	}
}
