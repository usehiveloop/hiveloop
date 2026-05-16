package sandbox

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/config"
	"github.com/usehiveloop/hiveloop/internal/employeeruntime"
	"github.com/usehiveloop/hiveloop/internal/model"
	githubprofile "github.com/usehiveloop/hiveloop/internal/profiles/github"
)

func TestEmployeeSandboxEnvVars_ProvidesDriveUploadURLAndBearer(t *testing.T) {
	agentID := uuid.New()
	orgID := uuid.New()
	sb := &model.Sandbox{ID: uuid.New()}
	agent := &model.Agent{ID: agentID, Name: "Aria"}

	env := employeeSandboxEnvVars(&config.Config{BridgeHost: "api.example.test"}, "runtime-secret", sb, orgID, agent, &employeeruntime.StartupSecrets{
		SlackBotToken: "xoxb-test",
		SlackAppToken: "xapp-test",
		ProxyToken:    "ptok-test",
	}, nil)

	want := "https://api.example.test/internal/employees/" + agentID.String() + "/assets/employee"
	if env["HIVELOOP_DRIVE_UPLOAD_URL"] != want {
		t.Fatalf("HIVELOOP_DRIVE_UPLOAD_URL = %q, want %q", env["HIVELOOP_DRIVE_UPLOAD_URL"], want)
	}
	if env["UPLOAD_BEARER"] != "runtime-secret" {
		t.Fatalf("UPLOAD_BEARER = %q, want runtime-secret", env["UPLOAD_BEARER"])
	}
	if env["HIVELOOP_GIT_USERNAME"] != "aria" {
		t.Fatalf("HIVELOOP_GIT_USERNAME = %q, want aria", env["HIVELOOP_GIT_USERNAME"])
	}
	if env["HIVELOOP_GIT_EMAIL"] != "aria@users.noreply.github.com" {
		t.Fatalf("HIVELOOP_GIT_EMAIL = %q, want aria@users.noreply.github.com", env["HIVELOOP_GIT_EMAIL"])
	}
	if env["BUGSINK_URL"] != "https://api.example.test/internal/bugsink-proxy/"+agentID.String() {
		t.Fatalf("BUGSINK_URL = %q", env["BUGSINK_URL"])
	}
	if env["BUGSINK_TOKEN"] != "runtime-secret" {
		t.Fatalf("BUGSINK_TOKEN = %q, want runtime-secret", env["BUGSINK_TOKEN"])
	}
}

func TestEmployeeSandboxEnvVars_UsesEncryptedGitHubProfileIdentity(t *testing.T) {
	orch, _, db := setupOrchestrator(t)
	org := createTestOrg(t, db)
	cred := createTestCred(t, db, org.ID)
	agent := createTestAgent(t, db, org.ID, cred.ID)

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
		AgentID:           agent.ID,
		Provider:          "github",
		ExternalID:        "octocat",
		Label:             "octocat",
		Identity:          model.JSON{"email": "plaintext@example.com"},
		EncryptedIdentity: encryptedIdentity,
		Config:            model.JSON{},
		Status:            "active",
	}
	if err := db.Create(&profile).Error; err != nil {
		t.Fatalf("create github profile: %v", err)
	}

	gitIdentity, err := orch.loadEmployeeGitIdentity(context.Background(), &agent)
	if err != nil {
		t.Fatalf("load git identity: %v", err)
	}
	env := employeeSandboxEnvVars(&config.Config{BridgeHost: "api.example.test"}, "runtime-secret", &model.Sandbox{ID: uuid.New()}, org.ID, &agent, &employeeruntime.StartupSecrets{
		SlackBotToken: "xoxb-test",
		SlackAppToken: "xapp-test",
		ProxyToken:    "ptok-test",
	}, gitIdentity)

	if env["HIVELOOP_GIT_USERNAME"] != "The Octocat" {
		t.Fatalf("HIVELOOP_GIT_USERNAME = %q, want The Octocat", env["HIVELOOP_GIT_USERNAME"])
	}
	if env["HIVELOOP_GIT_EMAIL"] != "octocat@example.com" {
		t.Fatalf("HIVELOOP_GIT_EMAIL = %q, want octocat@example.com", env["HIVELOOP_GIT_EMAIL"])
	}
}

