package hindsight

import (
	"testing"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/model"
)

func TestBaseMemoryTags(t *testing.T) {
	orgID := uuid.New()
	teamID := uuid.New()
	agentID := uuid.New()
	agent := &model.Agent{
		ID:     agentID,
		OrgID:  &orgID,
		TeamID: &teamID,
	}

	tags := baseMemoryTags(agent, "manual")
	want := map[string]bool{
		"company:" + orgID.String(): false,
		"team:" + teamID.String():   false,
		"source:manual":             false,
		"visibility:team":           false,
	}
	for _, tag := range tags {
		if tag == "employee:"+agentID.String() {
			t.Fatalf("memory tags must not include employee-private scoping: %#v", tags)
		}
		if _, ok := want[tag]; ok {
			want[tag] = true
		}
	}
	for tag, seen := range want {
		if !seen {
			t.Fatalf("missing tag %s in %#v", tag, tags)
		}
	}
}
