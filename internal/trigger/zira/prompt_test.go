package zira

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/ziraloop/ziraloop/internal/model"
)

func TestPrompt_IncludesAllAgents(t *testing.T) {
	agents := testAgents()
	prompt := BuildRoutingPrompt("", agents, nil, nil)

	for _, agent := range agents {
		if !strings.Contains(prompt, agent.Name) {
			t.Errorf("prompt should contain agent name %q", agent.Name)
		}
		if !strings.Contains(prompt, agent.ID.String()) {
			t.Errorf("prompt should contain agent ID %s", agent.ID)
		}
	}
}

func TestPrompt_IncludesConnectionSchemas(t *testing.T) {
	connections := testConnections()
	prompt := BuildRoutingPrompt("", nil, connections, nil)

	if !strings.Contains(prompt, "github-app") {
		t.Error("prompt should contain github-app provider")
	}
	if !strings.Contains(prompt, "pulls_get") {
		t.Error("prompt should contain pulls_get action")
	}
	if !strings.Contains(prompt, "conversations_replies") {
		t.Error("prompt should contain conversations_replies action")
	}
	if !strings.Contains(prompt, "pull_number") {
		t.Error("prompt should contain param names from action schema")
	}
}

func TestPrompt_IncludesRecentDecisions(t *testing.T) {
	decisions := []model.RoutingDecision{
		{
			EventType:      "app_mention",
			IntentSummary:  "review PR for auth issues",
			SelectedAgents: pq.StringArray{"code-review-agent"},
		},
		{
			EventType:      "issues.opened",
			IntentSummary:  "triage new bug report",
			SelectedAgents: pq.StringArray{"bug-triage-agent"},
		},
	}
	prompt := BuildRoutingPrompt("", nil, nil, decisions)

	if !strings.Contains(prompt, "review PR for auth issues") {
		t.Error("prompt should contain intent from recent decision")
	}
	if !strings.Contains(prompt, "code-review-agent") {
		t.Error("prompt should contain selected agent from recent decision")
	}
	if !strings.Contains(prompt, "Recent Routing Decisions") {
		t.Error("prompt should have recent decisions section header")
	}
}

func TestPrompt_EmptyAgents(t *testing.T) {
	prompt := BuildRoutingPrompt("", nil, nil, nil)

	if !strings.Contains(prompt, "No agents configured") {
		t.Error("prompt should say no agents configured")
	}
	if !strings.Contains(prompt, "route_to_agent") {
		t.Error("prompt should still contain tool descriptions")
	}
}

func TestPrompt_EmptyConnections(t *testing.T) {
	prompt := BuildRoutingPrompt("", testAgents(), nil, nil)

	if !strings.Contains(prompt, "No additional connections") {
		t.Error("prompt should say no connections available")
	}
}

func TestPrompt_LargeAgentCatalog(t *testing.T) {
	agents := make([]model.Agent, 200)
	for index := range agents {
		desc := "Agent description for testing"
		agents[index] = model.Agent{
			ID:          uuid.New(),
			Name:        "agent-" + uuid.New().String()[:8],
			Description: &desc,
		}
	}
	prompt := BuildRoutingPrompt("", agents, testConnections(), nil)

	agentCount := strings.Count(prompt, "agent-")
	if agentCount < 200 {
		t.Errorf("expected 200 agents in prompt, counted %d", agentCount)
	}
}
