package sandbox

import (
	"encoding/base64"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	bridgepkg "github.com/ziraloop/ziraloop/internal/bridge"
	"github.com/ziraloop/ziraloop/internal/config"
	"github.com/ziraloop/ziraloop/internal/crypto"
	"github.com/ziraloop/ziraloop/internal/model"
	subagents "github.com/ziraloop/ziraloop/internal/sub-agents"
)

const pusherTestDBURL = "postgres://ziraloop:localdev@localhost:5433/ziraloop_test?sslmode=disable"

// TestPusherBuildAgentDefinition seeds a real agent (modeled after the Railway
// devops agent) into a test database, builds the Bridge AgentDefinition via the
// Pusher, and asserts the output is correct.
func TestPusherBuildAgentDefinition(t *testing.T) {
	db := setupPusherTestDB(t)
	encKey := testPusherEncKey(t)
	signingKey := []byte("test-signing-key-for-pusher-test")

	// 1. Seed system subagents (codebase-explorer, codebase-summarizer, critic)
	if err := subagents.Seed(db); err != nil {
		t.Fatalf("seed subagents: %v", err)
	}

	// 2. Create org
	org := model.Org{ID: uuid.New(), Name: "test-pusher-org", Active: true}
	db.Create(&org)
	t.Cleanup(func() { db.Where("id = ?", org.ID).Delete(&model.Org{}) })

	// 3. Create credential
	encrypted, _ := encKey.EncryptString("sk-test-key-for-pusher")
	cred := model.Credential{
		ID: uuid.New(), OrgID: org.ID,
		ProviderID: "moonshotai", Label: "Test Kimi",
		EncryptedKey: encrypted, WrappedDEK: []byte("test"),
		BaseURL: "https://api.moonshot.cn", AuthScheme: "bearer",
	}
	db.Create(&cred)
	t.Cleanup(func() { db.Where("id = ?", cred.ID).Delete(&model.Credential{}) })

	// 4. Create the agent with permissions and resources
	permissions := model.JSON{
		"RipGrep": "allow", "Read": "allow", "Glob": "allow", "LS": "allow",
		"bash": "allow", "skill": "allow",
		"edit": "deny", "write": "deny", "multiedit": "deny",
		"web_fetch": "deny", "web_search": "deny", "web_crawl": "deny",
	}
	resources := model.JSON{
		"conn-github-123": map[string]any{
			"repository": []any{
				map[string]any{"id": "ziraloop/bridge", "name": "bridge"},
				map[string]any{"id": "ziraloop/ziraloop", "name": "ziraloop"},
			},
		},
	}
	agent := model.Agent{
		ID: uuid.New(), OrgID: &org.ID, CredentialID: &cred.ID,
		Name: "Test Railway Agent", Model: "kimi-k2",
		SystemPrompt: "You are a DevOps engineer.", SandboxType: "dedicated",
		Status: "active", AgentType: "agent", SharedMemory: false,
		Permissions: permissions, Resources: resources,
		Tools: model.JSON{}, McpServers: model.JSON{}, Skills: model.JSON{},
		Integrations: model.JSON{
			"conn-github-123": map[string]any{"actions": []any{"repos.list", "issues.list"}},
		},
		AgentConfig: model.JSON{}, SandboxTools: pq.StringArray{"chrome"},
	}
	db.Create(&agent)
	t.Cleanup(func() { db.Where("id = ?", agent.ID).Delete(&model.Agent{}) })

	// 5. Attach subagents
	var subagentRecords []model.Agent
	db.Where("agent_type = 'subagent' AND is_system = true AND name IN ?",
		[]string{"codebase-explorer", "codebase-summarizer", "critic"}).
		Find(&subagentRecords)

	if len(subagentRecords) != 3 {
		t.Fatalf("expected 3 subagents seeded, got %d", len(subagentRecords))
	}

	for _, sub := range subagentRecords {
		db.Create(&model.AgentSubagent{AgentID: agent.ID, SubagentID: sub.ID})
	}
	t.Cleanup(func() { db.Where("agent_id = ?", agent.ID).Delete(&model.AgentSubagent{}) })

	// 6. Create and attach a skill
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

	// 7. Build the Pusher and call buildAgentDefinition
	cfg := &config.Config{
		ProxyHost:  "proxy.test.com",
		MCPBaseURL: "https://mcp.test.com",
	}
	pusher := NewPusher(db, nil, signingKey, cfg)

	proxyToken := "ptok_test_token"
	jti := uuid.New().String()
	def := pusher.buildAgentDefinition(&agent, &cred, proxyToken, jti)

	// ─── ASSERTIONS ───────────────────────────────────────────────────

	// Agent basics
	assertEqual(t, "name", def.Name, "Test Railway Agent")
	assertEqual(t, "model", def.Provider.Model, "kimi-k2")
	assertEqual(t, "provider_type", string(def.Provider.ProviderType), string(bridgepkg.OpenAi))
	assertContains(t, "base_url", *def.Provider.BaseUrl, "proxy.test.com")
	assertEqual(t, "api_key", def.Provider.ApiKey, proxyToken)

	// System prompt includes repo context
	assertContains(t, "system_prompt", def.SystemPrompt, "You are a DevOps engineer.")
	assertContains(t, "system_prompt repo context", def.SystemPrompt, "CLONED REPOSITORIES")
	assertContains(t, "system_prompt bridge repo", def.SystemPrompt, "ziraloop/bridge")
	assertContains(t, "system_prompt ziraloop repo", def.SystemPrompt, "ziraloop/ziraloop")

	// Permissions — deny entries should be stripped out and moved to DisabledTools
	if def.Permissions == nil {
		t.Fatal("permissions should not be nil")
	}
	perms := *def.Permissions
	if len(perms) != 6 {
		t.Errorf("permissions: expected 6 allow keys, got %d", len(perms))
	}
	if perms["RipGrep"] != bridgepkg.ToolPermissionAllow {
		t.Errorf("permissions[RipGrep]: got %q, want allow", perms["RipGrep"])
	}
	if _, hasDeny := perms["edit"]; hasDeny {
		t.Error("permissions should not contain denied tool 'edit'")
	}

	// DisabledTools — denied permissions should appear here
	if def.Config == nil || def.Config.DisabledTools == nil {
		t.Fatal("config.disabled_tools should not be nil")
	}
	disabledSet := make(map[string]bool)
	for _, tool := range *def.Config.DisabledTools {
		disabledSet[tool] = true
	}
	if len(disabledSet) != 6 {
		t.Errorf("disabled_tools: expected 6, got %d: %v", len(disabledSet), *def.Config.DisabledTools)
	}
	for _, denied := range []string{"edit", "write", "multiedit", "web_fetch", "web_search", "web_crawl"} {
		if !disabledSet[denied] {
			t.Errorf("disabled_tools: missing %q", denied)
		}
	}

	// MCP servers (ziraloop MCP should be present because agent has integrations)
	if def.McpServers == nil {
		t.Fatal("mcp_servers should not be nil")
	}
	mcpNames := make([]string, len(*def.McpServers))
	for i, mcp := range *def.McpServers {
		mcpNames[i] = mcp.Name
	}
	assertSliceContains(t, "mcp_servers", mcpNames, "ziraloop")

	// Skills
	if def.Skills == nil {
		t.Fatal("skills should not be nil")
	}
	if len(*def.Skills) != 1 {
		t.Errorf("skills: expected 1, got %d", len(*def.Skills))
	} else {
		if (*def.Skills)[0].Title != "use-railway" {
			t.Errorf("skill title: got %q, want use-railway", (*def.Skills)[0].Title)
		}
	}

	// Config defaults
	if def.Config == nil {
		t.Fatal("config should not be nil")
	}
	if def.Config.MaxTurns == nil || *def.Config.MaxTurns != 250 {
		t.Errorf("config.max_turns: expected 250, got %v", def.Config.MaxTurns)
	}

	// 8. Build subagent definitions and attach to parent (mirrors pushAgentToSandbox)
	subDefs, err := pusher.buildSubagentDefinitions(&agent, &cred)
	if err != nil {
		t.Fatalf("buildSubagentDefinitions: %v", err)
	}
	if len(subDefs) != 3 {
		t.Fatalf("subagents: expected 3, got %d", len(subDefs))
	}

	// Attach subagents to parent def (exactly as pushAgentToSandbox does)
	def.Subagents = &subDefs
	if def.Subagents == nil || len(*def.Subagents) != 3 {
		t.Fatalf("parent def.Subagents: expected 3, got %v", def.Subagents)
	}

	subNames := make(map[string]bridgepkg.AgentDefinition)
	for _, sub := range subDefs {
		subNames[sub.Name] = sub
	}

	// Each subagent should have:
	// - permissions from YAML (RipGrep, AstGrep, Read, Glob, LS, bash, web_*, skill)
	// - no MCP servers (subagents don't get integration tools)
	// - inherits parent model
	for _, name := range []string{"codebase-explorer", "codebase-summarizer", "critic"} {
		sub, ok := subNames[name]
		if !ok {
			t.Errorf("subagent %q not found", name)
			continue
		}

		// Should have permissions
		if sub.Permissions == nil || len(*sub.Permissions) == 0 {
			t.Errorf("subagent %q: permissions should not be empty", name)
		} else {
			subPerms := *sub.Permissions
			if subPerms["RipGrep"] != bridgepkg.ToolPermissionAllow {
				t.Errorf("subagent %q: RipGrep permission should be allow, got %q", name, subPerms["RipGrep"])
			}
			if subPerms["AstGrep"] != bridgepkg.ToolPermissionAllow {
				t.Errorf("subagent %q: AstGrep permission should be allow, got %q", name, subPerms["AstGrep"])
			}
			if subPerms["Read"] != bridgepkg.ToolPermissionAllow {
				t.Errorf("subagent %q: Read permission should be allow, got %q", name, subPerms["Read"])
			}
			if subPerms["bash"] != bridgepkg.ToolPermissionAllow {
				t.Errorf("subagent %q: bash permission should be allow, got %q", name, subPerms["bash"])
			}
			if subPerms["skill"] != bridgepkg.ToolPermissionAllow {
				t.Errorf("subagent %q: skill permission should be allow, got %q", name, subPerms["skill"])
			}
		}

		// Should inherit parent model
		if sub.Provider.Model != "kimi-k2" {
			t.Errorf("subagent %q: model should be kimi-k2 (inherited), got %q", name, sub.Provider.Model)
		}

		// Should have a system prompt
		if sub.SystemPrompt == "" {
			t.Errorf("subagent %q: system_prompt should not be empty", name)
		}

		// Should NOT have MCP servers (subagents don't get integrations)
		if sub.McpServers != nil && len(*sub.McpServers) > 0 {
			t.Errorf("subagent %q: should not have MCP servers, got %d", name, len(*sub.McpServers))
		}
	}
}

