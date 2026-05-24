package sandbox

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func TestRunSandboxLifecycle_SkipsPersistentEmployees(t *testing.T) {
	orch, provider, db := setupOrchestrator(t)
	org := createTestOrg(t, db)
	cred := createTestCred(t, db, org.ID)

	employeeAgent := newAgentWithHarness(t, db, org.ID, cred.ID, "employee-sandbox")
	claudeAgent := newAgentWithHarness(t, db, org.ID, cred.ID, "claude")

	idleAt := time.Now().Add(-30 * time.Minute)
	employeeSb := newRunningSandbox(t, db, &org.ID, &employeeAgent.ID, "ext-employee", idleAt)
	claudeSb := newRunningSandbox(t, db, &org.ID, &claudeAgent.ID, "ext-claude", idleAt)
	provider.registerSandbox("ext-employee", StatusRunning)
	provider.registerSandbox("ext-claude", StatusRunning)

	orch.RunSandboxLifecycle(context.Background())

	var employeeAfter, claudeAfter model.Sandbox
	db.Where("id = ?", employeeSb.ID).First(&employeeAfter)
	db.Where("id = ?", claudeSb.ID).First(&claudeAfter)

	if employeeAfter.Status != string(StatusRunning) {
		t.Errorf("employee sandbox status = %q, want running (excluded from idle stop)", employeeAfter.Status)
	}
	if claudeAfter.Status == string(StatusRunning) {
		t.Errorf("claude sandbox should have been stopped; status = %q", claudeAfter.Status)
	}
}

func TestRunSandboxLifecycle_SkipsPersistentEmployeesFromArchive(t *testing.T) {
	orch, provider, db := setupOrchestrator(t)
	org := createTestOrg(t, db)
	cred := createTestCred(t, db, org.ID)

	employeeAgent := newAgentWithHarness(t, db, org.ID, cred.ID, "employee-sandbox")
	claudeAgent := newAgentWithHarness(t, db, org.ID, cred.ID, "claude")

	stoppedAt := time.Now().Add(-30 * time.Hour)
	employeeSb := newStoppedSandbox(t, db, &org.ID, &employeeAgent.ID, "ext-e-stop", stoppedAt)
	claudeSb := newStoppedSandbox(t, db, &org.ID, &claudeAgent.ID, "ext-c-stop", stoppedAt)
	provider.registerSandbox("ext-e-stop", StatusStopped)
	provider.registerSandbox("ext-c-stop", StatusStopped)

	orch.RunSandboxLifecycle(context.Background())

	var employeeAfter, claudeAfter model.Sandbox
	db.Where("id = ?", employeeSb.ID).First(&employeeAfter)
	db.Where("id = ?", claudeSb.ID).First(&claudeAfter)

	if employeeAfter.Status != string(StatusStopped) {
		t.Errorf("employee sandbox status = %q, want stopped (excluded from archive)", employeeAfter.Status)
	}
	if claudeAfter.Status == string(StatusStopped) {
		t.Errorf("claude sandbox should have been archived; status = %q", claudeAfter.Status)
	}
}

func newAgentWithHarness(t *testing.T, db *gorm.DB, orgID, credID uuid.UUID, harness string) model.Employee {
	t.Helper()
	a := model.Employee{
		OrgID: &orgID, Name: "agent-" + uuid.NewString()[:8],
		CredentialID: &credID, SystemPrompt: "x", Model: "y",
		Harness: harness, IsEmployee: harness == "employee-sandbox",
		Status: "active",
	}
	if err := db.Create(&a).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", a.ID).Delete(&model.Employee{}) })
	return a
}

func newRunningSandbox(t *testing.T, db *gorm.DB, orgID, agentID *uuid.UUID, externalID string, lastActiveAt time.Time) model.Sandbox {
	t.Helper()
	sb := model.Sandbox{
		OrgID: orgID, EmployeeID: agentID,
		ExternalID:            externalID,
		BridgeURL:             "https://stub.example",
		EncryptedBridgeAPIKey: []byte("enc"),
		Status:                string(StatusRunning),
		LastActiveAt:          &lastActiveAt,
	}
	if err := db.Create(&sb).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", sb.ID).Delete(&model.Sandbox{}) })
	return sb
}

func newStoppedSandbox(t *testing.T, db *gorm.DB, orgID, agentID *uuid.UUID, externalID string, stoppedAt time.Time) model.Sandbox {
	t.Helper()
	sb := model.Sandbox{
		OrgID: orgID, EmployeeID: agentID,
		ExternalID:            externalID,
		BridgeURL:             "https://stub.example",
		EncryptedBridgeAPIKey: []byte("enc"),
		Status:                string(StatusStopped),
		StoppedAt:             &stoppedAt,
	}
	if err := db.Create(&sb).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", sb.ID).Delete(&model.Sandbox{}) })
	return sb
}
