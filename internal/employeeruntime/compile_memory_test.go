package employeeruntime

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/hindsight"
	"github.com/usehivy/hivy/internal/model"
)

func TestCompile_PopulatesMemoryContextFromHindsight(t *testing.T) {
	orgID := uuid.New()
	agent := model.Employee{
		ID:    uuid.New(),
		OrgID: &orgID,
		Name:  "Aria",
		Model: DefaultEmployeeModel,
	}
	fake := &fakeMemoryRecall{response: &hindsight.RecallResponse{
		Results: []any{
			map[string]any{
				"content":     "The Platform team requires integration tests for employee-runtime changes.",
				"source":      "manual",
				"memory_type": "technical_context",
				"tags":        []any{"company:" + orgID.String()},
			},
		},
	}}

	def, err := Compile(context.Background(), CompileDeps{Hindsight: fake, Cfg: &config.Config{}}, &agent)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if fake.bankID != "org-"+orgID.String() {
		t.Fatalf("bank id = %q", fake.bankID)
	}
	if len(fake.request.TagGroups) != 1 {
		t.Fatalf("expected strict org tag group, got %#v", fake.request.TagGroups)
	}
	memory, ok := def.Context["memory"].(MemoryContext)
	if !ok {
		t.Fatalf("memory context missing or wrong type: %#v", def.Context["memory"])
	}
	if len(memory.Entries) != 1 {
		t.Fatalf("memory entries = %#v", memory.Entries)
	}
	if memory.Entries[0].MemoryType != "technical_context" {
		t.Fatalf("memory type = %q", memory.Entries[0].MemoryType)
	}
}

func TestCompile_SucceedsWhenHindsightRecallFails(t *testing.T) {
	orgID := uuid.New()
	agent := model.Employee{ID: uuid.New(), OrgID: &orgID, Name: "Aria", Model: DefaultEmployeeModel}

	def, err := Compile(context.Background(), CompileDeps{Hindsight: &fakeMemoryRecall{err: errors.New("offline")}, Cfg: &config.Config{}}, &agent)
	if err != nil {
		t.Fatalf("compile should not fail when memory recall fails: %v", err)
	}
	memory, ok := def.Context["memory"].(MemoryContext)
	if !ok {
		t.Fatalf("memory context missing or wrong type: %#v", def.Context["memory"])
	}
	if len(memory.Entries) != 0 {
		t.Fatalf("expected empty memory entries, got %#v", memory.Entries)
	}
}