// TestBuildScopesFromIntegrations verifies the integration → MCP scope conversion
// that pushAgentToSandbox stores in the DB token.
func TestBuildScopesFromIntegrations(t *testing.T) {
	// Empty integrations
	scopes := buildScopesFromIntegrations(model.JSON{})
	if scopes != nil {
		t.Errorf("empty integrations: expected nil, got %v", scopes)
	}

	// Single connection with actions
	integrations := model.JSON{
		"conn-github-123": map[string]any{
			"actions": []any{"repos.list", "issues.list", "pulls.create"},
		},
	}
	scopes = buildScopesFromIntegrations(integrations)
	if len(scopes) != 1 {
		t.Fatalf("expected 1 scope, got %d", len(scopes))
	}
	scopeActions, ok := scopes[0]["actions"].([]string)
	if !ok {
		t.Fatal("scope actions should be []string")
	}
	if len(scopeActions) != 3 {
		t.Errorf("expected 3 actions, got %d", len(scopeActions))
	}

	// Multiple connections
	integrations = model.JSON{
		"conn-github-123": map[string]any{
			"actions": []any{"repos.list"},
		},
		"conn-slack-456": map[string]any{
			"actions": []any{"channels.list", "messages.send"},
		},
	}
	scopes = buildScopesFromIntegrations(integrations)
	if len(scopes) != 2 {
		t.Errorf("expected 2 scopes, got %d", len(scopes))
	}

	// Connection with no actions key
	integrations = model.JSON{
		"conn-github-123": map[string]any{
			"other_key": "value",
		},
	}
	scopes = buildScopesFromIntegrations(integrations)
	if len(scopes) != 0 {
		t.Errorf("connection with no actions: expected 0 scopes, got %d", len(scopes))
	}
}

