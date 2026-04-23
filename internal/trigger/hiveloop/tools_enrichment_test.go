package hiveloop

import (
	"context"
	"strings"
	"testing"
)

func TestPlanEnrich_Valid(t *testing.T) {
	planned := NewPlannedStepRegistry()
	var enrichments []PlannedEnrichment
	handler := NewPlanEnrichmentHandler(testConnections(), nil, planned, &enrichments)

	result, done, err := handler(context.Background(), "call-1", marshalArgs(planEnrichmentArgs{
		ConnectionID: "cccccccc-0000-0000-0000-000000000001",
		Action:       "pulls_get",
		As:           "pr_detail",
		Params:       map[string]any{"owner": "acme", "repo": "api", "pull_number": 456},
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if done {
		t.Error("plan_enrichment should not signal done")
	}
	if !strings.Contains(result, "✓") {
		t.Errorf("expected success marker in result: %q", result)
	}
	if !strings.Contains(result, "pr_detail") {
		t.Errorf("result should mention step name: %q", result)
	}
	if len(enrichments) != 1 {
		t.Fatalf("expected 1 enrichment, got %d", len(enrichments))
	}
	if enrichments[0].As != "pr_detail" {
		t.Errorf("enrichment.As: got %q", enrichments[0].As)
	}
	if !planned.Has("pr_detail") {
		t.Error("step should be registered in planned registry")
	}
}

func TestPlanEnrich_UnknownConnection(t *testing.T) {
	planned := NewPlannedStepRegistry()
	var enrichments []PlannedEnrichment
	handler := NewPlanEnrichmentHandler(testConnections(), nil, planned, &enrichments)

	_, _, err := handler(context.Background(), "call-1", marshalArgs(planEnrichmentArgs{
		ConnectionID: "ffffffff-0000-0000-0000-000000000099",
		Action:       "pulls_get",
		As:           "pr_detail",
		Params:       map[string]any{"owner": "acme", "repo": "api", "pull_number": 456},
	}))
	if err == nil {
		t.Fatal("expected error for unknown connection")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention 'not found': %v", err)
	}
	if !strings.Contains(err.Error(), "github-app") || !strings.Contains(err.Error(), "slack") {
		t.Errorf("error should list available connections with providers: %v", err)
	}
}

func TestPlanEnrich_UnknownAction(t *testing.T) {
	planned := NewPlannedStepRegistry()
	var enrichments []PlannedEnrichment
	handler := NewPlanEnrichmentHandler(testConnections(), nil, planned, &enrichments)

	_, _, err := handler(context.Background(), "call-1", marshalArgs(planEnrichmentArgs{
		ConnectionID: "cccccccc-0000-0000-0000-000000000001",
		Action:       "nonexistent_action",
		As:           "whatever",
		Params:       map[string]any{},
	}))
	if err == nil {
		t.Fatal("expected error for unknown action")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention 'not found': %v", err)
	}
	if !strings.Contains(err.Error(), "pulls_get") {
		t.Errorf("error should list available read actions: %v", err)
	}
}

func TestPlanEnrich_WriteActionNotInReadActions(t *testing.T) {
	planned := NewPlannedStepRegistry()
	var enrichments []PlannedEnrichment
	handler := NewPlanEnrichmentHandler(testConnections(), nil, planned, &enrichments)

	_, _, err := handler(context.Background(), "call-1", marshalArgs(planEnrichmentArgs{
		ConnectionID: "cccccccc-0000-0000-0000-000000000001",
		Action:       "issues_create_comment",
		As:           "comment",
		Params:       map[string]any{"owner": "acme", "repo": "api", "issue_number": 1, "body": "hi"},
	}))
	if err == nil {
		t.Fatal("expected error for write action (not in ReadActions)")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should say action not found: %v", err)
	}
	if !strings.Contains(err.Error(), "pulls_get") {
		t.Errorf("error should list available read actions: %v", err)
	}
}

func TestPlanEnrich_DuplicateStepName(t *testing.T) {
	planned := NewPlannedStepRegistry()
	planned.Add("pr_detail", "pulls_get")
	var enrichments []PlannedEnrichment
	handler := NewPlanEnrichmentHandler(testConnections(), nil, planned, &enrichments)

	_, _, err := handler(context.Background(), "call-1", marshalArgs(planEnrichmentArgs{
		ConnectionID: "cccccccc-0000-0000-0000-000000000001",
		Action:       "pulls_get",
		As:           "pr_detail",
		Params:       map[string]any{"owner": "acme", "repo": "api", "pull_number": 456},
	}))
	if err == nil {
		t.Fatal("expected error for duplicate step name")
	}
	if !strings.Contains(err.Error(), "already used") {
		t.Errorf("error should mention 'already used': %v", err)
	}
}

func TestPlanEnrich_MissingRequiredParam(t *testing.T) {
	planned := NewPlannedStepRegistry()
	var enrichments []PlannedEnrichment
	handler := NewPlanEnrichmentHandler(testConnections(), nil, planned, &enrichments)

	_, _, err := handler(context.Background(), "call-1", marshalArgs(planEnrichmentArgs{
		ConnectionID: "cccccccc-0000-0000-0000-000000000001",
		Action:       "pulls_get",
		As:           "pr_detail",
		Params:       map[string]any{"owner": "acme"},
	}))
	if err == nil {
		t.Fatal("expected error for missing required params")
	}
	if !strings.Contains(err.Error(), "missing required param") {
		t.Errorf("error should mention missing param: %v", err)
	}
}

func TestPlanEnrich_InvalidStepRef(t *testing.T) {
	planned := NewPlannedStepRegistry()
	var enrichments []PlannedEnrichment
	handler := NewPlanEnrichmentHandler(testConnections(), nil, planned, &enrichments)

	_, _, err := handler(context.Background(), "call-1", marshalArgs(planEnrichmentArgs{
		ConnectionID: "cccccccc-0000-0000-0000-000000000001",
		Action:       "pulls_get",
		As:           "pr_detail",
		Params:       map[string]any{"owner": "{{nonexistent_step.owner}}", "repo": "api", "pull_number": 456},
	}))
	if err == nil {
		t.Fatal("expected error for invalid step reference")
	}
	if !strings.Contains(err.Error(), "nonexistent_step") {
		t.Errorf("error should mention the bad step name: %v", err)
	}
	if !strings.Contains(err.Error(), "hasn't been planned") {
		t.Errorf("error should explain the step doesn't exist: %v", err)
	}
}

func TestPlanEnrich_ValidStepRef(t *testing.T) {
	planned := NewPlannedStepRegistry()
	planned.Add("pr_detail", "pulls_get")
	var enrichments []PlannedEnrichment
	handler := NewPlanEnrichmentHandler(testConnections(), nil, planned, &enrichments)

	_, _, err := handler(context.Background(), "call-1", marshalArgs(planEnrichmentArgs{
		ConnectionID: "cccccccc-0000-0000-0000-000000000001",
		Action:       "pulls_get_diff",
		As:           "pr_diff",
		Params:       map[string]any{"owner": "{{pr_detail.base.repo.owner.login}}", "repo": "{{pr_detail.base.repo.name}}", "pull_number": "{{pr_detail.number}}"},
	}))
	if err != nil {
		t.Fatalf("valid step ref should succeed: %v", err)
	}
	if len(enrichments) != 1 {
		t.Fatalf("expected 1 enrichment, got %d", len(enrichments))
	}
}

func TestPlanEnrich_RefsParam(t *testing.T) {
	planned := NewPlannedStepRegistry()
	var enrichments []PlannedEnrichment
	handler := NewPlanEnrichmentHandler(testConnections(), nil, planned, &enrichments)

	_, _, err := handler(context.Background(), "call-1", marshalArgs(planEnrichmentArgs{
		ConnectionID: "cccccccc-0000-0000-0000-000000000002",
		Action:       "conversations_replies",
		As:           "thread",
		Params:       map[string]any{"channel": "$refs.channel_id", "ts": "$refs.thread_id"},
	}))
	if err != nil {
		t.Fatalf("$refs params should be accepted: %v", err)
	}
}
