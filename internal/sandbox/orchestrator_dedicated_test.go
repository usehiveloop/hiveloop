package sandbox

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/model"
	githubprofile "github.com/usehiveloop/hiveloop/internal/profiles/github"
)

func TestCreateDedicatedSandbox(t *testing.T) {
	orch, provider, db := setupOrchestrator(t)
	orch.cfg.Environment = "production"
	orch.cfg.SentryDSN = "https://backend@example.com/2"
	orch.cfg.AgentSandboxSentryDSN = "https://agent@example.com/3"
	orch.cfg.SentryRelease = "agent-bridge@test"
	orch.cfg.SentryTracesSampleRate = 0.35
	org := createTestOrg(t, db)
	cred := createTestCred(t, db, org.ID)
	agent := createTestAgent(t, db, org.ID, cred.ID)

	ctx := context.Background()
	sb, err := orch.CreateDedicatedSandbox(ctx, &agent)
	if err != nil {
		t.Fatalf("CreateDedicatedSandbox: %v", err)
	}
	t.Cleanup(func() {
		db.Where("id = ?", sb.ID).Delete(&model.Sandbox{})
		db.Where("org_id = ?", org.ID).Delete(&model.WorkspaceStorage{})
	})

	if sb.AgentID == nil || *sb.AgentID != agent.ID {
		t.Error("agent_id should be set")
	}
	if sb.OrgID == nil || *sb.OrgID != org.ID {
		t.Error("org_id should be set")
	}
	if sb.Status != "running" {
		t.Errorf("status: got %q", sb.Status)
	}
	if provider.count() != 1 {
		t.Errorf("expected 1 sandbox, got %d", provider.count())
	}
	env := provider.createCalls[len(provider.createCalls)-1].EnvVars
	if got := env["SENTRY_DSN"]; got != "https://agent@example.com/3" {
		t.Fatalf("SENTRY_DSN = %q, want agent sandbox DSN", got)
	}
	if got := env["SENTRY_ENVIRONMENT"]; got != "production" {
		t.Fatalf("SENTRY_ENVIRONMENT = %q, want production", got)
	}
	if got := env["SENTRY_RELEASE"]; got != "agent-bridge@test" {
		t.Fatalf("SENTRY_RELEASE = %q, want agent-bridge@test", got)
	}
	if got := env["SENTRY_TRACES_SAMPLE_RATE"]; got != "0.35" {
		t.Fatalf("SENTRY_TRACES_SAMPLE_RATE = %q, want 0.35", got)
	}
	if got := env["SENTRY_ENABLE_LOGS"]; got != "true" {
		t.Fatalf("SENTRY_ENABLE_LOGS = %q, want true", got)
	}
}

func TestCreateDedicatedSandbox_InheritsEmployeeEnvWithSubagentOverrides(t *testing.T) {
	orch, provider, db := setupOrchestrator(t)
	org := createTestOrg(t, db)
	cred := createTestCred(t, db, org.ID)
	employee := createTestAgent(t, db, org.ID, cred.ID)
	employee.IsEmployee = true
	employee.EncryptedEnvVars = encryptedEnvVars(t, orch, map[string]string{
		"SHARED_ENV":  "from-employee",
		"ONLY_PARENT": "parent-value",
		"BRIDGE_SKIP": "must-not-leak",
	})
	if err := db.Save(&employee).Error; err != nil {
		t.Fatalf("save employee: %v", err)
	}
	subagent := createTestAgent(t, db, org.ID, cred.ID)
	subagent.EncryptedEnvVars = encryptedEnvVars(t, orch, map[string]string{
		"SHARED_ENV": "from-subagent",
		"ONLY_CHILD": "child-value",
	})
	if err := db.Save(&subagent).Error; err != nil {
		t.Fatalf("save subagent: %v", err)
	}
	if err := db.Create(&model.AgentSubagent{AgentID: employee.ID, SubagentID: subagent.ID}).Error; err != nil {
		t.Fatalf("create agent subagent link: %v", err)
	}

	sb, err := orch.CreateDedicatedSandbox(context.Background(), &subagent)
	if err != nil {
		t.Fatalf("CreateDedicatedSandbox: %v", err)
	}
	t.Cleanup(func() {
		db.Where("id = ?", sb.ID).Delete(&model.Sandbox{})
		db.Where("org_id = ?", org.ID).Delete(&model.WorkspaceStorage{})
	})

	env := provider.createCalls[len(provider.createCalls)-1].EnvVars
	if got := env["ONLY_PARENT"]; got != "parent-value" {
		t.Fatalf("ONLY_PARENT = %q, want parent-value", got)
	}
	if got := env["ONLY_CHILD"]; got != "child-value" {
		t.Fatalf("ONLY_CHILD = %q, want child-value", got)
	}
	if got := env["SHARED_ENV"]; got != "from-subagent" {
		t.Fatalf("SHARED_ENV = %q, want subagent override", got)
	}
	if _, ok := env["BRIDGE_SKIP"]; ok {
		t.Fatalf("BRIDGE_SKIP should not be inherited into cloud agent env")
	}
}

