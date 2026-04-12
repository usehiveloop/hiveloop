package zira

import (
	"context"
	"testing"
)

// Enrichment depth tests verify that the agent loop correctly plans
// chained enrichment sequences of increasing depth. All tests use
// GitHub + Slack schemas (the two validated providers).

func TestEnrich_1Fetch(t *testing.T) {
	mock := NewMockCompletionClient()
	mock.OnMessage("check PR #456",
		toolCallResponse(
			routeCall("aaaaaaaa-0000-0000-0000-000000000001", 1),
			enrichCall("cccccccc-0000-0000-0000-000000000001", "pulls_get", "pr",
				map[string]any{"owner": "acme", "repo": "api", "pull_number": 456}),
			finalizeCall(),
		),
	)

	agent := NewRouterAgent(mock, "test-model", 10)
	result, err := agent.Route(context.Background(), "system prompt", "check PR #456", testAgents(), testConnections())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.EnrichmentPlan) != 1 {
		t.Fatalf("expected 1 enrichment, got %d", len(result.EnrichmentPlan))
	}
	assertEnrichment(t, result.EnrichmentPlan[0], "pr", "pulls_get")
}

func TestEnrich_3Fetches(t *testing.T) {
	mock := NewMockCompletionClient()
	mock.OnMessage("review PR deeply",
		toolCallResponse(
			routeCall("aaaaaaaa-0000-0000-0000-000000000001", 1),
			enrichCall("cccccccc-0000-0000-0000-000000000001", "pulls_get", "pr",
				map[string]any{"owner": "acme", "repo": "api", "pull_number": 456}),
		),
		toolCallResponse(
			enrichCall("cccccccc-0000-0000-0000-000000000001", "pulls_get_diff", "pr_diff",
				map[string]any{"owner": "{{pr.base.repo.owner}}", "repo": "{{pr.base.repo.name}}", "pull_number": "{{pr.number}}"}),
		),
		toolCallResponse(
			enrichCall("cccccccc-0000-0000-0000-000000000002", "conversations_replies", "thread",
				map[string]any{"channel": "$refs.channel_id", "ts": "$refs.thread_id"}),
			finalizeCall(),
		),
	)

	agent := NewRouterAgent(mock, "test-model", 10)
	result, err := agent.Route(context.Background(), "system prompt", "review PR deeply", testAgents(), testConnections())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.EnrichmentPlan) != 3 {
		t.Fatalf("expected 3 enrichments, got %d", len(result.EnrichmentPlan))
	}
	assertEnrichment(t, result.EnrichmentPlan[0], "pr", "pulls_get")
	assertEnrichment(t, result.EnrichmentPlan[1], "pr_diff", "pulls_get_diff")
	assertEnrichment(t, result.EnrichmentPlan[2], "thread", "conversations_replies")
}

func TestEnrich_5Fetches(t *testing.T) {
	mock := NewMockCompletionClient()
	mock.OnMessage("full PR context",
		toolCallResponse(
			routeCall("aaaaaaaa-0000-0000-0000-000000000001", 1),
			enrichCall("cccccccc-0000-0000-0000-000000000001", "pulls_get", "pr",
				map[string]any{"owner": "acme", "repo": "api", "pull_number": 456}),
			enrichCall("cccccccc-0000-0000-0000-000000000001", "pulls_get_diff", "pr_diff",
				map[string]any{"owner": "acme", "repo": "api", "pull_number": 456}),
		),
		toolCallResponse(
			enrichCall("cccccccc-0000-0000-0000-000000000001", "pulls_get", "pr_reviews",
				map[string]any{"owner": "acme", "repo": "api", "pull_number": 456}),
			enrichCall("cccccccc-0000-0000-0000-000000000002", "conversations_replies", "slack_thread",
				map[string]any{"channel": "$refs.channel_id", "ts": "$refs.thread_id"}),
		),
		toolCallResponse(
			enrichCall("cccccccc-0000-0000-0000-000000000001", "pulls_get", "repo_info",
				map[string]any{"owner": "acme", "repo": "api", "pull_number": 1}),
			finalizeCall(),
		),
	)

	agent := NewRouterAgent(mock, "test-model", 10)
	result, err := agent.Route(context.Background(), "system prompt", "full PR context", testAgents(), testConnections())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.EnrichmentPlan) != 5 {
		t.Fatalf("expected 5 enrichments, got %d", len(result.EnrichmentPlan))
	}
}

