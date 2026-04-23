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
	connID := testConnID
	return model.RouterTrigger{
		ID:           triggerID,
		OrgID:        testOrgID,
		RouterID:     testRouterID,
		ConnectionID: &connID,
		TriggerKeys:  pq.StringArray(keys),
		Enabled:      true,
		TriggerType:  "webhook",
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
// --------------------------------------------------------------------------
// RunForTrigger tests (HTTP / cron triggers)
// --------------------------------------------------------------------------

func newHTTPTrigger(triggerID uuid.UUID) model.RouterTrigger {
	return model.RouterTrigger{
		ID:           triggerID,
		OrgID:        testOrgID,
		RouterID:     testRouterID,
		TriggerType:  "http",
		RoutingMode:  "rule",
		Enabled:      true,
		Instructions: "You received an HTTP webhook. Process the payload.",
	}
}

func newCronTrigger(triggerID uuid.UUID) model.RouterTrigger {
	return model.RouterTrigger{
		ID:           triggerID,
		OrgID:        testOrgID,
		RouterID:     testRouterID,
		TriggerType:  "cron",
		CronSchedule: "0 9 * * *",
		RoutingMode:  "rule",
		Enabled:      true,
		Instructions: "Run your daily check.",
	}
}

func setupDirectTriggerStore(trigger model.RouterTrigger, rules ...model.RoutingRule) (*MemoryRouterTriggerStore, *RouterDispatcher) {
	store := NewMemoryRouterTriggerStore()
	router := newTestRouter()
	store.AddTrigger(trigger, router)
	for _, rule := range rules {
		store.AddRule(trigger.ID, rule)
	}
	store.AddAgent(newTestAgent(testAgentA, "code-review-agent"))
	store.AddAgent(newTestAgent(testAgentB, "bug-triage-agent"))

	dispatcher := NewRouterDispatcher(store, catalog.Global(), nil, slog.Default())
	return store, dispatcher
}

func TestRunForTrigger_HTTP_RoutesToMatchingAgent(t *testing.T) {
	triggerID := uuid.New()
	_, dispatcher := setupDirectTriggerStore(
		newHTTPTrigger(triggerID),
		model.RoutingRule{AgentID: testAgentA, Priority: 1},
	)

	payload := map[string]any{"action": "deploy", "environment": "production"}
	dispatches, err := dispatcher.RunForTrigger(context.Background(), triggerID, payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dispatches) != 1 {
		t.Fatalf("expected 1 dispatch, got %d", len(dispatches))
	}
	if dispatches[0].AgentID != testAgentA {
		t.Errorf("agent: got %s, want %s", dispatches[0].AgentID, testAgentA)
	}
	if dispatches[0].RunIntent != "normal" {
		t.Errorf("intent: got %q, want normal", dispatches[0].RunIntent)
	}
}

func TestRunForTrigger_HTTP_InjectsTriggerInstructions(t *testing.T) {
	triggerID := uuid.New()
	_, dispatcher := setupDirectTriggerStore(
		newHTTPTrigger(triggerID),
		model.RoutingRule{AgentID: testAgentA, Priority: 1},
	)

	dispatches, err := dispatcher.RunForTrigger(context.Background(), triggerID, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dispatches) != 1 {
		t.Fatalf("expected 1 dispatch, got %d", len(dispatches))
	}
	if dispatches[0].TriggerInstructions != "You received an HTTP webhook. Process the payload." {
		t.Errorf("trigger instructions: got %q", dispatches[0].TriggerInstructions)
	}
}

func TestRunForTrigger_HTTP_WithConditions_MatchingPayload(t *testing.T) {
	triggerID := uuid.New()
	_, dispatcher := setupDirectTriggerStore(
		newHTTPTrigger(triggerID),
		model.RoutingRule{
			AgentID:  testAgentA,
			Priority: 1,
			Conditions: conditionsJSON("all",
				condition("environment", "equals", "production"),
			),
		},
	)

	payload := map[string]any{"environment": "production", "service": "api"}
	dispatches, err := dispatcher.RunForTrigger(context.Background(), triggerID, payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dispatches) != 1 {
		t.Fatalf("expected 1 dispatch for matching condition, got %d", len(dispatches))
	}
	if dispatches[0].AgentID != testAgentA {
		t.Errorf("agent: got %s, want %s", dispatches[0].AgentID, testAgentA)
	}
}

func TestRunForTrigger_HTTP_WithConditions_NonMatchingPayload(t *testing.T) {
	triggerID := uuid.New()
	_, dispatcher := setupDirectTriggerStore(
		newHTTPTrigger(triggerID),
		model.RoutingRule{
			AgentID:  testAgentA,
			Priority: 1,
			Conditions: conditionsJSON("all",
				condition("environment", "equals", "production"),
			),
		},
	)

	// Payload doesn't match — environment is staging.
	payload := map[string]any{"environment": "staging"}
	dispatches, err := dispatcher.RunForTrigger(context.Background(), triggerID, payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should fall back to default agent (testAgentB) since no rules matched.
	if len(dispatches) != 1 {
		t.Fatalf("expected 1 dispatch (fallback), got %d", len(dispatches))
	}
	if dispatches[0].AgentID != testAgentB {
		t.Errorf("fallback: got %s, want default agent %s", dispatches[0].AgentID, testAgentB)
	}
}

func TestRunForTrigger_HTTP_MultiAgent(t *testing.T) {
	triggerID := uuid.New()
	_, dispatcher := setupDirectTriggerStore(
		newHTTPTrigger(triggerID),
		model.RoutingRule{AgentID: testAgentA, Priority: 1},
		model.RoutingRule{AgentID: testAgentB, Priority: 2},
	)

	dispatches, err := dispatcher.RunForTrigger(context.Background(), triggerID, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dispatches) != 2 {
		t.Fatalf("expected 2 dispatches, got %d", len(dispatches))
	}
	// Both agents should have trigger instructions.
	for _, dispatch := range dispatches {
		if dispatch.TriggerInstructions == "" {
			t.Errorf("dispatch for agent %s missing trigger instructions", dispatch.AgentID)
		}
	}
}

func TestRunForTrigger_HTTP_NilPayload_UsesEmptyMap(t *testing.T) {
	triggerID := uuid.New()
	_, dispatcher := setupDirectTriggerStore(
		newHTTPTrigger(triggerID),
		model.RoutingRule{AgentID: testAgentA, Priority: 1}, // catch-all
	)

	// Pass nil payload — should not panic and should use empty map.
	dispatches, err := dispatcher.RunForTrigger(context.Background(), triggerID, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dispatches) != 1 {
		t.Fatalf("expected 1 dispatch, got %d", len(dispatches))
	}
}

func TestRunForTrigger_Cron_RoutesToAgent(t *testing.T) {
	triggerID := uuid.New()
	_, dispatcher := setupDirectTriggerStore(
		newCronTrigger(triggerID),
		model.RoutingRule{AgentID: testAgentA, Priority: 1},
	)

	dispatches, err := dispatcher.RunForTrigger(context.Background(), triggerID, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dispatches) != 1 {
		t.Fatalf("expected 1 dispatch, got %d", len(dispatches))
	}
	if dispatches[0].AgentID != testAgentA {
		t.Errorf("agent: got %s, want %s", dispatches[0].AgentID, testAgentA)
	}
	if dispatches[0].TriggerInstructions != "Run your daily check." {
		t.Errorf("instructions: got %q", dispatches[0].TriggerInstructions)
	}
}

func TestRunForTrigger_Cron_StoresRoutingDecision(t *testing.T) {
	triggerID := uuid.New()
	store, dispatcher := setupDirectTriggerStore(
		newCronTrigger(triggerID),
		model.RoutingRule{AgentID: testAgentA, Priority: 1},
	)

	_, err := dispatcher.RunForTrigger(context.Background(), triggerID, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	decisions := store.StoredDecisions()
	if len(decisions) != 1 {
		t.Fatalf("expected 1 stored decision, got %d", len(decisions))
	}
	if decisions[0].EventType != "cron" {
		t.Errorf("decision event type: got %q, want cron", decisions[0].EventType)
	}
	if decisions[0].RouterTriggerID != triggerID {
		t.Errorf("decision trigger ID: got %s, want %s", decisions[0].RouterTriggerID, triggerID)
	}
}

func TestRunForTrigger_DisabledTrigger_ReturnsError(t *testing.T) {
	triggerID := uuid.New()
	trigger := newHTTPTrigger(triggerID)
	trigger.Enabled = false

	store := NewMemoryRouterTriggerStore()
	store.AddTrigger(trigger, newTestRouter())

	dispatcher := NewRouterDispatcher(store, catalog.Global(), nil, slog.Default())
	_, err := dispatcher.RunForTrigger(context.Background(), triggerID, nil)
	if err == nil {
		t.Fatal("expected error for disabled trigger, got nil")
	}
}

func TestRunForTrigger_UnknownTrigger_ReturnsError(t *testing.T) {
	store := NewMemoryRouterTriggerStore()
	dispatcher := NewRouterDispatcher(store, catalog.Global(), nil, slog.Default())

	unknownID := uuid.New()
	_, err := dispatcher.RunForTrigger(context.Background(), unknownID, nil)
	if err == nil {
		t.Fatal("expected error for unknown trigger, got nil")
	}
}

func TestRunForTrigger_HTTP_NoRulesNoDefault_Empty(t *testing.T) {
	triggerID := uuid.New()
	trigger := newHTTPTrigger(triggerID)

	store := NewMemoryRouterTriggerStore()
	routerNoDefault := model.Router{
		ID:    testRouterID,
		OrgID: testOrgID,
		Name:  "Zira",
	}
	store.AddTrigger(trigger, routerNoDefault)
	// No rules and no default agent.
	store.AddRule(triggerID, model.RoutingRule{
		AgentID:    testAgentA,
		Priority:   1,
		Conditions: conditionsJSON("all", condition("action", "equals", "never")),
	})

	dispatcher := NewRouterDispatcher(store, catalog.Global(), nil, slog.Default())
	dispatches, err := dispatcher.RunForTrigger(context.Background(), triggerID, map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dispatches) != 0 {
		t.Fatalf("expected 0 dispatches (no match, no default), got %d", len(dispatches))
	}
}

func TestRunForTrigger_Cron_NoInstructions_EmptyField(t *testing.T) {
	triggerID := uuid.New()
	trigger := newCronTrigger(triggerID)
	trigger.Instructions = "" // no per-trigger instructions

	store, dispatcher := setupDirectTriggerStore(
		trigger,
		model.RoutingRule{AgentID: testAgentA, Priority: 1},
	)

	dispatches, err := dispatcher.RunForTrigger(context.Background(), triggerID, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dispatches) != 1 {
		t.Fatalf("expected 1 dispatch, got %d", len(dispatches))
	}
	if dispatches[0].TriggerInstructions != "" {
		t.Errorf("expected empty trigger instructions, got %q", dispatches[0].TriggerInstructions)
	}

	// Decision should still be stored.
	decisions := store.StoredDecisions()
	if len(decisions) != 1 {
		t.Fatalf("expected 1 stored decision, got %d", len(decisions))
	}
}
