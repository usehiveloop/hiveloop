package sandbox

import (
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
		Status:       "active", AgentType: "agent", SharedMemory: false,
		Permissions: permissions, Resources: resources,
		Tools: model.JSON{}, McpServers: model.JSON{}, Skills: model.JSON{},
		Integrations: model.JSON{
			"conn-github-123": map[string]any{"actions": []any{"repos.list", "issues.list"}},
		},
		AgentConfig: model.JSON{}, SandboxTools: pq.StringArray{"chrome"},
	}
	db.Create(&agent)
	t.Cleanup(func() { db.Where("id = ?", agent.ID).Delete(&model.Agent{}) })

	suffix := "-" + uuid.New().String()[:8]
	subagentNames := []string{"codebase-explorer" + suffix, "codebase-summarizer" + suffix, "critic" + suffix}
	subagentPerms := model.JSON{
		"RipGrep": "allow", "AstGrep": "allow", "Read": "allow",
		"Glob": "allow", "LS": "allow", "bash": "allow", "skill": "allow",
	}
	subagentRecords := make([]model.Agent, 0, len(subagentNames))
	for _, name := range subagentNames {
		sub := model.Agent{
			ID: uuid.New(), Name: name, Model: "kimi-k2",
			SystemPrompt: "subagent for tests", Status: "active",
			AgentType: "subagent", IsSystem: false,
			Tools: model.JSON{}, McpServers: model.JSON{}, Skills: model.JSON{},
			Integrations: model.JSON{}, AgentConfig: model.JSON{},
			Permissions: subagentPerms,
		}
		if err := db.Create(&sub).Error; err != nil {
			t.Fatalf("creating subagent %s: %v", name, err)
		}
		subagentRecords = append(subagentRecords, sub)
	}
	t.Cleanup(func() {
		for _, sub := range subagentRecords {
			db.Where("id = ?", sub.ID).Delete(&model.Agent{})
		}
	})

	for _, sub := range subagentRecords {
		db.Create(&model.AgentSubagent{AgentID: agent.ID, SubagentID: sub.ID})
	}
	t.Cleanup(func() { db.Where("agent_id = ?", agent.ID).Delete(&model.AgentSubagent{}) })

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
	// Wave 1: every agent is forced onto the Claude harness; Wave 2 will
	// implement deterministic harness selection from (provider, model).
	assertEqual(t, "harness", string(def.Harness), string(bridgepkg.Claude))
	assertContains(t, "base_url", *def.Provider.BaseUrl, "proxy.test.com")
	assertEqual(t, "api_key", def.Provider.ApiKey, proxyToken)

	assertContains(t, "system_prompt", def.SystemPrompt, "You are a DevOps engineer.")
	assertContains(t, "system_prompt repo context", def.SystemPrompt, "CLONED REPOSITORIES")
	assertContains(t, "system_prompt bridge repo", def.SystemPrompt, "hiveloop/bridge")
	assertContains(t, "system_prompt hiveloop repo", def.SystemPrompt, "hiveloop/hiveloop")

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
	} else {
		if (*def.Skills)[0].Title != "use-railway" {
			t.Errorf("skill title: got %q, want use-railway", (*def.Skills)[0].Title)
		}
	}

	if def.Config == nil {
		t.Fatal("config should not be nil")
	}
	if def.Config.MaxTurns == nil || *def.Config.MaxTurns != 250 {
		t.Errorf("config.max_turns: expected 250, got %v", def.Config.MaxTurns)
	}

	// TODO(wave-2): The new ACP-harness AgentDefinition removed the nested
	// `subagents` field — Wave 2 will rebuild subagent registration as a
	// separate call (or move it into the harness adapter). For Wave 1
	// buildSubagentDefinitions returns nil so we just assert the no-op.
	subDefs, err := pusher.buildSubagentDefinitions(t.Context(), &agent, &cred)
	if err != nil {
		t.Fatalf("buildSubagentDefinitions: %v", err)
	}
	if subDefs != nil {
		t.Fatalf("Wave 1 subagent stub must return nil, got %d defs", len(subDefs))
	}
	_ = subagentNames
}
