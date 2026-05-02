package sandbox

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
)

// createTestSubagent provisions a subagent row + an attachment row tying it
// to a parent agent. Cleanup happens via t.Cleanup — both rows are wiped
// after the test.
func createTestSubagent(t *testing.T, db *gorm.DB, orgID, parentID uuid.UUID) model.Agent {
	t.Helper()
	suffix := uuid.New().String()[:8]
	sub := model.Agent{
		OrgID:        &orgID,
		Name:         "subagent-" + suffix,
		SystemPrompt: "test sub",
		Model:        "gpt-4o",
		AgentType:    model.AgentTypeSubagent,
	}
	if err := db.Create(&sub).Error; err != nil {
		t.Fatalf("create subagent: %v", err)
	}
	link := model.AgentSubagent{AgentID: parentID, SubagentID: sub.ID}
	if err := db.Create(&link).Error; err != nil {
		t.Fatalf("create agent_subagent link: %v", err)
	}
	t.Cleanup(func() {
		db.Where("agent_id = ? AND subagent_id = ?", parentID, sub.ID).Delete(&model.AgentSubagent{})
		db.Where("id = ?", sub.ID).Delete(&model.Agent{})
	})
	return sub
}

func TestEnsureSubagentSandbox_CreatesSeparateSandbox(t *testing.T) {
	orch, provider, db := setupOrchestrator(t)
	org := createTestOrg(t, db)
	cred := createTestCred(t, db, org.ID)
	parent := createTestAgent(t, db, org.ID, cred.ID)

	ctx := context.Background()
	parentSB, err := orch.CreateDedicatedSandbox(ctx, &parent)
	if err != nil {
		t.Fatalf("CreateDedicatedSandbox(parent): %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", parentSB.ID).Delete(&model.Sandbox{}) })

	sub1 := createTestSubagent(t, db, org.ID, parent.ID)
	sub2 := createTestSubagent(t, db, org.ID, parent.ID)

	subSB1, err := orch.EnsureSubagentSandbox(ctx, org.ID, parent.ID, sub1.ID)
	if err != nil {
		t.Fatalf("EnsureSubagentSandbox(sub1): %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", subSB1.ID).Delete(&model.Sandbox{}) })

	subSB2, err := orch.EnsureSubagentSandbox(ctx, org.ID, parent.ID, sub2.ID)
	if err != nil {
		t.Fatalf("EnsureSubagentSandbox(sub2): %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", subSB2.ID).Delete(&model.Sandbox{}) })

	if subSB1.ID == parentSB.ID {
		t.Errorf("subagent sandbox must differ from parent (%s)", parentSB.ID)
	}
	if subSB2.ID == parentSB.ID {
		t.Errorf("subagent sandbox must differ from parent (%s)", parentSB.ID)
	}
	if subSB1.ID == subSB2.ID {
		t.Errorf("each subagent must get a distinct sandbox; got %s twice", subSB1.ID)
	}
	if subSB1.AgentID == nil || *subSB1.AgentID != sub1.ID {
		t.Errorf("subagent sandbox 1 must be keyed to subagent id; got %v", subSB1.AgentID)
	}
	if subSB2.AgentID == nil || *subSB2.AgentID != sub2.ID {
		t.Errorf("subagent sandbox 2 must be keyed to subagent id; got %v", subSB2.AgentID)
	}
	// 3 sandboxes provisioned total: parent + 2 children.
	if provider.count() != 3 {
		t.Errorf("expected 3 provider sandboxes, got %d", provider.count())
	}
}

func TestEnsureSubagentSandbox_Idempotent(t *testing.T) {
	orch, provider, db := setupOrchestrator(t)
	org := createTestOrg(t, db)
	cred := createTestCred(t, db, org.ID)
	parent := createTestAgent(t, db, org.ID, cred.ID)
	sub := createTestSubagent(t, db, org.ID, parent.ID)

	ctx := context.Background()
	first, err := orch.EnsureSubagentSandbox(ctx, org.ID, parent.ID, sub.ID)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", first.ID).Delete(&model.Sandbox{}) })

	second, err := orch.EnsureSubagentSandbox(ctx, org.ID, parent.ID, sub.ID)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}

	if first.ID != second.ID {
		t.Errorf("expected idempotent call to return same sandbox; got %s vs %s", first.ID, second.ID)
	}

	var count int64
	if err := db.Model(&model.Sandbox{}).Where("agent_id = ?", sub.ID).Count(&count).Error; err != nil {
		t.Fatalf("count sandboxes: %v", err)
	}
	if count != 1 {
		t.Errorf("expected exactly 1 sandbox row for subagent; got %d", count)
	}
	if provider.count() != 1 {
		t.Errorf("expected 1 provider sandbox total; got %d", provider.count())
	}
}

func TestEnsureSubagentSandbox_RejectsNonAttached(t *testing.T) {
	orch, _, db := setupOrchestrator(t)
	org := createTestOrg(t, db)
	cred := createTestCred(t, db, org.ID)
	parent := createTestAgent(t, db, org.ID, cred.ID)
	otherParent := createTestAgent(t, db, org.ID, cred.ID)
	sub := createTestSubagent(t, db, org.ID, otherParent.ID)

	ctx := context.Background()
	_, err := orch.EnsureSubagentSandbox(ctx, org.ID, parent.ID, sub.ID)
	if err == nil {
		t.Fatal("expected error when subagent is not attached to caller parent")
	}
}