func TestEmployeeSandboxEnvVars_UsesGitHubNoreplyWhenProfileEmailMissing(t *testing.T) {
	orch, _, db := setupOrchestrator(t)
	org := createTestOrg(t, db)
	cred := createTestCred(t, db, org.ID)
	agent := createTestAgent(t, db, org.ID, cred.ID)

	encryptedIdentity, err := githubprofile.EncryptIdentity(orch.encKey, model.JSON{
		"id":    float64(12345),
		"login": "octocat",
		"name":  "The Octocat",
	})
	if err != nil {
		t.Fatalf("encrypt identity: %v", err)
	}
	profile := model.AgentProfile{
		ID:                uuid.New(),
		OrgID:             org.ID,
		AgentID:           agent.ID,
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

	gitIdentity, err := orch.loadEmployeeGitIdentity(context.Background(), &agent)
	if err != nil {
		t.Fatalf("load git identity: %v", err)
	}
	env := employeeSandboxEnvVars(&config.Config{BridgeHost: "api.example.test"}, "runtime-secret", &model.Sandbox{ID: uuid.New()}, org.ID, &agent, &employeeruntime.StartupSecrets{
		SlackBotToken: "xoxb-test",
		SlackAppToken: "xapp-test",
		ProxyToken:    "ptok-test",
	}, gitIdentity)

	if env["HIVELOOP_GIT_EMAIL"] != "12345+octocat@users.noreply.github.com" {
		t.Fatalf("HIVELOOP_GIT_EMAIL = %q, want GitHub noreply", env["HIVELOOP_GIT_EMAIL"])
	}
}

func TestCreateDedicatedSandboxWithEnv_CapabilityEnvOverridesUserEnv(t *testing.T) {
	orch, provider, db := setupOrchestrator(t)
	org := createTestOrg(t, db)
	cred := createTestCred(t, db, org.ID)
	agent := createTestAgent(t, db, org.ID, cred.ID)
	encryptedEnv, err := orch.encKey.EncryptString(`{"HIVELOOP_DRIVE_UPLOAD_URL":"https://attacker.test/assets"}`)
	if err != nil {
		t.Fatalf("encrypt env: %v", err)
	}
	agent.EncryptedEnvVars = encryptedEnv
	if err := db.Save(&agent).Error; err != nil {
		t.Fatalf("save agent env: %v", err)
	}

	_, err = orch.CreateDedicatedSandboxWithEnv(context.Background(), &agent, map[string]string{
		"HIVELOOP_DRIVE_UPLOAD_URL": "https://api.example.test/internal/employees/emp/assets/tasks/task",
	})
	if err != nil {
		t.Fatalf("CreateDedicatedSandboxWithEnv: %v", err)
	}

	got := provider.createCalls[len(provider.createCalls)-1].EnvVars["HIVELOOP_DRIVE_UPLOAD_URL"]
	want := "https://api.example.test/internal/employees/emp/assets/tasks/task"
	if got != want {
		t.Fatalf("HIVELOOP_DRIVE_UPLOAD_URL = %q, want %q", got, want)
	}
}

func TestCreateDedicatedSandboxWithEnv_PlatformUploadBearerOverridesUserEnv(t *testing.T) {
	orch, provider, db := setupOrchestrator(t)
	org := createTestOrg(t, db)
	cred := createTestCred(t, db, org.ID)
	agent := createTestAgent(t, db, org.ID, cred.ID)
	encryptedEnv, err := orch.encKey.EncryptString(`{"UPLOAD_BEARER":"attacker-token"}`)
	if err != nil {
		t.Fatalf("encrypt env: %v", err)
	}
	agent.EncryptedEnvVars = encryptedEnv
	if err := db.Save(&agent).Error; err != nil {
		t.Fatalf("save agent env: %v", err)
	}

	_, err = orch.CreateDedicatedSandboxWithEnv(context.Background(), &agent, map[string]string{})
	if err != nil {
		t.Fatalf("CreateDedicatedSandboxWithEnv: %v", err)
	}

	env := provider.createCalls[len(provider.createCalls)-1].EnvVars
	if env["UPLOAD_BEARER"] == "" {
		t.Fatal("UPLOAD_BEARER should be set")
	}
	if env["UPLOAD_BEARER"] == "attacker-token" {
		t.Fatalf("UPLOAD_BEARER should not be user-overridable")
	}
	if env["UPLOAD_BEARER"] != env["BRIDGE_CONTROL_PLANE_API_KEY"] {
		t.Fatalf("UPLOAD_BEARER = %q, want bridge api key %q", env["UPLOAD_BEARER"], env["BRIDGE_CONTROL_PLANE_API_KEY"])
	}
}
