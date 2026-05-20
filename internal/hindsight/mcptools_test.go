package hindsight

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func TestBaseMemoryTags(t *testing.T) {
	orgID := uuid.New()
	agentID := uuid.New()
	agent := &model.Agent{
		ID:    agentID,
		OrgID: &orgID,
	}

	tags := baseMemoryTags(agent, "manual")
	want := map[string]bool{
		"company:" + orgID.String(): false,
		"source:manual":             false,
		"visibility:company":        false,
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

func TestMemoryRetainResponseExplainsBackgroundProcessing(t *testing.T) {
	resp := memoryRetainResponse("org-test", "manual-agent-doc", &RetainResponse{
		Success:     true,
		BankID:      "org-test",
		ItemsCount:  1,
		Async:       true,
		OperationID: "retain-op-1",
	})
	if !resp.Async {
		t.Fatal("expected async retain response")
	}
	if !strings.Contains(resp.Message, "processed in the background") {
		t.Fatalf("message does not explain background processing: %q", resp.Message)
	}
	if !strings.Contains(resp.Message, "memory_recall") {
		t.Fatalf("message does not explain delayed recall visibility: %q", resp.Message)
	}
}
