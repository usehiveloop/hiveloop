package sandbox

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
)

func TestSelection_PicksLowestMemoryUsage(t *testing.T) {
	orch, _, db := setupOrchestrator(t)
	org := createTestOrg(t, db)
	cred := createTestCred(t, db, org.ID)
	agent := createTestAgent(t, db, org.ID, cred.ID, "shared")

	gb := int64(1024 * 1024 * 1024)

	sbHigh := seedSharedSandbox(t, db, 70*gb/100, gb)
	sbLow := seedSharedSandbox(t, db, 20*gb/100, gb)
	sbMid := seedSharedSandbox(t, db, 50*gb/100, gb)
	_ = sbHigh
	_ = sbMid

	ctx := context.Background()
	picked, err := orch.AssignPoolSandbox(ctx, &agent)
	if err != nil {
		t.Fatalf("AssignPoolSandbox: %v", err)
	}

	if picked.ID != sbLow.ID {
		t.Errorf("should pick lowest usage sandbox (20%%): got %s, want %s", picked.ID, sbLow.ID)
	}

	var reloaded model.Agent
	db.Where("id = ?", agent.ID).First(&reloaded)
	if reloaded.SandboxID == nil || *reloaded.SandboxID != sbLow.ID {
		t.Error("agent.SandboxID should point to the lowest-usage sandbox")
	}
}

func TestSelection_SkipsOverThreshold(t *testing.T) {
	orch, _, db := setupOrchestrator(t)
	org := createTestOrg(t, db)
	cred := createTestCred(t, db, org.ID)
	agent := createTestAgent(t, db, org.ID, cred.ID, "shared")

	gb := int64(1024 * 1024 * 1024)

	sbOver := seedSharedSandbox(t, db, 90*gb/100, gb)
	sbUnder := seedSharedSandbox(t, db, 50*gb/100, gb)
	_ = sbOver

	ctx := context.Background()
	picked, err := orch.AssignPoolSandbox(ctx, &agent)
	if err != nil {
		t.Fatalf("AssignPoolSandbox: %v", err)
	}

	if picked.ID != sbUnder.ID {
		t.Errorf("should skip over-threshold sandbox: got %s, want %s", picked.ID, sbUnder.ID)
	}
}

func TestSelection_AllOverThreshold_CreatesNew(t *testing.T) {
	orch, provider, db := setupOrchestrator(t)
	org := createTestOrg(t, db)
	cred := createTestCred(t, db, org.ID)
	agent := createTestAgent(t, db, org.ID, cred.ID, "shared")

	gb := int64(1024 * 1024 * 1024)

	seedSharedSandbox(t, db, 85*gb/100, gb)
	seedSharedSandbox(t, db, 95*gb/100, gb)

	ctx := context.Background()
	picked, err := orch.AssignPoolSandbox(ctx, &agent)
	if err != nil {
		t.Fatalf("AssignPoolSandbox: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", picked.ID).Delete(&model.Sandbox{}) })

	if provider.count() != 1 {
		t.Errorf("provider should have created 1 new sandbox, got %d", provider.count())
	}
	if picked.SandboxType != "shared" {
		t.Errorf("new sandbox type: got %q, want shared", picked.SandboxType)
	}
}

func TestSelection_UnmeasuredSandboxesPreferred(t *testing.T) {
	orch, _, db := setupOrchestrator(t)
	org := createTestOrg(t, db)
	cred := createTestCred(t, db, org.ID)
	agent := createTestAgent(t, db, org.ID, cred.ID, "shared")

	gb := int64(1024 * 1024 * 1024)

	sbMeasured := seedSharedSandbox(t, db, 50*gb/100, gb)
	sbUnmeasured := seedSharedSandbox(t, db, 0, 0)
	_ = sbMeasured

	ctx := context.Background()
	picked, err := orch.AssignPoolSandbox(ctx, &agent)
	if err != nil {
		t.Fatalf("AssignPoolSandbox: %v", err)
	}

	if picked.ID != sbUnmeasured.ID {
		t.Errorf("should prefer unmeasured sandbox: got %s, want %s", picked.ID, sbUnmeasured.ID)
	}
}

func TestSelection_SkipsNonRunningSandboxes(t *testing.T) {
	orch, _, db := setupOrchestrator(t)
	org := createTestOrg(t, db)
	cred := createTestCred(t, db, org.ID)
	agent := createTestAgent(t, db, org.ID, cred.ID, "shared")

	sbStopped := seedSharedSandbox(t, db, 0, 0)
	db.Model(&sbStopped).Update("status", "stopped")

	sbRunning := seedSharedSandbox(t, db, 0, 0)

	ctx := context.Background()
	picked, err := orch.AssignPoolSandbox(ctx, &agent)
	if err != nil {
		t.Fatalf("AssignPoolSandbox: %v", err)
	}

	if picked.ID != sbRunning.ID {
		t.Errorf("should skip stopped sandbox: got %s, want %s", picked.ID, sbRunning.ID)
	}
}

func TestSelection_SkipsDedicatedSandboxes(t *testing.T) {
	orch, _, db := setupOrchestrator(t)
	org := createTestOrg(t, db)
	cred := createTestCred(t, db, org.ID)
	agent := createTestAgent(t, db, org.ID, cred.ID, "shared")

	encKey := testEncKey(t)
	apiKey, _ := generateRandomHex(32)
	encrypted, _ := encKey.EncryptString(apiKey)
	dedicated := model.Sandbox{
		OrgID: &org.ID, SandboxType: "dedicated",
		ExternalID: "ded-" + uuid.New().String()[:8], BridgeURL: "https://mock:25434",
		EncryptedBridgeAPIKey: encrypted, Status: "running",
	}
	db.Create(&dedicated)
	t.Cleanup(func() { db.Where("id = ?", dedicated.ID).Delete(&model.Sandbox{}) })

	sbShared := seedSharedSandbox(t, db, 0, 0)

	ctx := context.Background()
	picked, err := orch.AssignPoolSandbox(ctx, &agent)
	if err != nil {
		t.Fatalf("AssignPoolSandbox: %v", err)
	}

	if picked.ID != sbShared.ID {
		t.Errorf("should only pick shared sandboxes: got %s, want %s", picked.ID, sbShared.ID)
	}
}

func TestSelection_CrossOrg(t *testing.T) {
	orch, _, db := setupOrchestrator(t)

	org1 := createTestOrg(t, db)
	cred1 := createTestCred(t, db, org1.ID)
	agent1 := createTestAgent(t, db, org1.ID, cred1.ID, "shared")

	org2 := createTestOrg(t, db)
	cred2 := createTestCred(t, db, org2.ID)
	agent2 := createTestAgent(t, db, org2.ID, cred2.ID, "shared")

	sbPool := seedSharedSandbox(t, db, 0, 0)

	ctx := context.Background()

	picked1, err := orch.AssignPoolSandbox(ctx, &agent1)
	if err != nil {
		t.Fatalf("org1 assign: %v", err)
	}
	if picked1.ID != sbPool.ID {
		t.Errorf("org1 agent should get pool sandbox: got %s, want %s", picked1.ID, sbPool.ID)
	}

	picked2, err := orch.AssignPoolSandbox(ctx, &agent2)
	if err != nil {
		t.Fatalf("org2 assign: %v", err)
	}
	if picked2.ID != sbPool.ID {
		t.Errorf("org2 agent should get same pool sandbox: got %s, want %s", picked2.ID, sbPool.ID)
	}
}
