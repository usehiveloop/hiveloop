package sandbox

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/model"
	githubprofile "github.com/usehiveloop/hiveloop/internal/profiles/github"
)

func TestCreateDedicatedSandbox_ClonesEmployeeSelectedGitHubRepositories(t *testing.T) {
	orch, provider, db := setupOrchestrator(t)
	org := createTestOrg(t, db)
	cred := createTestCred(t, db, org.ID)
	employee := createTestAgent(t, db, org.ID, cred.ID)
	employee.IsEmployee = true
	if err := db.Save(&employee).Error; err != nil {
		t.Fatalf("save employee: %v", err)
	}
	subagent := createTestAgent(t, db, org.ID, cred.ID)
	if err := db.Create(&model.AgentSubagent{AgentID: employee.ID, SubagentID: subagent.ID}).Error; err != nil {
		t.Fatalf("create agent subagent link: %v", err)
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
				map[string]any{"name": "api", "full_name": "octo-org/api"},
				map[string]any{"name": "web", "full_name": "octo-org/web"},
			},
		},
		Status: "active",
	}
	if err := db.Create(&profile).Error; err != nil {
		t.Fatalf("create github profile: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", profile.ID).Delete(&model.AgentProfile{}) })

	var commands []string
	provider.executeCommandFn = func(_ context.Context, _ string, command string) (string, error) {
		commands = append(commands, command)
		return "", nil
	}

	sb, err := orch.CreateDedicatedSandbox(context.Background(), &subagent)
	if err != nil {
		t.Fatalf("CreateDedicatedSandbox: %v", err)
	}
	t.Cleanup(func() {
		db.Where("id = ?", sb.ID).Delete(&model.Sandbox{})
		db.Where("org_id = ?", org.ID).Delete(&model.WorkspaceStorage{})
	})

	assertCommandContains(t, commands, "mkdir -p /workspace/repos")
	assertCommandContains(t, commands, "git clone --depth=1 https://github.com/octo-org/api.git /workspace/repos/api")
	assertCommandContains(t, commands, "git clone --depth=1 https://github.com/octo-org/web.git /workspace/repos/web")
}
