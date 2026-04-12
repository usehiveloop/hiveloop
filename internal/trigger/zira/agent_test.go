package zira

import (
	"context"
	"encoding/json"
	"testing"
)

// --------------------------------------------------------------------------
// Helper: build a CompletionResponse with tool calls
// --------------------------------------------------------------------------

func toolCallResponse(calls ...ToolCall) CompletionResponse {
	return CompletionResponse{
		Message: Message{
			Role:      "assistant",
			ToolCalls: calls,
		},
	}
}

func tc(name, argsJSON string) ToolCall {
	return ToolCall{ID: "call-" + name, Name: name, Arguments: argsJSON}
}

func routeCall(agentID string, priority int) ToolCall {
	args, _ := json.Marshal(routeToAgentArgs{AgentID: agentID, Priority: priority, Reason: "test"})
	return ToolCall{ID: "call-route-" + agentID[:8], Name: "route_to_agent", Arguments: string(args)}
}

func enrichCall(connID, action, as string, params map[string]any) ToolCall {
	args, _ := json.Marshal(planEnrichmentArgs{ConnectionID: connID, Action: action, As: as, Params: params})
	return ToolCall{ID: "call-enrich-" + as, Name: "plan_enrichment", Arguments: string(args)}
}

func finalizeCall() ToolCall {
	return ToolCall{ID: "call-finalize", Name: "finalize", Arguments: "{}"}
}

// --------------------------------------------------------------------------
// Agent loop tests
// --------------------------------------------------------------------------

func TestAgent_SingleRoute_NoEnrich(t *testing.T) {
	mock := NewMockCompletionClient()
	mock.OnMessage("review this PR",
		toolCallResponse(routeCall("aaaaaaaa-0000-0000-0000-000000000001", 1), finalizeCall()),
	)

	agent := NewRouterAgent(mock, "test-model", 10)
	result, err := agent.Route(context.Background(), "system prompt", "review this PR", testAgents(), testConnections())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.SelectedAgents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(result.SelectedAgents))
	}
	if result.SelectedAgents[0].AgentID.String() != "aaaaaaaa-0000-0000-0000-000000000001" {
		t.Errorf("wrong agent: %s", result.SelectedAgents[0].AgentID)
	}
	if len(result.EnrichmentPlan) != 0 {
		t.Errorf("expected 0 enrichments, got %d", len(result.EnrichmentPlan))
	}
	if result.TurnCount != 1 {
		t.Errorf("turn count: got %d, want 1", result.TurnCount)
	}
}

func TestAgent_SingleRoute_WithEnrich(t *testing.T) {
	mock := NewMockCompletionClient()
	mock.OnMessage("review PR #456",
		// Turn 1: route + plan enrichment
		toolCallResponse(
			routeCall("aaaaaaaa-0000-0000-0000-000000000001", 1),
			enrichCall("cccccccc-0000-0000-0000-000000000001", "pulls_get", "pr_detail",
				map[string]any{"owner": "acme", "repo": "api", "pull_number": 456}),
		),
		// Turn 2: chain enrichment + finalize
		toolCallResponse(
			enrichCall("cccccccc-0000-0000-0000-000000000001", "pulls_get_diff", "pr_diff",
				map[string]any{"owner": "{{pr_detail.base.repo.owner.login}}", "repo": "{{pr_detail.base.repo.name}}", "pull_number": "{{pr_detail.number}}"}),
			finalizeCall(),
		),
	)

	agent := NewRouterAgent(mock, "test-model", 10)
	result, err := agent.Route(context.Background(), "system prompt", "review PR #456", testAgents(), testConnections())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.SelectedAgents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(result.SelectedAgents))
	}
	if len(result.EnrichmentPlan) != 2 {
		t.Fatalf("expected 2 enrichments, got %d", len(result.EnrichmentPlan))
	}
	if result.EnrichmentPlan[0].As != "pr_detail" {
		t.Errorf("enrichment[0].As: got %q", result.EnrichmentPlan[0].As)
	}
	if result.EnrichmentPlan[1].As != "pr_diff" {
		t.Errorf("enrichment[1].As: got %q", result.EnrichmentPlan[1].As)
	}
	if result.TurnCount != 2 {
		t.Errorf("turn count: got %d, want 2", result.TurnCount)
	}
}

func TestAgent_MultiRoute(t *testing.T) {
	mock := NewMockCompletionClient()
	mock.OnMessage("review PR and check security",
		toolCallResponse(
			routeCall("aaaaaaaa-0000-0000-0000-000000000001", 1),
			routeCall("aaaaaaaa-0000-0000-0000-000000000002", 2),
			finalizeCall(),
		),
	)

	agent := NewRouterAgent(mock, "test-model", 10)
	result, err := agent.Route(context.Background(), "system prompt", "review PR and check security", testAgents(), testConnections())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.SelectedAgents) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(result.SelectedAgents))
	}
	if result.SelectedAgents[0].Priority != 1 {
		t.Errorf("first agent priority: got %d, want 1", result.SelectedAgents[0].Priority)
	}
	if result.SelectedAgents[1].Priority != 2 {
		t.Errorf("second agent priority: got %d, want 2", result.SelectedAgents[1].Priority)
	}
}

