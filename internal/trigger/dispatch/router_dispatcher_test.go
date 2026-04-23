package dispatch

import (
	"context"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/usehiveloop/hiveloop/internal/mcp/catalog"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/trigger/hiveloop"
)

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
