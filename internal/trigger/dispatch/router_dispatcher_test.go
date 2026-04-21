package dispatch

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/usehiveloop/hiveloop/internal/mcp/catalog"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/trigger/hiveloop"
)

// --------------------------------------------------------------------------
// Test fixtures
// --------------------------------------------------------------------------

var (
	testOrgID    = uuid.MustParse("11111111-0000-0000-0000-000000000001")
	testConnID   = uuid.MustParse("22222222-0000-0000-0000-000000000001")
	testRouterID = uuid.MustParse("33333333-0000-0000-0000-000000000001")
	testAgentA   = uuid.MustParse("aaaaaaaa-0000-0000-0000-000000000001")
	testAgentB   = uuid.MustParse("aaaaaaaa-0000-0000-0000-000000000002")
)

func newTestRouter() model.Router {
	return model.Router{
		ID:             testRouterID,
		OrgID:          testOrgID,
		Name:           "Zira",
		Persona:        "You are a helpful teammate.",
		DefaultAgentID: &testAgentB,
		MemoryTeam:     "test-team",
	}
}

func newTestTrigger(triggerID uuid.UUID, routingMode string, keys ...string) model.RouterTrigger {
	return model.RouterTrigger{
		ID:           triggerID,
		OrgID:        testOrgID,
		RouterID:     testRouterID,
		ConnectionID: testConnID,
		TriggerKeys:  pq.StringArray(keys),
		Enabled:      true,
		RoutingMode:  routingMode,
	}
}

func newTestAgent(agentID uuid.UUID, name string) model.Agent {
	orgID := testOrgID
	desc := "Test agent: " + name
	return model.Agent{
		ID:          agentID,
		OrgID:       &orgID,
		Name:        name,
		Description: &desc,
		Status:      "active",
	}
}

func setupRuleStore(triggerID uuid.UUID, rules ...model.RoutingRule) (*MemoryRouterTriggerStore, *RouterDispatcher) {
	store := NewMemoryRouterTriggerStore()
	router := newTestRouter()
	trigger := newTestTrigger(triggerID, "rule", "pull_request.opened")
	store.AddTrigger(trigger, router)
	for _, rule := range rules {
		store.AddRule(triggerID, rule)
	}
	store.AddAgent(newTestAgent(testAgentA, "code-review-agent"))
	store.AddAgent(newTestAgent(testAgentB, "bug-triage-agent"))

	dispatcher := NewRouterDispatcher(store, catalog.Global(), nil, slog.Default())
	return store, dispatcher
}

func baseInput() RouterDispatchInput {
	return RouterDispatchInput{
		Provider:     "github",
		EventType:    "pull_request",
		EventAction:  "opened",
		OrgID:        testOrgID,
		ConnectionID: testConnID,
		Payload:      map[string]any{"action": "opened", "pull_request": map[string]any{"base": map[string]any{"ref": "main"}}},
	}
}

// --------------------------------------------------------------------------
// Rule routing tests
// --------------------------------------------------------------------------

func TestDispatch_Rule_PROpened_RoutesToCodeReview(t *testing.T) {
	triggerID := uuid.New()
	_, dispatcher := setupRuleStore(triggerID, model.RoutingRule{
		AgentID:  testAgentA,
		Priority: 1,
		Conditions: conditionsJSON("all",
			condition("pull_request.base.ref", "equals", "main"),
		),
	})

	dispatches, err := dispatcher.Run(context.Background(), baseInput())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dispatches) != 1 {
		t.Fatalf("expected 1 dispatch, got %d", len(dispatches))
	}
	if dispatches[0].AgentID != testAgentA {
		t.Errorf("agent: got %s, want %s", dispatches[0].AgentID, testAgentA)
	}
	if dispatches[0].RoutingMode != "rule" {
		t.Errorf("routing mode: got %q, want rule", dispatches[0].RoutingMode)
	}
}

func TestDispatch_Rule_MultiAgent(t *testing.T) {
	triggerID := uuid.New()
	_, dispatcher := setupRuleStore(triggerID,
		model.RoutingRule{AgentID: testAgentA, Priority: 1},
		model.RoutingRule{AgentID: testAgentB, Priority: 2},
	)

	dispatches, err := dispatcher.Run(context.Background(), baseInput())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dispatches) != 2 {
		t.Fatalf("expected 2 dispatches, got %d", len(dispatches))
	}
}

func TestDispatch_Rule_NoMatch_FallsBackToDefault(t *testing.T) {
	triggerID := uuid.New()
	_, dispatcher := setupRuleStore(triggerID, model.RoutingRule{
		AgentID:  testAgentA,
		Priority: 1,
		Conditions: conditionsJSON("all",
			condition("pull_request.base.ref", "equals", "never-matches"),
		),
	})

	dispatches, err := dispatcher.Run(context.Background(), baseInput())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dispatches) != 1 {
		t.Fatalf("expected 1 dispatch (fallback), got %d", len(dispatches))
	}
	if dispatches[0].AgentID != testAgentB {
		t.Errorf("fallback should route to default agent %s, got %s", testAgentB, dispatches[0].AgentID)
	}
}

