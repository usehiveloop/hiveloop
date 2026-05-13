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
				Name:         "Cfg Agent " + tc.name + "-" + uuid.New().String()[:8],
				Model:        "claude-sonnet-4-5",
				SystemPrompt: "test agent",
				Status:       "active",
				Tools:        model.JSON{}, McpServers: model.JSON{}, Skills: model.JSON{},
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

func TestPusherAgentConfig_ProviderPromptModelOverride(t *testing.T) {
	db := setupPusherTestDB(t)
	encKey := testPusherEncKey(t)
	signingKey := []byte("test-signing-key-for-provider-model")

	org := model.Org{ID: uuid.New(), Name: "provider-model-org-" + uuid.New().String()[:8], Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", org.ID).Delete(&model.Org{}) })

	encrypted, err := encKey.EncryptString("sk-provider-model")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	cred := model.Credential{
		ID: uuid.New(), OrgID: org.ID,
		ProviderID: "openai", Label: "Cfg OpenAI",
		EncryptedKey: encrypted, WrappedDEK: []byte("test"),
		BaseURL: "https://api.openai.com", AuthScheme: "bearer",
	}
	if err := db.Create(&cred).Error; err != nil {
		t.Fatalf("create cred: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", cred.ID).Delete(&model.Credential{}) })

	agent := model.Agent{
		ID: uuid.New(), OrgID: &org.ID, CredentialID: &cred.ID,
		Name:  "Provider Model Override " + uuid.New().String()[:8],
		Model: "gpt-5-pro",
		ProviderPrompts: model.ProviderPromptsMap{
			"openai": {SystemPrompt: "Use the provider-specific prompt.", Model: "kimi-k2"},
		},
		SystemPrompt: "fallback prompt",
		Status:       "active",
		Tools:        model.JSON{}, McpServers: model.JSON{}, Skills: model.JSON{},
		Integrations: model.JSON{}, AgentConfig: model.JSON{}, Permissions: model.JSON{},
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", agent.ID).Delete(&model.Agent{}) })

	cfg := &config.Config{ProxyHost: "proxy.cfg.test", MCPBaseURL: "https://mcp.cfg.test"}
	pusher := NewPusher(db, nil, signingKey, cfg, nil)

	def := pusher.buildAgentDefinition(t.Context(), &agent, &cred, "ptok_cfg", uuid.New().String())
	if def.Provider.Model != "kimi-k2" {
		t.Fatalf("provider.model = %q, want kimi-k2", def.Provider.Model)
	}
	if def.SystemPrompt != "Use the provider-specific prompt." {
		t.Fatalf("system_prompt = %q, want provider-specific prompt", def.SystemPrompt)
	}
	if def.Config == nil || def.Config.MaxTokens == nil {
		t.Fatal("expected config.max_tokens")
	}
	if *def.Config.MaxTokens != 209715 {
		t.Fatalf("config.max_tokens = %d, want kimi default 209715", *def.Config.MaxTokens)
	}
}
