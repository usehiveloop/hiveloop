package sandbox

import (
	"context"
	"testing"
	"time"

	"github.com/usehiveloop/hiveloop/internal/model"
)

func TestHealthCheck_SharedSandboxWithAgents_NotStopped(t *testing.T) {
	orch, _, db := setupOrchestrator(t)
	org := createTestOrg(t, db)
	cred := createTestCred(t, db, org.ID)
	agent := createTestAgent(t, db, org.ID, cred.ID, "shared")

	sb := seedSharedSandbox(t, db, 0, 0)
	db.Model(&agent).Update("sandbox_id", sb.ID)

	old := time.Now().Add(-2 * time.Hour)
	db.Model(&sb).Update("last_active_at", old)
	sb.LastActiveAt = &old

	ctx := context.Background()
	orch.checkSandboxHealth(ctx, &sb)

	var reloaded model.Sandbox
	db.Where("id = ?", sb.ID).First(&reloaded)
	if reloaded.Status != "running" {
		t.Errorf("shared sandbox with agents should stay running, got %q", reloaded.Status)
	}
}

func TestHealthCheck_SharedSandboxEmpty_Stopped(t *testing.T) {
	orch, provider, db := setupOrchestrator(t)
	orch.cfg.PoolSandboxIdleTimeoutMins = 1

	sb := seedSharedSandbox(t, db, 0, 0)
	provider.registerSandbox(sb.ExternalID, StatusRunning)

	old := time.Now().Add(-2 * time.Hour)
	db.Model(&sb).Update("last_active_at", old)
	sb.LastActiveAt = &old

	ctx := context.Background()
	orch.checkSandboxHealth(ctx, &sb)

	var reloaded model.Sandbox
	db.Where("id = ?", sb.ID).First(&reloaded)
	if reloaded.Status != "stopped" {
		t.Errorf("empty shared sandbox should be stopped, got %q", reloaded.Status)
	}
}

func TestHealthCheck_SharedSandboxError_UnassignsAgents(t *testing.T) {
	orch, provider, db := setupOrchestrator(t)
	org := createTestOrg(t, db)
	cred := createTestCred(t, db, org.ID)
	agent1 := createTestAgent(t, db, org.ID, cred.ID, "shared")
	agent2 := createTestAgent(t, db, org.ID, cred.ID, "shared")

	sb := seedSharedSandbox(t, db, 0, 0)
	provider.registerSandbox(sb.ExternalID, StatusError)
	db.Model(&agent1).Update("sandbox_id", sb.ID)
	db.Model(&agent2).Update("sandbox_id", sb.ID)

	db.Model(&sb).Update("status", "error")
	sb.Status = "error"

	ctx := context.Background()
	orch.checkSandboxHealth(ctx, &sb)

	var a1, a2 model.Agent
	db.Where("id = ?", agent1.ID).First(&a1)
	db.Where("id = ?", agent2.ID).First(&a2)
	if a1.SandboxID != nil {
		t.Error("agent1 should be unassigned after sandbox error")
	}
	if a2.SandboxID != nil {
		t.Error("agent2 should be unassigned after sandbox error")
	}
}
