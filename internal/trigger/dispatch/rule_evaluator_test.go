package dispatch

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/model"
)

func conditionsJSON(mode string, conditions ...map[string]any) model.RawJSON {
	match := map[string]any{"mode": mode, "conditions": conditions}
	data, _ := json.Marshal(match)
	return data
}

func condition(path, operator string, value any) map[string]any {
	cond := map[string]any{"path": path, "operator": operator}
	if value != nil {
		cond["value"] = value
	}
	return cond
}

var (
	agentA = uuid.MustParse("aaaaaaaa-0000-0000-0000-000000000001")
	agentB = uuid.MustParse("aaaaaaaa-0000-0000-0000-000000000002")
	agentC = uuid.MustParse("aaaaaaaa-0000-0000-0000-000000000003")
)

func TestRuleEvaluator_SingleRuleMatch(t *testing.T) {
	rules := []model.RoutingRule{
		{AgentID: agentA, Priority: 1, Conditions: conditionsJSON("all",
			condition("pull_request.base.ref", "equals", "main"),
		)},
	}
	payload := map[string]any{"pull_request": map[string]any{"base": map[string]any{"ref": "main"}}}

	matches := EvaluateRules(rules, payload)
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].AgentID != agentA {
		t.Errorf("agent: got %s, want %s", matches[0].AgentID, agentA)
	}
}

func TestRuleEvaluator_MultipleRulesMatch(t *testing.T) {
	rules := []model.RoutingRule{
		{AgentID: agentA, Priority: 1, Conditions: conditionsJSON("all",
			condition("pull_request.base.ref", "equals", "main"),
		)},
		{AgentID: agentB, Priority: 2, Conditions: conditionsJSON("all",
			condition("pull_request.base.ref", "equals", "main"),
		)},
		{AgentID: agentC, Priority: 1, Conditions: conditionsJSON("all",
			condition("pull_request.base.ref", "equals", "develop"),
		)},
	}
	payload := map[string]any{"pull_request": map[string]any{"base": map[string]any{"ref": "main"}}}

	matches := EvaluateRules(rules, payload)
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches (A and B), got %d", len(matches))
	}
	if matches[0].AgentID != agentA {
		t.Errorf("first match should be agentA (priority 1), got %s", matches[0].AgentID)
	}
	if matches[1].AgentID != agentB {
		t.Errorf("second match should be agentB (priority 2), got %s", matches[1].AgentID)
	}
}

func TestRuleEvaluator_NoRulesMatch(t *testing.T) {
	rules := []model.RoutingRule{
		{AgentID: agentA, Priority: 1, Conditions: conditionsJSON("all",
			condition("pull_request.base.ref", "equals", "main"),
		)},
	}
	payload := map[string]any{"pull_request": map[string]any{"base": map[string]any{"ref": "develop"}}}

	matches := EvaluateRules(rules, payload)
	if len(matches) != 0 {
		t.Fatalf("expected 0 matches, got %d", len(matches))
	}
}

func TestRuleEvaluator_PriorityOrdering(t *testing.T) {
	rules := []model.RoutingRule{
		{AgentID: agentC, Priority: 3},
		{AgentID: agentA, Priority: 1},
		{AgentID: agentB, Priority: 2},
	}
	payload := map[string]any{}

	matches := EvaluateRules(rules, payload)
	if len(matches) != 3 {
		t.Fatalf("expected 3 matches (all catch-all), got %d", len(matches))
	}
	if matches[0].Priority != 1 || matches[1].Priority != 2 || matches[2].Priority != 3 {
		t.Errorf("priority ordering wrong: %d, %d, %d", matches[0].Priority, matches[1].Priority, matches[2].Priority)
	}
}

func TestRuleEvaluator_NilConditions_AlwaysMatches(t *testing.T) {
	rules := []model.RoutingRule{
		{AgentID: agentA, Priority: 1, Conditions: nil},
	}
	payload := map[string]any{"anything": "here"}

	matches := EvaluateRules(rules, payload)
	if len(matches) != 1 {
		t.Fatalf("nil conditions should match everything, got %d matches", len(matches))
	}
}

func TestRuleEvaluator_ComplexConditions_AllMode(t *testing.T) {
	rules := []model.RoutingRule{
		{AgentID: agentA, Priority: 1, Conditions: conditionsJSON("all",
			condition("pull_request.base.ref", "equals", "main"),
			condition("pull_request.draft", "equals", false),
		)},
	}
	// Both conditions met.
	payload := map[string]any{"pull_request": map[string]any{"base": map[string]any{"ref": "main"}, "draft": false}}
	matches := EvaluateRules(rules, payload)
	if len(matches) != 1 {
		t.Fatalf("both conditions met, expected 1 match, got %d", len(matches))
	}

	// One condition fails.
	payload["pull_request"].(map[string]any)["draft"] = true
	matches = EvaluateRules(rules, payload)
	if len(matches) != 0 {
		t.Fatalf("one condition fails in ALL mode, expected 0 matches, got %d", len(matches))
	}
}

func TestRuleEvaluator_ComplexConditions_AnyMode(t *testing.T) {
	rules := []model.RoutingRule{
		{AgentID: agentA, Priority: 1, Conditions: conditionsJSON("any",
			condition("pull_request.base.ref", "equals", "main"),
			condition("pull_request.base.ref", "equals", "develop"),
		)},
	}
	payload := map[string]any{"pull_request": map[string]any{"base": map[string]any{"ref": "develop"}}}

	matches := EvaluateRules(rules, payload)
	if len(matches) != 1 {
		t.Fatalf("one of two conditions met in ANY mode, expected 1 match, got %d", len(matches))
	}
}

func TestRuleEvaluator_MixedMatchAndMiss(t *testing.T) {
	rules := []model.RoutingRule{
		{AgentID: agentA, Priority: 1, Conditions: conditionsJSON("all", condition("action", "equals", "opened"))},
		{AgentID: agentB, Priority: 2, Conditions: conditionsJSON("all", condition("action", "equals", "closed"))},
		{AgentID: agentC, Priority: 1, Conditions: nil}, // catch-all
	}
	payload := map[string]any{"action": "opened"}

	matches := EvaluateRules(rules, payload)
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches (A + C), got %d", len(matches))
	}
	// Both priority 1 — sorted by insertion order (agentA first, agentC second).
	if matches[0].AgentID != agentA {
		t.Errorf("first match: got %s, want agentA", matches[0].AgentID)
	}
	if matches[1].AgentID != agentC {
		t.Errorf("second match: got %s, want agentC", matches[1].AgentID)
	}
}
