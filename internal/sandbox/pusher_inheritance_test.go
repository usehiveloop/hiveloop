package sandbox

import (
	"testing"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/config"
	"github.com/usehiveloop/hiveloop/internal/model"
	githubprofile "github.com/usehiveloop/hiveloop/internal/profiles/github"
)

func TestPusherBuildAgentDefinition_InheritsEmployeeSkillsIntegrationsAndResources(t *testing.T) {
	db := setupPusherTestDB(t)
	encKey := testPusherEncKey(t)

	org := model.Org{ID: uuid.New(), Name: "inherit-pusher-org-" + uuid.New().String()[:8], Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", org.ID).Delete(&model.Org{}) })

	encrypted, _ := encKey.EncryptString("sk-test-key-for-pusher-inherit")
	cred := model.Credential{
		ID: uuid.New(), OrgID: org.ID,
		ProviderID: "openai", Label: "Test OpenAI",
		EncryptedKey: encrypted, WrappedDEK: []byte("test"),
		BaseURL: "https://api.openai.com", AuthScheme: "bearer",
	}
	if err := db.Create(&cred).Error; err != nil {
		t.Fatalf("create cred: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", cred.ID).Delete(&model.Credential{}) })

	employee := model.Agent{
		ID: uuid.New(), OrgID: &org.ID, CredentialID: &cred.ID,
		Name: "Employee Owner", Model: "gpt-4o",
		SystemPrompt: "employee", Status: "active", IsEmployee: true,
		Integrations: model.JSON{
			"employee-conn": map[string]any{"actions": []any{"issues.read"}},
		},
		Resources: model.JSON{
			"employee-conn": map[string]any{
				"repository": []any{map[string]any{"id": "octo-org/api", "name": "api"}},
			},
		},
	}
	if err := db.Create(&employee).Error; err != nil {
		t.Fatalf("create employee: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", employee.ID).Delete(&model.Agent{}) })

	subagent := model.Agent{
		ID: uuid.New(), OrgID: &org.ID, CredentialID: &cred.ID,
		Name: "Cloud Specialist", Model: "gpt-4o",
		SystemPrompt: "subagent", Status: "active",
		Resources: model.JSON{
			"subagent-conn": map[string]any{
				"repository": []any{map[string]any{"id": "octo-org/worker", "name": "worker"}},
			},
		},
	}
	if err := db.Create(&subagent).Error; err != nil {
		t.Fatalf("create subagent: %v", err)
	}
	if err := db.Create(&model.AgentSubagent{AgentID: employee.ID, SubagentID: subagent.ID}).Error; err != nil {
		t.Fatalf("create subagent link: %v", err)
	}
	profile := model.AgentProfile{
		ID:         uuid.New(),
		OrgID:      org.ID,
		AgentID:    employee.ID,
		Provider:   githubprofile.Provider,
		ExternalID: "octocat",
		Label:      "octocat",
		Config: model.JSON{
			"selected_repositories": []any{
				map[string]any{"id": int64(1), "name": "selected-api", "full_name": "octo-org/selected-api"},
			},
		},
		Status: "active",
	}
	if err := db.Create(&profile).Error; err != nil {
		t.Fatalf("create github profile: %v", err)
	}
	t.Cleanup(func() {
		db.Where("agent_id = ? AND subagent_id = ?", employee.ID, subagent.ID).Delete(&model.AgentSubagent{})
		db.Where("id = ?", profile.ID).Delete(&model.AgentProfile{})
		db.Where("id = ?", subagent.ID).Delete(&model.Agent{})
	})

	employeeSkill := createPusherSkill(t, db, "linear")
	subagentSkill := createPusherSkill(t, db, "asset-uploads")
	if err := db.Create(&model.AgentSkill{AgentID: employee.ID, SkillID: employeeSkill.ID}).Error; err != nil {
		t.Fatalf("link employee skill: %v", err)
	}
	if err := db.Create(&model.AgentSkill{AgentID: subagent.ID, SkillID: subagentSkill.ID}).Error; err != nil {
		t.Fatalf("link subagent skill: %v", err)
	}

	pusher := NewPusher(db, nil, []byte("test-signing-key-for-pusher-inherit"), &config.Config{
		ProxyHost:  "proxy.test.com",
		MCPBaseURL: "https://mcp.test.com",
	}, nil)
	def := pusher.buildAgentDefinition(t.Context(), &subagent, &employee, &cred, "ptok_inherit", uuid.New().String())

	if def.McpServers == nil {
		t.Fatal("mcp_servers should be injected from inherited employee integrations")
	}
	if def.Skills == nil || len(*def.Skills) != 2 {
		t.Fatalf("skills = %#v, want employee and subagent skills", def.Skills)
	}
	var titles []string
	for _, skill := range *def.Skills {
		titles = append(titles, skill.Title)
	}
	assertSliceContains(t, "skills", titles, "linear")
	assertSliceContains(t, "skills", titles, "asset-uploads")
	assertContains(t, "system_prompt inherited employee repo", def.SystemPrompt, "octo-org/api")
	assertContains(t, "system_prompt subagent repo", def.SystemPrompt, "octo-org/worker")
	assertContains(t, "system_prompt selected employee repo", def.SystemPrompt, "octo-org/selected-api")
	assertContains(t, "system_prompt selected employee repo path", def.SystemPrompt, "/workspace/repos/selected-api")
}
