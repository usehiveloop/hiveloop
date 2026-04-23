package sandbox

import (
	"context"
	"testing"

	"github.com/usehiveloop/hiveloop/internal/model"
)

func TestAssign_EmptyPool_CreatesAndPersistsSandbox(t *testing.T) {
	orch, provider, db := setupOrchestrator(t)
	org := createTestOrg(t, db)
	cred := createTestCred(t, db, org.ID)
	agent1 := createTestAgent(t, db, org.ID, cred.ID, "shared")
	agent2 := createTestAgent(t, db, org.ID, cred.ID, "shared")

	ctx := context.Background()
	sb, err := orch.AssignPoolSandbox(ctx, &agent1)
	if err != nil {
		t.Fatalf("AssignPoolSandbox: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", sb.ID).Delete(&model.Sandbox{}) })

	if provider.count() != 1 {
		t.Fatalf("provider should have created 1 sandbox, got %d", provider.count())
	}

	var persisted model.Sandbox
	if err := db.Where("id = ?", sb.ID).First(&persisted).Error; err != nil {
		t.Fatalf("sandbox should be persisted in DB: %v", err)
	}
	if persisted.SandboxType != "shared" {
		t.Errorf("persisted type: got %q, want shared", persisted.SandboxType)
	}
	if persisted.OrgID != nil {
		t.Error("persisted sandbox should have nil OrgID (pool sandbox)")
	}
	if persisted.Status != "running" {
		t.Errorf("persisted status: got %q, want running", persisted.Status)
	}
	if persisted.ExternalID == "" {
		t.Error("persisted sandbox should have an external_id from the provider")
	}
	if persisted.BridgeURL == "" {
		t.Error("persisted sandbox should have a bridge_url")
	}
	if persisted.BridgeURLExpiresAt == nil {
		t.Error("persisted sandbox should have bridge_url_expires_at set")
	}
	if len(persisted.EncryptedBridgeAPIKey) == 0 {
		t.Error("persisted sandbox should have encrypted bridge API key")
	}
	if persisted.LastActiveAt == nil {
		t.Error("persisted sandbox should have last_active_at set")
	}

	var a1 model.Agent
	db.Where("id = ?", agent1.ID).First(&a1)
	if a1.SandboxID == nil || *a1.SandboxID != sb.ID {
		t.Error("agent1 should be assigned to the new sandbox")
	}

	sb2, err := orch.AssignPoolSandbox(ctx, &agent2)
	if err != nil {
		t.Fatalf("second AssignPoolSandbox: %v", err)
	}
	if sb2.ID != sb.ID {
		t.Errorf("second agent should reuse the auto-provisioned sandbox: got %s, want %s", sb2.ID, sb.ID)
	}
	if provider.count() != 1 {
		t.Errorf("provider should still have 1 sandbox (reused), got %d", provider.count())
	}
}
