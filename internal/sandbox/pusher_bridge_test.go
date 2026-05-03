package sandbox

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/lib/pq"

	bridgepkg "github.com/usehiveloop/hiveloop/internal/bridge"
	"github.com/usehiveloop/hiveloop/internal/config"
	"github.com/usehiveloop/hiveloop/internal/model"
)

func TestPusherBuildAgentDefinition(t *testing.T) {
	db := setupPusherTestDB(t)
	encKey := testPusherEncKey(t)
	signingKey := []byte("test-signing-key-for-pusher-test")

	org := model.Org{ID: uuid.New(), Name: "test-pusher-org", Active: true}
	db.Create(&org)
	t.Cleanup(func() { db.Where("id = ?", org.ID).Delete(&model.Org{}) })

	encrypted, _ := encKey.EncryptString("sk-test-key-for-pusher")
	cred := model.Credential{
		ID: uuid.New(), OrgID: org.ID,
		ProviderID: "moonshotai", Label: "Test Kimi",
		EncryptedKey: encrypted, WrappedDEK: []byte("test"),
		BaseURL: "https://api.moonshot.cn", AuthScheme: "bearer",
	}
	db.Create(&cred)
	t.Cleanup(func() { db.Where("id = ?", cred.ID).Delete(&model.Credential{}) })

	permissions := model.JSON{
		"RipGrep": "allow", "Read": "allow", "Glob": "allow", "LS": "allow",
		"bash": "allow", "skill": "allow",
		"edit": "deny", "write": "deny", "multiedit": "deny",
		"web_fetch": "deny", "web_search": "deny", "web_crawl": "deny",
	}
	resources := model.JSON{
		"conn-github-123": map[string]any{
			"repository": []any{
				map[string]any{"id": "hiveloop/bridge", "name": "bridge"},
				map[string]any{"id": "hiveloop/hiveloop", "name": "hiveloop"},
			},
		},
	}
	agent := model.Agent{
		ID: uuid.New(), OrgID: &org.ID, CredentialID: &cred.ID,
		Name: "Test Railway Agent", Model: "kimi-k2",
		SystemPrompt: "You are a DevOps engineer.",
		Status:       "active", SharedMemory: false,
		Permissions: permissions, Resources: resources,
		Tools: model.JSON{}, McpServers: model.JSON{}, Skills: model.JSON{},
		Integrations: model.JSON{
			"conn-github-123": map[string]any{"actions": []any{"repos.list", "issues.list"}},
		},
		AgentConfig: model.JSON{}, SandboxTools: pq.StringArray{"chrome"},
	}
	db.Create(&agent)
	t.Cleanup(func() { db.Where("id = ?", agent.ID).Delete(&model.Agent{}) })

	skillVersion := model.SkillVersion{
		ID:      uuid.New(),
		Version: "v1",
		Bundle: model.RawJSON(`{
			"id": "use-railway-test",
			"title": "use-railway",
			"description": "Operate Railway infrastructure",
			"content": "# Use Railway\nDeploy and manage services on Railway."
		}`),
	}
	skill := model.Skill{
		ID: uuid.New(), Slug: "use-railway-test-" + uuid.New().String()[:8],
		Name: "use-railway", SourceType: "inline", Status: "published",
		LatestVersionID: &skillVersion.ID,
	}
	skillVersion.SkillID = skill.ID
	db.Create(&skill)
	db.Create(&skillVersion)
	db.Create(&model.AgentSkill{AgentID: agent.ID, SkillID: skill.ID})
	t.Cleanup(func() {
		db.Where("agent_id = ? AND skill_id = ?", agent.ID, skill.ID).Delete(&model.AgentSkill{})
		db.Where("skill_id = ?", skill.ID).Delete(&model.SkillVersion{})
		db.Where("id = ?", skill.ID).Delete(&model.Skill{})
	})

	cfg := &config.Config{
		ProxyHost:  "proxy.test.com",
		MCPBaseURL: "https://mcp.test.com",
	}
	pusher := NewPusher(db, nil, signingKey, cfg, nil)

	proxyToken := "ptok_test_token"
	jti := uuid.New().String()
	def := pusher.buildAgentDefinition(t.Context(), &agent, &cred, proxyToken, jti)

	assertEqual(t, "name", def.Name, "Test Railway Agent")
	assertEqual(t, "model", def.Provider.Model, "kimi-k2")
	assertEqual(t, "provider_type", string(def.Provider.ProviderType), string(bridgepkg.OpenAi))
	assertEqual(t, "harness", string(def.Harness), string(bridgepkg.OpenCode))
	assertContains(t, "base_url", *def.Provider.BaseUrl, "proxy.test.com")
	assertEqual(t, "api_key", def.Provider.ApiKey, proxyToken)

	assertContains(t, "system_prompt", def.SystemPrompt, "You are a DevOps engineer.")
	assertContains(t, "system_prompt repo context", def.SystemPrompt, "CLONED REPOSITORIES")
	assertContains(t, "system_prompt bridge repo", def.SystemPrompt, "hiveloop/bridge")
	assertContains(t, "system_prompt hiveloop repo", def.SystemPrompt, "hiveloop/hiveloop")

	if def.Permissions != nil {
		t.Errorf("permissions should be nil (per-tool permissions are not pushed to bridge)")
	}
	if def.Config != nil && def.Config.DisabledTools != nil {
		t.Errorf("config.disabled_tools should be nil (per-tool permissions are not pushed to bridge)")
	}

	// Hiveloop MCP server should be injected because the agent has integrations.
	if def.McpServers == nil {
		t.Fatal("mcp_servers should not be nil")
	}
	mcpNames := make([]string, len(*def.McpServers))
	for i, mcp := range *def.McpServers {
		mcpNames[i] = mcp.Name
	}
	assertSliceContains(t, "mcp_servers", mcpNames, "hiveloop")

	if def.Skills == nil {
		t.Fatal("skills should not be nil")
	}
	if len(*def.Skills) != 1 {
		t.Errorf("skills: expected 1, got %d", len(*def.Skills))
	} else if (*def.Skills)[0].Title != "use-railway" {
		t.Errorf("skill title: got %q, want use-railway", (*def.Skills)[0].Title)
	}

	if def.Config == nil {
		t.Fatal("config should not be nil")
	}
	if def.Config.MaxTurns == nil || *def.Config.MaxTurns != 250 {
		t.Errorf("config.max_turns: expected 250, got %v", def.Config.MaxTurns)
	}

	// AgentDefinition has no nested `subagents` field. The subagent feature
	// has been removed entirely; this assertion guards against regressions.
	defJSON, err := json.Marshal(def)
	if err != nil {
		t.Fatalf("marshal def: %v", err)
	}
	if bytes.Contains(defJSON, []byte(`"subagents"`)) {
		t.Errorf("AgentDefinition JSON must not contain a `subagents` field; got %s", defJSON)
	}

	var hiveloopMCP *bridgepkg.McpServerDefinition
	for i := range *def.McpServers {
		if (*def.McpServers)[i].Name == "hiveloop" {
			hiveloopMCP = &(*def.McpServers)[i]
			break
		}
	}
	if hiveloopMCP == nil {
		t.Fatal("expected hiveloop MCP server in def.McpServers when integrations are attached")
	}
	transport, err := hiveloopMCP.Transport.AsMcpTransport1()
	if err != nil {
		t.Fatalf("hiveloop MCP transport must be streamable_http: %v", err)
	}
	if transport.Type != bridgepkg.StreamableHttp {
		t.Errorf("transport.type = %q, want streamable_http", transport.Type)
	}
	if !strings.HasPrefix(transport.Url, "https://mcp.test.com/") {
		t.Errorf("transport.url should start with mcp base URL; got %q", transport.Url)
	}
	if !strings.HasSuffix(transport.Url, jti) {
		t.Errorf("transport.url should end with jti %q; got %q", jti, transport.Url)
	}
	if transport.Headers == nil {
		t.Fatal("hiveloop MCP transport must carry an Authorization header")
	}
	if (*transport.Headers)["Authorization"] != "Bearer "+proxyToken {
		t.Errorf("Authorization header = %q, want bearer of proxy token", (*transport.Headers)["Authorization"])
	}
}
