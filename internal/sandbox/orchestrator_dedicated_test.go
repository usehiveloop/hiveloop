package sandbox

import (
	"context"
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
