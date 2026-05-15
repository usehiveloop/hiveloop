package tasks

import (
	"context"
	"strings"
	"testing"

	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/sandbox"
)

func TestEmployeeSandboxUpgradeWorker_SucceedsAndDeletesOld(t *testing.T) {
	f := newEmployeeUpgradeFixture(t)
	if err := f.handler.Handle(context.Background(), employeeUpgradeTask(t, f.upgrade.ID, f.agent.ID)); err != nil {
		t.Fatalf("handle: %v", err)
	}
	var upgrade model.EmployeeSandboxUpgrade
	if err := f.db.First(&upgrade, "id = ?", f.upgrade.ID).Error; err != nil {
		t.Fatalf("load upgrade: %v", err)
	}
	if upgrade.Status != model.EmployeeSandboxUpgradeStatusSucceeded || upgrade.Phase != model.EmployeeSandboxUpgradePhaseCompleted {
		t.Fatalf("status/phase = %s/%s", upgrade.Status, upgrade.Phase)
	}
	if upgrade.BackupKey == nil || !strings.Contains(*upgrade.BackupKey, "/upgrades/"+f.upgrade.ID.String()+".db.gz") {
		t.Fatalf("backup key not recorded: %#v", upgrade.BackupKey)
	}
	var oldCount int64
	f.db.Model(&model.Sandbox{}).Where("id = ?", f.old.ID).Count(&oldCount)
	if oldCount != 0 {
		t.Fatalf("old sandbox still exists")
	}
	if upgrade.NewSandboxID == nil {
		t.Fatalf("new sandbox id not recorded")
	}
}

func TestEmployeeSandboxUpgradeWorker_BackupFailureLeavesOldRunning(t *testing.T) {
	f := newEmployeeUpgradeFixture(t)
	f.provider.failBackup = true
	if err := f.handler.Handle(context.Background(), employeeUpgradeTask(t, f.upgrade.ID, f.agent.ID)); err == nil {
		t.Fatal("expected backup failure")
	}
	var upgrade model.EmployeeSandboxUpgrade
	if err := f.db.First(&upgrade, "id = ?", f.upgrade.ID).Error; err != nil {
		t.Fatalf("load upgrade: %v", err)
	}
	if upgrade.Status != model.EmployeeSandboxUpgradeStatusFailed || upgrade.Phase != model.EmployeeSandboxUpgradePhaseBackup {
		t.Fatalf("status/phase = %s/%s", upgrade.Status, upgrade.Phase)
	}
	var old model.Sandbox
	if err := f.db.First(&old, "id = ?", f.old.ID).Error; err != nil {
		t.Fatalf("load old sandbox: %v", err)
	}
	if old.Status != string(sandbox.StatusRunning) {
		t.Fatalf("old status = %s, want running", old.Status)
	}
	if len(f.provider.created) != 0 || len(f.provider.stopped) != 0 {
		t.Fatalf("backup failure should not stop or create: stopped=%v created=%v", f.provider.stopped, f.provider.created)
	}
}

func TestEmployeeSandboxUpgradeWorker_RestoreFailureRollsBackToOld(t *testing.T) {
	f := newEmployeeUpgradeFixture(t)
	f.provider.failRestore = true
	if err := f.handler.Handle(context.Background(), employeeUpgradeTask(t, f.upgrade.ID, f.agent.ID)); err == nil {
		t.Fatal("expected restore failure")
	}
	var upgrade model.EmployeeSandboxUpgrade
	if err := f.db.First(&upgrade, "id = ?", f.upgrade.ID).Error; err != nil {
		t.Fatalf("load upgrade: %v", err)
	}
	if upgrade.Status != model.EmployeeSandboxUpgradeStatusFailed || upgrade.Phase != model.EmployeeSandboxUpgradePhaseRestore {
		t.Fatalf("status/phase = %s/%s", upgrade.Status, upgrade.Phase)
	}
	var old model.Sandbox
	if err := f.db.First(&old, "id = ?", f.old.ID).Error; err != nil {
		t.Fatalf("load old sandbox: %v", err)
	}
	if old.Status != string(sandbox.StatusRunning) {
		t.Fatalf("old status = %s, want running", old.Status)
	}
	if len(f.provider.created) != 1 || len(f.provider.deleted) == 0 || len(f.provider.started) == 0 {
		t.Fatalf("restore failure did not rollback: created=%v deleted=%v started=%v", f.provider.created, f.provider.deleted, f.provider.started)
	}
}
