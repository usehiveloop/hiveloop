package tasks_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/tasks"
)

// --------------------------------------------------------------------------
// Payload round-trip
// --------------------------------------------------------------------------

func TestBillingUsageEventTask_PayloadRoundTrip(t *testing.T) {
	orgID := uuid.New()
	agentID := uuid.New()
	runID := uuid.New()

	task, err := tasks.NewBillingUsageEventTask(orgID, agentID, runID, "shared")
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	if task.Type() != tasks.TypeBillingUsageEvent {
		t.Fatalf("expected type %q, got %q", tasks.TypeBillingUsageEvent, task.Type())
	}

	var payload tasks.BillingUsageEventPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if payload.OrgID != orgID {
		t.Fatalf("expected org ID %s, got %s", orgID, payload.OrgID)
	}
	if payload.AgentID != agentID {
		t.Fatalf("expected agent ID %s, got %s", agentID, payload.AgentID)
	}
	if payload.RunID != runID {
		t.Fatalf("expected run ID %s, got %s", runID, payload.RunID)
	}
	if payload.SandboxType != "shared" {
		t.Fatalf("expected sandbox type 'shared', got %q", payload.SandboxType)
	}
}

func TestBillingUsageEventTask_DedicatedSandboxType(t *testing.T) {
	task, err := tasks.NewBillingUsageEventTask(uuid.New(), uuid.New(), uuid.New(), "dedicated")
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	var payload tasks.BillingUsageEventPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if payload.SandboxType != "dedicated" {
		t.Fatalf("expected sandbox type 'dedicated', got %q", payload.SandboxType)
	}
}

// --------------------------------------------------------------------------
// Handler — skips free users (no PolarCustomerID)
// --------------------------------------------------------------------------

func TestBillingUsageEventHandler_SkipsFreeUser(t *testing.T) {
	db := connectDB(t)

	// Create org with no PolarCustomerID (free user)
	orgID := uuid.New()
	org := model.Org{
		ID:          orgID,
		Name:        fmt.Sprintf("billing-task-test-%s", uuid.New().String()[:8]),
		Active:      true,
		BillingPlan: "free",
	}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	t.Cleanup(func() {
		db.Where("id = ?", orgID).Delete(&model.Org{})
	})

	// Handler with nil PolarClient — will not attempt to call Polar
	handler := tasks.NewBillingUsageEventHandler(db, nil)

	task, err := tasks.NewBillingUsageEventTask(orgID, uuid.New(), uuid.New(), "shared")
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	// Should return nil (skip) since org has no PolarCustomerID
	if err := handler.Handle(t.Context(), task); err != nil {
		t.Fatalf("expected nil error for free user, got: %v", err)
	}
}

// --------------------------------------------------------------------------
// Handler — errors on missing org
// --------------------------------------------------------------------------

func TestBillingUsageEventHandler_ErrorOnMissingOrg(t *testing.T) {
	db := connectDB(t)

	handler := tasks.NewBillingUsageEventHandler(db, nil)

	task, err := tasks.NewBillingUsageEventTask(uuid.New(), uuid.New(), uuid.New(), "shared")
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	// Should return error since org doesn't exist
	if err := handler.Handle(t.Context(), task); err == nil {
		t.Fatal("expected error for missing org, got nil")
	}
}
