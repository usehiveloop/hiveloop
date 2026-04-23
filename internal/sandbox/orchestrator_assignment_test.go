package sandbox

import (
	"context"
	"testing"

	"github.com/usehiveloop/hiveloop/internal/model"
)

func TestAssign_AgentWithExistingSandbox_ReturnsIt(t *testing.T) {
	orch, provider, db := setupOrchestrator(t)
	org := createTestOrg(t, db)
	cred := createTestCred(t, db, org.ID)
	agent := createTestAgent(t, db, org.ID, cred.ID, "shared")

	sbExisting := seedSharedSandbox(t, db, 0, 0)
	sbOther := seedSharedSandbox(t, db, 0, 0)
	_ = sbOther

	provider.registerSandbox(sbExisting.ExternalID, StatusRunning)

	db.Model(&agent).Update("sandbox_id", sbExisting.ID)
	agent.SandboxID = &sbExisting.ID

	ctx := context.Background()
	picked, err := orch.AssignPoolSandbox(ctx, &agent)
	if err != nil {
		t.Fatalf("AssignPoolSandbox: %v", err)
	}

	if picked.ID != sbExisting.ID {
		t.Errorf("should return existing assignment: got %s, want %s", picked.ID, sbExisting.ID)
	}
}

func TestRelease_ClearsAgentSandboxID(t *testing.T) {
	orch, _, db := setupOrchestrator(t)
	org := createTestOrg(t, db)
	cred := createTestCred(t, db, org.ID)
	agent := createTestAgent(t, db, org.ID, cred.ID, "shared")

	sb := seedSharedSandbox(t, db, 0, 0)
	db.Model(&agent).Update("sandbox_id", sb.ID)
	agent.SandboxID = &sb.ID

	ctx := context.Background()
	if err := orch.ReleasePoolSandbox(ctx, &agent); err != nil {
		t.Fatalf("release: %v", err)
	}

	var reloaded model.Agent
	db.Where("id = ?", agent.ID).First(&reloaded)
	if reloaded.SandboxID != nil {
		t.Error("agent.SandboxID should be nil after release")
	}
}

func TestRelease_NilSandboxID_Noop(t *testing.T) {
	orch, _, db := setupOrchestrator(t)
	org := createTestOrg(t, db)
	cred := createTestCred(t, db, org.ID)
	agent := createTestAgent(t, db, org.ID, cred.ID, "shared")

	ctx := context.Background()
	if err := orch.ReleasePoolSandbox(ctx, &agent); err != nil {
		t.Fatalf("release with nil SandboxID should be noop: %v", err)
	}
}
