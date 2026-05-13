package sandbox

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/config"
	"github.com/usehiveloop/hiveloop/internal/employeeruntime"
	"github.com/usehiveloop/hiveloop/internal/model"
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
	})

	want := "https://api.example.test/internal/employees/" + agentID.String() + "/assets/employee"
	if env["HIVELOOP_DRIVE_UPLOAD_URL"] != want {
		t.Fatalf("HIVELOOP_DRIVE_UPLOAD_URL = %q, want %q", env["HIVELOOP_DRIVE_UPLOAD_URL"], want)
	}
	if env["UPLOAD_BEARER"] != "runtime-secret" {
		t.Fatalf("UPLOAD_BEARER = %q, want runtime-secret", env["UPLOAD_BEARER"])
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