func TestCreateDedicatedSandbox_InheritsEmployeeResourcesForRepositoryClone(t *testing.T) {
	orch, provider, db := setupOrchestrator(t)
	org := createTestOrg(t, db)
	cred := createTestCred(t, db, org.ID)
	employee := createTestAgent(t, db, org.ID, cred.ID)
	employee.IsEmployee = true
	employee.Resources = model.JSON{
		"employee-github": map[string]any{
			"repository": []any{
				map[string]any{"id": "octo-org/api", "name": "api"},
			},
		},
	}
	if err := db.Save(&employee).Error; err != nil {
		t.Fatalf("save employee: %v", err)
	}
	subagent := createTestAgent(t, db, org.ID, cred.ID)
	subagent.Resources = model.JSON{
		"subagent-github": map[string]any{
			"repository": []any{
				map[string]any{"id": "octo-org/worker", "name": "worker"},
			},
		},
	}
	if err := db.Save(&subagent).Error; err != nil {
		t.Fatalf("save subagent: %v", err)
	}
	if err := db.Create(&model.AgentSubagent{AgentID: employee.ID, SubagentID: subagent.ID}).Error; err != nil {
		t.Fatalf("create agent subagent link: %v", err)
	}

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

	assertCommandContains(t, commands, "git clone --depth=1 https://github.com/octo-org/api.git /home/daytona/repos/api")
	assertCommandContains(t, commands, "git clone --depth=1 https://github.com/octo-org/worker.git /home/daytona/repos/worker")
}

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

func TestCreateDedicatedSandbox_InheritsGitIdentityFromEmployeeProfile(t *testing.T) {
	orch, provider, db := setupOrchestrator(t)
	org := createTestOrg(t, db)
	cred := createTestCred(t, db, org.ID)
	employee := createTestAgent(t, db, org.ID, cred.ID)
	employee.IsEmployee = true
	employee.Name = "Employee Owner"
	if err := db.Save(&employee).Error; err != nil {
		t.Fatalf("save employee: %v", err)
	}
	subagent := createTestAgent(t, db, org.ID, cred.ID)

	link := model.AgentSubagent{AgentID: employee.ID, SubagentID: subagent.ID}
	if err := db.Create(&link).Error; err != nil {
		t.Fatalf("create agent subagent link: %v", err)
	}
	t.Cleanup(func() {
		db.Where("agent_id = ? AND subagent_id = ?", employee.ID, subagent.ID).Delete(&model.AgentSubagent{})
	})

	encryptedIdentity, err := githubprofile.EncryptIdentity(orch.encKey, model.JSON{
		"name":  "The Octocat",
		"login": "octocat",
		"email": "octocat@example.com",
	})
	if err != nil {
		t.Fatalf("encrypt identity: %v", err)
	}
	profile := model.AgentProfile{
		ID:                uuid.New(),
		OrgID:             org.ID,
		AgentID:           employee.ID,
		Provider:          "github",
		ExternalID:        "octocat",
		Label:             "octocat",
		Identity:          model.JSON{},
		EncryptedIdentity: encryptedIdentity,
		Config:            model.JSON{},
		Status:            "active",
	}
	if err := db.Create(&profile).Error; err != nil {
		t.Fatalf("create github profile: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", profile.ID).Delete(&model.AgentProfile{}) })

	sb, err := orch.CreateDedicatedSandbox(context.Background(), &subagent)
	if err != nil {
		t.Fatalf("CreateDedicatedSandbox: %v", err)
	}
	t.Cleanup(func() {
		db.Where("id = ?", sb.ID).Delete(&model.Sandbox{})
		db.Where("org_id = ?", org.ID).Delete(&model.WorkspaceStorage{})
	})

	env := provider.createCalls[len(provider.createCalls)-1].EnvVars
	if env["HIVELOOP_GIT_USERNAME"] != "The Octocat" {
		t.Fatalf("HIVELOOP_GIT_USERNAME = %q, want The Octocat", env["HIVELOOP_GIT_USERNAME"])
	}
	if env["HIVELOOP_GIT_EMAIL"] != "octocat@example.com" {
		t.Fatalf("HIVELOOP_GIT_EMAIL = %q, want octocat@example.com", env["HIVELOOP_GIT_EMAIL"])
	}
}

func encryptedEnvVars(t *testing.T, orch *Orchestrator, vars map[string]string) []byte {
	t.Helper()
	raw, err := json.Marshal(vars)
	if err != nil {
		t.Fatalf("marshal env vars: %v", err)
	}
	encrypted, err := orch.encKey.EncryptString(string(raw))
	if err != nil {
		t.Fatalf("encrypt env vars: %v", err)
	}
	return encrypted
}

func assertCommandContains(t *testing.T, commands []string, want string) {
	t.Helper()
	for _, command := range commands {
		if command == want {
			return
		}
	}
	t.Fatalf("commands = %#v, missing %q", commands, want)
}