// ─── Helpers ──────────────────────────────────────────────────────────────

func setupPusherTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = pusherTestDBURL
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Skipf("DB not available: %v", err)
	}
	sqlDB, _ := db.DB()
	if err := sqlDB.Ping(); err != nil {
		t.Skipf("DB ping failed: %v", err)
	}
	if err := model.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	return db
}

func testPusherEncKey(t *testing.T) *crypto.SymmetricKey {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 42)
	}
	sk, err := crypto.NewSymmetricKey(base64.StdEncoding.EncodeToString(key))
	if err != nil {
		t.Fatal(err)
	}
	return sk
}

func assertEqual(t *testing.T, field, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s: got %q, want %q", field, got, want)
	}
}

func assertContains(t *testing.T, field, haystack, needle string) {
	t.Helper()
	if len(haystack) == 0 {
		t.Errorf("%s: empty string", field)
		return
	}
	for i := 0; i <= len(haystack)-len(needle); i++ {
		if haystack[i:i+len(needle)] == needle {
			return
		}
	}
	t.Errorf("%s: does not contain %q (len=%d)", field, needle, len(haystack))
}

func assertSliceContains(t *testing.T, field string, slice []string, want string) {
	t.Helper()
	for _, s := range slice {
		if s == want {
			return
		}
	}
	t.Errorf("%s: %v does not contain %q", field, slice, want)
}
