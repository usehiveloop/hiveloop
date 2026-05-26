package handler_test

import (
	"testing"

	"github.com/usehivy/hivy/internal/model"
)

func TestEmployeeHandlerSyncOrgHivyEmployee_CreatesSandboxAndPushesConfig(t *testing.T) {
	h := newEmployeeHarness(t)
	m := h.createOrg(t)
	agent := h.seedEmployeeAgent(t, m)

	if err := h.handler.SyncOrgHivyEmployee(t.Context(), m.org.ID); err != nil {
		t.Fatalf("SyncOrgHivyEmployee: %v", err)
	}

	configCalls, configBearer := h.sidecar.snapshot()
	if configCalls != 1 {
		t.Fatalf("config sync calls = %d, want 1", configCalls)
	}
	if configBearer == "" {
		t.Fatal("config sync bearer should be set")
	}
	envCalls, envBearer := h.sidecar.snapshotRuntime()
	if envCalls != 1 {
		t.Fatalf("runtime env sync calls = %d, want 1", envCalls)
	}
	if envBearer == "" {
		t.Fatal("runtime env bearer should be set")
	}

	var sb model.Sandbox
	if err := h.db.Where("org_id = ? AND employee_id = ? AND status = ?", m.org.ID, agent.ID, "running").First(&sb).Error; err != nil {
		t.Fatalf("load running sandbox: %v", err)
	}
	if sb.ExternalID == "" || sb.RuntimeURL == "" {
		t.Fatalf("sandbox missing provider fields: %#v", sb)
	}
}
