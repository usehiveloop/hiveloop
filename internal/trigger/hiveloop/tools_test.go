package hiveloop

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/mcp/catalog"
	"github.com/usehiveloop/hiveloop/internal/model"
)

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
					DisplayName:    "Get Pull Request",
					Description:    "Get a single pull request by number",
					Access:         "read",
					Parameters:     json.RawMessage(`{"type":"object","properties":{"owner":{"type":"string","description":"Repository owner"},"repo":{"type":"string","description":"Repository name"},"pull_number":{"type":"integer","description":"PR number"}},"required":["owner","repo","pull_number"]}`),
					ResponseSchema: "pull_request",
				},
				"pulls_get_diff": {
					DisplayName: "Get PR Diff",
					Description: "Get the diff for a pull request",
					Access:      "read",
					Parameters:  json.RawMessage(`{"type":"object","properties":{"owner":{"type":"string","description":"Repository owner"},"repo":{"type":"string","description":"Repository name"},"pull_number":{"type":"integer","description":"PR number"}},"required":["owner","repo","pull_number"]}`),
				},
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
