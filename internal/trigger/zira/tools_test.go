package zira

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/mcp/catalog"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// --------------------------------------------------------------------------
// Test fixtures
// --------------------------------------------------------------------------

func testAgents() []model.Agent {
	desc1 := "Reviews pull requests for bugs, security issues, and style violations"
	desc2 := "Triages bug reports, assigns priority, recommends assignee"
	return []model.Agent{
		{ID: uuid.MustParse("aaaaaaaa-0000-0000-0000-000000000001"), Name: "code-review-agent", Description: &desc1},
		{ID: uuid.MustParse("aaaaaaaa-0000-0000-0000-000000000002"), Name: "bug-triage-agent", Description: &desc2},
	}
}

func testConnections() []ConnectionWithActions {
	return []ConnectionWithActions{
		{
			Connection: model.InConnection{ID: uuid.MustParse("cccccccc-0000-0000-0000-000000000001")},
			Provider:   "github-app",
			ReadActions: map[string]catalog.ActionDef{
				"pulls_get": {
					DisplayName: "Get Pull Request",
					Description: "Get a single pull request by number",
					Access:      "read",
					Parameters:  json.RawMessage(`{"type":"object","properties":{"owner":{"type":"string","description":"Repository owner"},"repo":{"type":"string","description":"Repository name"},"pull_number":{"type":"integer","description":"PR number"}},"required":["owner","repo","pull_number"]}`),
					ResponseSchema: "pull_request",
				},
				"pulls_get_diff": {
					DisplayName: "Get PR Diff",
					Description: "Get the diff for a pull request",
					Access:      "read",
					Parameters:  json.RawMessage(`{"type":"object","properties":{"owner":{"type":"string","description":"Repository owner"},"repo":{"type":"string","description":"Repository name"},"pull_number":{"type":"integer","description":"PR number"}},"required":["owner","repo","pull_number"]}`),
				},
				// Write actions (issues_create_comment, etc.) are NOT in
				// ReadActions — they're exposed via the Reply MCP handler.
			},
		},
		{
			Connection: model.InConnection{ID: uuid.MustParse("cccccccc-0000-0000-0000-000000000002")},
			Provider:   "slack",
			ReadActions: map[string]catalog.ActionDef{
				"conversations_replies": {
					DisplayName: "Get Thread Replies",
					Description: "Fetch all replies in a Slack thread",
					Access:      "read",
					Parameters:  json.RawMessage(`{"type":"object","properties":{"channel":{"type":"string","description":"Channel ID"},"ts":{"type":"string","description":"Thread timestamp"}},"required":["channel","ts"]}`),
				},
			},
		},
	}
}

func marshalArgs(args any) json.RawMessage {
	data, _ := json.Marshal(args)
	return data
}

// --------------------------------------------------------------------------
// route_to_agent tests
// --------------------------------------------------------------------------

func TestRouteToAgent_ValidAgent(t *testing.T) {
	var selections []AgentSelection
	handler := NewRouteToAgentHandler(testAgents(), &selections)

	result, done, err := handler(context.Background(), "call-1", marshalArgs(routeToAgentArgs{
		AgentID:  "aaaaaaaa-0000-0000-0000-000000000001",
		Priority: 1,
		Reason:   "PR review request",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if done {
		t.Error("route_to_agent should not signal done")
	}
	if !strings.Contains(result, "✓") {
		t.Errorf("expected success marker in result: %q", result)
	}
	if len(selections) != 1 {
		t.Fatalf("expected 1 selection, got %d", len(selections))
	}
	if selections[0].AgentID.String() != "aaaaaaaa-0000-0000-0000-000000000001" {
		t.Errorf("agent_id: got %s", selections[0].AgentID)
	}
	if selections[0].Priority != 1 {
		t.Errorf("priority: got %d", selections[0].Priority)
	}
}

func TestRouteToAgent_UnknownAgent(t *testing.T) {
	var selections []AgentSelection
	handler := NewRouteToAgentHandler(testAgents(), &selections)

	_, _, err := handler(context.Background(), "call-1", marshalArgs(routeToAgentArgs{
		AgentID:  "ffffffff-0000-0000-0000-000000000099",
		Priority: 1,
		Reason:   "test",
	}))
	if err == nil {
		t.Fatal("expected error for unknown agent")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention 'not found': %v", err)
	}
	if !strings.Contains(err.Error(), "code-review-agent") {
		t.Errorf("error should list available agents: %v", err)
	}
	if len(selections) != 0 {
		t.Errorf("no selection should be recorded on error")
	}
}

func TestRouteToAgent_InvalidPriority(t *testing.T) {
	var selections []AgentSelection
	handler := NewRouteToAgentHandler(testAgents(), &selections)

	_, _, err := handler(context.Background(), "call-1", marshalArgs(routeToAgentArgs{
		AgentID:  "aaaaaaaa-0000-0000-0000-000000000001",
		Priority: 0,
		Reason:   "test",
	}))
	if err == nil {
		t.Fatal("expected error for priority 0")
	}
	if !strings.Contains(err.Error(), "1-5") {
		t.Errorf("error should mention valid range: %v", err)
	}
}

func TestRouteToAgent_DuplicateAccepted(t *testing.T) {
	var selections []AgentSelection
	handler := NewRouteToAgentHandler(testAgents(), &selections)

	handler(context.Background(), "call-1", marshalArgs(routeToAgentArgs{
		AgentID: "aaaaaaaa-0000-0000-0000-000000000001", Priority: 1, Reason: "first",
	}))
	handler(context.Background(), "call-2", marshalArgs(routeToAgentArgs{
		AgentID: "aaaaaaaa-0000-0000-0000-000000000001", Priority: 2, Reason: "second",
	}))

	if len(selections) != 2 {
		t.Fatalf("expected 2 selections for duplicate agent (different priorities), got %d", len(selections))
	}
}

func TestRouteToAgent_EmptyAgentID(t *testing.T) {
	var selections []AgentSelection
	handler := NewRouteToAgentHandler(testAgents(), &selections)

	_, _, err := handler(context.Background(), "call-1", marshalArgs(routeToAgentArgs{
		Priority: 1, Reason: "test",
	}))
	if err == nil {
		t.Fatal("expected error for empty agent_id")
	}
}

// --------------------------------------------------------------------------
// plan_enrichment tests
// --------------------------------------------------------------------------

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

	// Write actions (issues_create_comment) are excluded from the ReadActions
	// map at fixture construction time — same as production, where the store
	// only populates read actions. Requesting a write action returns "not found"
	// with the list of available read actions.
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
		Params:       map[string]any{"owner": "acme"}, // missing repo + pull_number
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

// --------------------------------------------------------------------------
// finalize tests
// --------------------------------------------------------------------------

func TestFinalize(t *testing.T) {
	handler := NewFinalizeHandler()
	result, done, err := handler(context.Background(), "call-1", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !done {
		t.Error("finalize should signal done")
	}
	if !strings.Contains(result, "✓") {
		t.Errorf("expected success marker: %q", result)
	}
}
