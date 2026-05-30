package tasks

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hibiken/asynq"

	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
)

func TestEmployeeSandboxUpgradeWorker_SucceedsAndSchedulesOldRetirement(t *testing.T) {
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
	var old model.Sandbox
	if err := f.db.First(&old, "id = ?", f.old.ID).Error; err != nil {
		t.Fatalf("load old sandbox: %v", err)
	}
	if old.Status != string(sandbox.StatusStopped) {
		t.Fatalf("old status = %s, want stopped", old.Status)
	}
	if len(f.provider.deleted) != 0 {
		t.Fatalf("old sandbox deleted immediately: %v", f.provider.deleted)
	}
	if upgrade.NewSandboxID == nil {
		t.Fatalf("new sandbox id not recorded")
	}
	task := requireRetireTask(t, f.enqueuer)
	var payload EmployeeSandboxRetirePayload
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		t.Fatalf("unmarshal retire payload: %v", err)
	}
	if payload.UpgradeID != f.upgrade.ID || payload.EmployeeID != f.agent.ID || payload.SandboxID != f.old.ID {
		t.Fatalf("retire payload = %#v", payload)
	}
}

func TestEmployeeSandboxUpgradeWorker_CreateFailureLeavesOldRunning(t *testing.T) {
	f := newEmployeeUpgradeFixture(t)
	f.provider.failCreate = true
	if err := f.handler.Handle(context.Background(), employeeUpgradeTask(t, f.upgrade.ID, f.agent.ID)); err == nil {
		t.Fatal("expected create failure")
	}
	var upgrade model.EmployeeSandboxUpgrade
	if err := f.db.First(&upgrade, "id = ?", f.upgrade.ID).Error; err != nil {
		t.Fatalf("load upgrade: %v", err)
	}
	if upgrade.Status != model.EmployeeSandboxUpgradeStatusFailed || upgrade.Phase != model.EmployeeSandboxUpgradePhaseCreatingNew {
		t.Fatalf("status/phase = %s/%s", upgrade.Status, upgrade.Phase)
	}
	var old model.Sandbox
	if err := f.db.First(&old, "id = ?", f.old.ID).Error; err != nil {
		t.Fatalf("load old sandbox: %v", err)
	}
	if old.Status != string(sandbox.StatusRunning) {
		t.Fatalf("old status = %s, want running", old.Status)
	}
	if len(f.provider.stopped) != 0 {
		t.Fatalf("create failure should not stop old: stopped=%v", f.provider.stopped)
	}
}

func TestEmployeeSandboxRetireHandler_DeletesStoppedOldSandboxAfterDelayTask(t *testing.T) {
	f := newEmployeeUpgradeFixture(t)
	if err := f.handler.Handle(context.Background(), employeeUpgradeTask(t, f.upgrade.ID, f.agent.ID)); err != nil {
		t.Fatalf("handle upgrade: %v", err)
	}
	task := requireRetireTask(t, f.enqueuer)
	retireHandler := NewEmployeeSandboxRetireHandler(f.db, f.handler.orchestrator)
	if err := retireHandler.Handle(context.Background(), asynq.NewTask(task.TypeName, task.Payload)); err != nil {
		t.Fatalf("handle retire: %v", err)
	}
	var oldCount int64
	f.db.Model(&model.Sandbox{}).Where("id = ?", f.old.ID).Count(&oldCount)
	if oldCount != 0 {
		t.Fatalf("old sandbox not deleted: count=%d", oldCount)
	}
}

func TestEmployeeSandboxUpgradeWorker_SetsSnapshotIDFromConfig(t *testing.T) {
	f := newEmployeeUpgradeFixture(t)
	if err := f.handler.Handle(context.Background(), employeeUpgradeTask(t, f.upgrade.ID, f.agent.ID)); err != nil {
		t.Fatalf("handle: %v", err)
	}
	var newSandbox model.Sandbox
	if err := f.db.First(&newSandbox, "id != ? AND employee_id = ?", f.old.ID, f.agent.ID).Error; err != nil {
		t.Fatalf("load new sandbox: %v", err)
	}
	if newSandbox.SnapshotID == nil || *newSandbox.SnapshotID != f.handler.compileDeps.Cfg.SandboxesRuntimeBaseImage {
		t.Fatalf("snapshot id = %v, want %s", newSandbox.SnapshotID, f.handler.compileDeps.Cfg.SandboxesRuntimeBaseImage)
	}
}

func requireRetireTask(t *testing.T, c *enqueue.MockClient) *enqueue.EnqueuedTask {
	t.Helper()
	for _, task := range c.Tasks() {
		if task.TypeName == TypeEmployeeSandboxRetire {
			return &task
		}
	}
	t.Fatal("retire task not enqueued")
	return nil
}
