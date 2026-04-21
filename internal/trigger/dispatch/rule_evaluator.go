package dispatch

import (
	"encoding/json"
	"sort"

	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/trigger/hiveloop"
)

// EvaluateRules runs deterministic rule evaluation against the webhook payload.
// Returns all matching rules' agents sorted by priority (ascending = highest
// priority first). Multiple rules can match the same event, enabling
// multi-agent dispatch from a single trigger.
func EvaluateRules(rules []model.RoutingRule, payload map[string]any) []hiveloop.AgentSelection {
	var matches []hiveloop.AgentSelection

	for _, rule := range rules {
		if ruleMatches(rule, payload) {
			matches = append(matches, hiveloop.AgentSelection{
				AgentID:  rule.AgentID,
				Priority: rule.Priority,
				Reason:   "deterministic rule match",
			})
		}
	}

	sort.Slice(matches, func(indexA, indexB int) bool {
		return matches[indexA].Priority < matches[indexB].Priority
	})

	return matches
}

// ruleMatches evaluates a single rule's conditions against the payload.
// A rule with nil/empty conditions is a catch-all (always matches).
func ruleMatches(rule model.RoutingRule, payload map[string]any) bool {
	if len(rule.Conditions) == 0 {
		return true
	}

	var match model.TriggerMatch
	if err := json.Unmarshal(rule.Conditions, &match); err != nil {
		return false
	}

	_, passed := evaluateConditions(&match, payload)
	return passed
}