func TestEnrich_CrossConnection(t *testing.T) {
	mock := NewMockCompletionClient()
	mock.OnMessage("check this thread and the PR",
		toolCallResponse(
			routeCall("aaaaaaaa-0000-0000-0000-000000000001", 1),
			enrichCall("cccccccc-0000-0000-0000-000000000001", "pulls_get", "github_pr",
				map[string]any{"owner": "acme", "repo": "api", "pull_number": 789}),
			enrichCall("cccccccc-0000-0000-0000-000000000002", "conversations_replies", "slack_thread",
				map[string]any{"channel": "$refs.channel_id", "ts": "$refs.thread_id"}),
			finalizeCall(),
		),
	)

	agent := NewRouterAgent(mock, "test-model", 10)
	result, err := agent.Route(context.Background(), "system prompt", "check this thread and the PR", testAgents(), testConnections())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.EnrichmentPlan) != 2 {
		t.Fatalf("expected 2 enrichments (cross-connection), got %d", len(result.EnrichmentPlan))
	}
	// Verify they target different connections
	connIDs := map[string]bool{}
	for _, enrichment := range result.EnrichmentPlan {
		connIDs[enrichment.ConnectionID.String()] = true
	}
	if len(connIDs) != 2 {
		t.Errorf("expected 2 different connection IDs, got %d", len(connIDs))
	}
}

func TestEnrich_ParamChaining(t *testing.T) {
	mock := NewMockCompletionClient()
	mock.OnMessage("issue context",
		toolCallResponse(
			routeCall("aaaaaaaa-0000-0000-0000-000000000002", 1),
			enrichCall("cccccccc-0000-0000-0000-000000000001", "pulls_get", "issue",
				map[string]any{"owner": "acme", "repo": "api", "pull_number": 123}),
		),
		toolCallResponse(
			enrichCall("cccccccc-0000-0000-0000-000000000001", "pulls_get", "repo",
				map[string]any{"owner": "{{issue.base.repo.owner}}", "repo": "{{issue.base.repo.name}}", "pull_number": 1}),
			finalizeCall(),
		),
	)

	agent := NewRouterAgent(mock, "test-model", 10)
	result, err := agent.Route(context.Background(), "system prompt", "issue context", testAgents(), testConnections())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.EnrichmentPlan) != 2 {
		t.Fatalf("expected 2 enrichments, got %d", len(result.EnrichmentPlan))
	}
	// Second enrichment should have template refs
	repoParams := result.EnrichmentPlan[1].Params
	ownerVal, ok := repoParams["owner"].(string)
	if !ok || ownerVal != "{{issue.base.repo.owner}}" {
		t.Errorf("repo.owner should be a template ref, got %v", repoParams["owner"])
	}
}

func TestEnrich_BareIssueNumber(t *testing.T) {
	mock := NewMockCompletionClient()
	mock.OnMessage("check #456",
		toolCallResponse(
			routeCall("aaaaaaaa-0000-0000-0000-000000000001", 1),
			enrichCall("cccccccc-0000-0000-0000-000000000001", "pulls_get", "pr",
				map[string]any{"owner": "$refs.owner", "repo": "$refs.repo", "pull_number": 456}),
			finalizeCall(),
		),
	)

	agent := NewRouterAgent(mock, "test-model", 10)
	result, err := agent.Route(context.Background(), "system prompt", "check #456", testAgents(), testConnections())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.EnrichmentPlan) != 1 {
		t.Fatalf("expected 1 enrichment, got %d", len(result.EnrichmentPlan))
	}
	// Verify params use $refs
	params := result.EnrichmentPlan[0].Params
	if params["owner"] != "$refs.owner" {
		t.Errorf("expected $refs.owner, got %v", params["owner"])
	}
}

func TestEnrich_NoRefs_Empty(t *testing.T) {
	mock := NewMockCompletionClient()
	mock.OnMessage("hello how are you",
		toolCallResponse(finalizeCall()),
	)

	agent := NewRouterAgent(mock, "test-model", 10)
	result, err := agent.Route(context.Background(), "system prompt", "hello how are you", testAgents(), testConnections())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.EnrichmentPlan) != 0 {
		t.Errorf("expected 0 enrichments for simple greeting, got %d", len(result.EnrichmentPlan))
	}
}

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

func assertEnrichment(t *testing.T, enrichment PlannedEnrichment, expectedAs, expectedAction string) {
	t.Helper()
	if enrichment.As != expectedAs {
		t.Errorf("enrichment.As: got %q, want %q", enrichment.As, expectedAs)
	}
	if enrichment.Action != expectedAction {
		t.Errorf("enrichment.Action: got %q, want %q", enrichment.Action, expectedAction)
	}
}