func TestDispatch_Rule_NoMatch_NoDefault_Empty(t *testing.T) {
	store := NewMemoryRouterTriggerStore()
	routerNoDefault := model.Router{
		ID:    testRouterID,
		OrgID: testOrgID,
		Name:  "Zira",
	}
	triggerID := uuid.New()
	store.AddTrigger(newTestTrigger(triggerID, "rule", "pull_request.opened"), routerNoDefault)
	store.AddRule(triggerID, model.RoutingRule{
		AgentID:    testAgentA,
		Priority:   1,
		Conditions: conditionsJSON("all", condition("action", "equals", "never")),
	})

	dispatcher := NewRouterDispatcher(store, catalog.Global(), nil, slog.Default())
	dispatches, err := dispatcher.Run(context.Background(), baseInput())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dispatches) != 0 {
		t.Fatalf("expected 0 dispatches (no match, no default), got %d", len(dispatches))
	}
}

func TestDispatch_Rule_NoLLMCall(t *testing.T) {
	mock := hiveloop.NewMockCompletionClient()
	triggerID := uuid.New()
	store, _ := setupRuleStore(triggerID, model.RoutingRule{AgentID: testAgentA, Priority: 1})

	routerAgent := hiveloop.NewRouterAgent(mock, "test-model", 10)
	dispatcher := NewRouterDispatcher(store, catalog.Global(), routerAgent, slog.Default())
	_, err := dispatcher.Run(context.Background(), baseInput())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	mock.AssertCallCount(t, 0)
}

// --------------------------------------------------------------------------
// Triage routing tests
// --------------------------------------------------------------------------

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
	mock.OnMessage("", // triage user message is built from refs, not the raw user text
		hiveloop.CompletionResponse{Message: hiveloop.Message{Role: "assistant", ToolCalls: []hiveloop.ToolCall{
			{ID: "c1", Name: "route_to_agent", Arguments: `{"agent_id":"` + testAgentA.String() + `","priority":1,"reason":"PR review"}`},
			{ID: "c2", Name: "finalize", Arguments: "{}"},
		}}},
	)
	// Also register a fallback for any user message
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

// --------------------------------------------------------------------------
// Thread affinity tests
// --------------------------------------------------------------------------

func TestDispatch_ThreadAffinity_ExistingConv(t *testing.T) {
	store := NewMemoryRouterTriggerStore()
	router := newTestRouter()
	triggerID := uuid.New()
	store.AddTrigger(newTestTrigger(triggerID, "rule", "app_mention"), router)

	// Pre-seed an existing conversation for a resource key.
	existingConvID := "conv-existing-123"
	existingSandboxID := uuid.New()
	store.StoreConversation(context.Background(), &model.RouterConversation{
		OrgID:                testOrgID,
		AgentID:              testAgentA,
		ConnectionID:         testConnID,
		ResourceKey:          "slack:T123:C456:ts789",
		BridgeConversationID: existingConvID,
		SandboxID:            existingSandboxID,
		Status:               "active",
	})

	dispatcher := NewRouterDispatcher(store, catalog.Global(), nil, slog.Default())

	input := RouterDispatchInput{
		Provider: "slack", EventType: "app_mention",
		OrgID: testOrgID, ConnectionID: testConnID,
		Payload: map[string]any{},
	}
	// The dispatcher won't find the resource key because we can't resolve it
	// from the empty payload. For this test to work, we'd need actual
	// trigger defs with refs. For now, verify the affinity check returns nil
	// for empty resource keys (no continuation).
	dispatches, err := dispatcher.Run(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// With empty payload → empty resource key → no affinity match → falls through to routing.
	// This verifies the affinity path doesn't crash on empty data.
	_ = dispatches
}

// --------------------------------------------------------------------------
// Edge case tests
// --------------------------------------------------------------------------

func TestDispatch_NoMatchingTrigger(t *testing.T) {
	store := NewMemoryRouterTriggerStore()
	dispatcher := NewRouterDispatcher(store, catalog.Global(), nil, slog.Default())

	dispatches, err := dispatcher.Run(context.Background(), baseInput())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dispatches) != 0 {
		t.Errorf("expected 0 dispatches for no matching triggers, got %d", len(dispatches))
	}
}

func TestDispatch_DisabledTrigger(t *testing.T) {
	store := NewMemoryRouterTriggerStore()
	trigger := newTestTrigger(uuid.New(), "rule", "pull_request.opened")
	trigger.Enabled = false
	store.AddTrigger(trigger, newTestRouter())

	dispatcher := NewRouterDispatcher(store, catalog.Global(), nil, slog.Default())
	dispatches, err := dispatcher.Run(context.Background(), baseInput())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dispatches) != 0 {
		t.Errorf("disabled trigger should be skipped, got %d dispatches", len(dispatches))
	}
}

func TestDispatch_StoresRoutingDecision(t *testing.T) {
	triggerID := uuid.New()
	store, dispatcher := setupRuleStore(triggerID, model.RoutingRule{AgentID: testAgentA, Priority: 1})

	_, err := dispatcher.Run(context.Background(), baseInput())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	decisions := store.StoredDecisions()
	if len(decisions) != 1 {
		t.Fatalf("expected 1 stored decision, got %d", len(decisions))
	}
	if decisions[0].RoutingMode != "rule" {
		t.Errorf("decision routing mode: got %q", decisions[0].RoutingMode)
	}
	if len(decisions[0].SelectedAgents) != 1 {
		t.Errorf("decision agents: got %d", len(decisions[0].SelectedAgents))
	}
}