func TestAgent_EnrichChain_StepRef(t *testing.T) {
	mock := NewMockCompletionClient()
	mock.OnMessage("check PR",
		toolCallResponse(
			routeCall("aaaaaaaa-0000-0000-0000-000000000001", 1),
			enrichCall("cccccccc-0000-0000-0000-000000000001", "pulls_get", "pr",
				map[string]any{"owner": "acme", "repo": "api", "pull_number": 123}),
		),
		toolCallResponse(
			enrichCall("cccccccc-0000-0000-0000-000000000001", "pulls_get_diff", "diff",
				map[string]any{"owner": "{{pr.base.repo.owner}}", "repo": "{{pr.base.repo.name}}", "pull_number": "{{pr.number}}"}),
			finalizeCall(),
		),
	)

	agent := NewRouterAgent(mock, "test-model", 10)
	result, err := agent.Route(context.Background(), "system prompt", "check PR", testAgents(), testConnections())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.EnrichmentPlan) != 2 {
		t.Fatalf("expected 2 enrichments, got %d", len(result.EnrichmentPlan))
	}
	// Verify the second step references the first via {{pr.field}}
	diffParams := result.EnrichmentPlan[1].Params
	ownerParam, ok := diffParams["owner"].(string)
	if !ok || ownerParam != "{{pr.base.repo.owner}}" {
		t.Errorf("diff.owner should be a step ref: got %v", diffParams["owner"])
	}
}

func TestAgent_InvalidAgent_Retry(t *testing.T) {
	mock := NewMockCompletionClient()
	mock.OnMessage("test retry",
		// Turn 1: LLM sends bad agent ID
		toolCallResponse(
			routeCall("ffffffff-0000-0000-0000-000000000099", 1),
		),
		// Turn 2: LLM corrects after seeing error
		toolCallResponse(
			routeCall("aaaaaaaa-0000-0000-0000-000000000001", 1),
			finalizeCall(),
		),
	)

	agent := NewRouterAgent(mock, "test-model", 10)
	result, err := agent.Route(context.Background(), "system prompt", "test retry", testAgents(), testConnections())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.SelectedAgents) != 1 {
		t.Fatalf("expected 1 agent after retry, got %d", len(result.SelectedAgents))
	}
	if result.TurnCount != 2 {
		t.Errorf("turn count: got %d, want 2 (initial + retry)", result.TurnCount)
	}
	mock.AssertCallCount(t, 2)
}

func TestAgent_MissingParam_Retry(t *testing.T) {
	mock := NewMockCompletionClient()
	mock.OnMessage("test param retry",
		// Turn 1: LLM sends plan_enrichment with missing param
		toolCallResponse(
			routeCall("aaaaaaaa-0000-0000-0000-000000000001", 1),
			enrichCall("cccccccc-0000-0000-0000-000000000001", "pulls_get", "pr",
				map[string]any{"owner": "acme"}), // missing repo + pull_number
		),
		// Turn 2: LLM retries with correct params after seeing schema in error
		toolCallResponse(
			enrichCall("cccccccc-0000-0000-0000-000000000001", "pulls_get", "pr",
				map[string]any{"owner": "acme", "repo": "api", "pull_number": 456}),
			finalizeCall(),
		),
	)

	agent := NewRouterAgent(mock, "test-model", 10)
	result, err := agent.Route(context.Background(), "system prompt", "test param retry", testAgents(), testConnections())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.EnrichmentPlan) != 1 {
		t.Fatalf("expected 1 enrichment after retry, got %d", len(result.EnrichmentPlan))
	}
	if result.TurnCount != 2 {
		t.Errorf("turn count: got %d, want 2", result.TurnCount)
	}
}

func TestAgent_MaxTurns_ReturnsPartial(t *testing.T) {
	mock := NewMockCompletionClient()
	// LLM never calls finalize — keeps routing forever
	mock.OnMessage("infinite loop",
		toolCallResponse(routeCall("aaaaaaaa-0000-0000-0000-000000000001", 1)),
	)

	agent := NewRouterAgent(mock, "test-model", 3)
	result, err := agent.Route(context.Background(), "system prompt", "infinite loop", testAgents(), testConnections())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should have collected the route from the repeated first response
	if len(result.SelectedAgents) == 0 {
		t.Error("expected at least 1 agent from partial result")
	}
	if result.TurnCount != 3 {
		t.Errorf("turn count: got %d, want 3 (max_turns)", result.TurnCount)
	}
}

func TestAgent_NoRoutes_EmptyResult(t *testing.T) {
	mock := NewMockCompletionClient()
	mock.OnMessage("hello how are you",
		toolCallResponse(finalizeCall()),
	)

	agent := NewRouterAgent(mock, "test-model", 10)
	result, err := agent.Route(context.Background(), "system prompt", "hello how are you", testAgents(), testConnections())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.SelectedAgents) != 0 {
		t.Errorf("expected 0 agents, got %d", len(result.SelectedAgents))
	}
	if len(result.EnrichmentPlan) != 0 {
		t.Errorf("expected 0 enrichments, got %d", len(result.EnrichmentPlan))
	}
}
