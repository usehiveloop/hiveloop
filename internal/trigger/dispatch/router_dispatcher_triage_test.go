package dispatch

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/mcp/catalog"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/trigger/hiveloop"
)

func setupTriageStore(triggerID uuid.UUID, mock *hiveloop.MockCompletionClient) (*MemoryRouterTriggerStore, *RouterDispatcher) {
	store := NewMemoryRouterTriggerStore()
	router := newTestRouter()
	trigger := newTestTrigger(triggerID, "triage", "app_mention")
	trigger.EnrichCrossReferences = true
	store.AddTrigger(trigger, router)
	store.AddAgent(newTestAgent(testAgentA, "code-review-agent"))
	store.AddAgent(newTestAgent(testAgentB, "bug-triage-agent"))

	readActions := map[string]catalog.ActionDef{
		"pulls_get": {DisplayName: "Get PR", Description: "Get a PR", Access: "read",
			Parameters: json.RawMessage(`{"type":"object","properties":{"owner":{"type":"string"},"repo":{"type":"string"},"pull_number":{"type":"integer"}},"required":["owner","repo","pull_number"]}`)},
	}
	store.AddConnection(hiveloop.ConnectionWithActions{
		Connection:  model.InConnection{ID: uuid.New(), OrgID: testOrgID},
		Provider:    "github-app",
		ReadActions: readActions,
	})

	routerAgent := hiveloop.NewRouterAgent(mock, "test-model", 10)
	dispatcher := NewRouterDispatcher(store, catalog.Global(), routerAgent, slog.Default())
	return store, dispatcher
}

func TestDispatch_Triage_SlackMention_RoutesToAgent(t *testing.T) {
	mock := hiveloop.NewMockCompletionClient()
	mock.OnMessage("",
		hiveloop.CompletionResponse{Message: hiveloop.Message{Role: "assistant", ToolCalls: []hiveloop.ToolCall{
			{ID: "c1", Name: "route_to_agent", Arguments: `{"agent_id":"` + testAgentA.String() + `","priority":1,"reason":"PR review"}`},
			{ID: "c2", Name: "finalize", Arguments: "{}"},
		}}},
	)
	mock.SetFallback(hiveloop.CompletionResponse{Message: hiveloop.Message{Role: "assistant", ToolCalls: []hiveloop.ToolCall{
		{ID: "c1", Name: "route_to_agent", Arguments: `{"agent_id":"` + testAgentA.String() + `","priority":1,"reason":"PR review"}`},
		{ID: "c2", Name: "finalize", Arguments: "{}"},
	}}})

	triggerID := uuid.New()
	_, dispatcher := setupTriageStore(triggerID, mock)

	input := RouterDispatchInput{
		Provider:     "slack",
		EventType:    "app_mention",
		OrgID:        testOrgID,
		ConnectionID: testConnID,
		Payload:      map[string]any{"event": map[string]any{"text": "review this PR"}},
	}
	dispatches, err := dispatcher.Run(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dispatches) != 1 {
		t.Fatalf("expected 1 dispatch, got %d", len(dispatches))
	}
	if dispatches[0].AgentID != testAgentA {
		t.Errorf("agent: got %s, want %s", dispatches[0].AgentID, testAgentA)
	}
	if dispatches[0].RoutingMode != "triage" {
		t.Errorf("routing mode: got %q, want triage", dispatches[0].RoutingMode)
	}
}

func TestDispatch_Triage_LLMEmpty_DefaultAgent(t *testing.T) {
	mock := hiveloop.NewMockCompletionClient()
	mock.SetFallback(hiveloop.CompletionResponse{Message: hiveloop.Message{Role: "assistant", ToolCalls: []hiveloop.ToolCall{
		{ID: "c1", Name: "finalize", Arguments: "{}"},
	}}})

	triggerID := uuid.New()
	_, dispatcher := setupTriageStore(triggerID, mock)

	input := RouterDispatchInput{
		Provider: "slack", EventType: "app_mention",
		OrgID: testOrgID, ConnectionID: testConnID,
		Payload: map[string]any{},
	}
	dispatches, err := dispatcher.Run(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dispatches) != 1 {
		t.Fatalf("expected 1 dispatch (default fallback), got %d", len(dispatches))
	}
	if dispatches[0].AgentID != testAgentB {
		t.Errorf("fallback: got %s, want default agent %s", dispatches[0].AgentID, testAgentB)
	}
}
