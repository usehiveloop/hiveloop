package executor

import (
	"testing"

	"github.com/google/uuid"

	"github.com/ziraloop/ziraloop/internal/trigger/dispatch"
)

func TestGroupByPriority_SinglePriority(t *testing.T) {
	dispatches := []dispatch.AgentDispatch{
		{AgentID: uuid.New(), Priority: 1},
		{AgentID: uuid.New(), Priority: 1},
	}
	groups := groupByPriority(dispatches)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if len(groups[0]) != 2 {
		t.Errorf("expected 2 dispatches in group, got %d", len(groups[0]))
	}
}

func TestGroupByPriority_TwoPriorities(t *testing.T) {
	dispatches := []dispatch.AgentDispatch{
		{AgentID: uuid.New(), Priority: 2},
		{AgentID: uuid.New(), Priority: 1},
		{AgentID: uuid.New(), Priority: 1},
	}
	groups := groupByPriority(dispatches)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	if len(groups[0]) != 2 {
		t.Errorf("first group (priority 1): expected 2, got %d", len(groups[0]))
	}
	if len(groups[1]) != 1 {
		t.Errorf("second group (priority 2): expected 1, got %d", len(groups[1]))
	}
}

func TestGroupByPriority_Empty(t *testing.T) {
	groups := groupByPriority(nil)
	if len(groups) != 0 {
		t.Errorf("expected 0 groups for nil input, got %d", len(groups))
	}
}

func TestBuildInstructions_IncludesPersona(t *testing.T) {
	agentDispatch := dispatch.AgentDispatch{
		RouterPersona: "You are Zira, a helpful AI teammate.",
		Refs:          map[string]string{"channel": "C123", "user": "U456"},
	}
	instructions := buildInstructions(agentDispatch)
	if len(instructions) == 0 {
		t.Fatal("instructions should not be empty")
	}
	if !containsString(instructions, "You are Zira") {
		t.Error("instructions should contain persona")
	}
	if !containsString(instructions, "C123") {
		t.Error("instructions should contain refs")
	}
}

func TestBuildInstructions_NoPersona(t *testing.T) {
	agentDispatch := dispatch.AgentDispatch{
		Refs: map[string]string{"channel": "C123"},
	}
	instructions := buildInstructions(agentDispatch)
	if containsString(instructions, "---") {
		t.Error("instructions without persona should not have separator")
	}
}

func TestBuildInstructions_WithEnrichedMessage(t *testing.T) {
	agentDispatch := dispatch.AgentDispatch{
		RouterPersona:   "You are Zira.",
		Refs:            map[string]string{"channel": "C123"},
		EnrichedMessage: "## PR #87: Auth refactor\n\nFull context here.",
	}
	instructions := buildInstructions(agentDispatch)
	if !containsString(instructions, "PR #87: Auth refactor") {
		t.Error("instructions should contain enriched message")
	}
	if !containsString(instructions, "You are Zira") {
		t.Error("instructions should still contain persona")
	}
	if containsString(instructions, "C123") {
		t.Error("instructions should not contain flat refs when enriched message is present")
	}
}

func TestBuildInstructions_FallsBackToRefsWithoutEnrichment(t *testing.T) {
	agentDispatch := dispatch.AgentDispatch{
		Refs: map[string]string{"channel": "C123"},
	}
	instructions := buildInstructions(agentDispatch)
	if !containsString(instructions, "C123") {
		t.Error("instructions should contain flat refs when no enrichment")
	}
}

func TestBuildContinuationMessage(t *testing.T) {
	agentDispatch := dispatch.AgentDispatch{
		Refs: map[string]string{"text": "hello again", "user": "U789"},
	}
	message := buildContinuationMessage(agentDispatch)
	if !containsString(message, "New event") {
		t.Error("continuation message should indicate new event")
	}
	if !containsString(message, "hello again") {
		t.Error("continuation message should contain refs")
	}
}

func containsString(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle || len(haystack) > 0 && findSubstring(haystack, needle))
}

func findSubstring(haystack, needle string) bool {
	for index := 0; index <= len(haystack)-len(needle); index++ {
		if haystack[index:index+len(needle)] == needle {
			return true
		}
	}
	return false
}
