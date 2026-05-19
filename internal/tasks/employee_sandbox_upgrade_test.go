package tasks

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hibiken/asynq"

	"github.com/usehiveloop/hiveloop/internal/enqueue"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/sandbox"
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
	if upgrade.BackupKey == nil || !strings.Contains(*upgrade.BackupKey, "/upgrades/"+f.upgrade.ID.String()+".db.gz") {
		t.Fatalf("backup key not recorded: %#v", upgrade.BackupKey)
	}
	if len(f.provider.commands) == 0 {
		t.Fatal("backup command was not executed")
	}
	backupCommand := f.provider.commands[0]
	if !strings.Contains(backupCommand, "https://s3.example/upload.db.gz?signature=test") {
		t.Fatalf("backup command did not use presigned upload url: %s", backupCommand)
	}
	if strings.Contains(backupCommand, "CLOUD_CONTROL_PLANE_URL") || strings.Contains(backupCommand, "Authorization: Bearer") {
		t.Fatalf("backup command still routes upload through control plane: %s", backupCommand)
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
	if payload.UpgradeID != f.upgrade.ID || payload.AgentID != f.agent.ID || payload.SandboxID != f.old.ID {
		t.Fatalf("retire payload = %#v", payload)
	}
	requireOption(t, task.Options, asynq.ProcessInOpt, employeeSandboxRetireDelay)
	requireOption(t, task.Options, asynq.QueueOpt, QueueDefault)
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
	if len(f.provider.created) != 1 || len(f.provider.deleted) == 0 || len(f.provider.started) == 0 {
		t.Fatalf("restore failure did not rollback: created=%v deleted=%v started=%v", f.provider.created, f.provider.deleted, f.provider.started)
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
		t.Fatalf("old sandbox still exists after retire task")
	}
}

func TestEmployeeSandboxRetireHandler_IgnoresRunningSandbox(t *testing.T) {
	f := newEmployeeUpgradeFixture(t)
	upgrade := f.upgrade
	upgrade.Status = model.EmployeeSandboxUpgradeStatusSucceeded
	upgrade.Phase = model.EmployeeSandboxUpgradePhaseCompleted
	if err := f.db.Save(&upgrade).Error; err != nil {
		t.Fatalf("save upgrade: %v", err)
	}
	task, _, err := NewEmployeeSandboxRetireTask(EmployeeSandboxRetirePayload{
		UpgradeID: upgrade.ID,
		AgentID:   f.agent.ID,
		SandboxID: f.old.ID,
	})
	if err != nil {
		t.Fatalf("new retire task: %v", err)
	}
	retireHandler := NewEmployeeSandboxRetireHandler(f.db, f.handler.orchestrator)
	if err := retireHandler.Handle(context.Background(), task); err != nil {
		t.Fatalf("handle retire: %v", err)
	}
	var old model.Sandbox
	if err := f.db.First(&old, "id = ?", f.old.ID).Error; err != nil {
		t.Fatalf("load old sandbox: %v", err)
	}
	if old.Status != string(sandbox.StatusRunning) || len(f.provider.deleted) != 0 {
		t.Fatalf("running sandbox should not retire: status=%s deleted=%v", old.Status, f.provider.deleted)
	}
}

func requireRetireTask(t *testing.T, enqueuer *enqueue.MockClient) enqueue.EnqueuedTask {
	t.Helper()
	for _, task := range enqueuer.Tasks() {
		if task.TypeName == TypeEmployeeSandboxRetire {
			return task
		}
	}
	t.Fatalf("expected %s task to be enqueued", TypeEmployeeSandboxRetire)
	return enqueue.EnqueuedTask{}
}

func requireOption(t *testing.T, opts []asynq.Option, typ asynq.OptionType, value any) {
	t.Helper()
	for _, opt := range opts {
		if opt.Type() == typ && opt.Value() == value {
			return
		}
	}
	t.Fatalf("expected option %v=%v in %v", typ, value, opts)
}
