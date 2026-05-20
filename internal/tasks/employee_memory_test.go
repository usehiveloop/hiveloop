package tasks

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func TestBuildEmployeeRetainItem_BundlesSessionAtCheckpoint(t *testing.T) {
	orgID := uuid.New()
	agentID := uuid.New()
	sandboxID := uuid.New()
	agent := &model.Agent{ID: agentID, OrgID: &orgID, Name: "Aria"}
	events := []model.EmployeeMemoryEvent{
		memoryEvent(t, orgID, agentID, sandboxID, "S1", "user.message.received", map[string]any{
			"source": "slack", "channel": "C123", "user": "U123", "user_display_name": "Kim",
			"text": "The Platform team requires rollback notes before deploys.",
		}),
		memoryEvent(t, orgID, agentID, sandboxID, "S1", "tool.invoked", map[string]any{
			"source": "slack", "tool": "bash", "result_summary": "Checked deployment docs.",
		}),
		memoryEvent(t, orgID, agentID, sandboxID, "S1", "agent.message.sent", map[string]any{
			"source": "slack", "text": "Done. I added rollback notes to the deploy plan.",
		}),
	}

	item, ok := buildEmployeeRetainItem(agent, EmployeeMemoryRetainPayload{
		AgentID: agentID, SandboxID: sandboxID, SessionID: "S1", SourceEvent: "agent.message.sent",
	}, events)
	if !ok {
		t.Fatal("expected retain item")
	}
	for _, want := range []string{
		"company:" + orgID.String(),
		"source:slack",
		"visibility:company",
		"memory_type:company_context",
		"channel:c123",
	} {
		if !hasTaskString(item.Tags, want) {
			t.Fatalf("missing tag %q in %#v", want, item.Tags)
		}
	}
	if !strings.Contains(item.Content, "rollback notes") || !strings.Contains(item.Content, "Employee Aria") {
		t.Fatalf("unexpected content: %q", item.Content)
	}
	if !strings.Contains(item.Content, "Teammate Kim (<@U123>)") {
		t.Fatalf("retain content should preserve Slack user identity: %q", item.Content)
	}
	if !strings.Contains(item.Context, "stable channel user IDs") || !strings.Contains(item.Content, "Do not retain active-conversation framing") {
		t.Fatalf("retain instructions should distinguish people facts from session state: context=%q content=%q", item.Context, item.Content)
	}
	if strings.Contains(item.Content, "Tool ") || strings.Contains(item.Content, "bash") || strings.Contains(item.Content, "Checked deployment docs") {
		t.Fatalf("retain content should not include raw tool calls: %q", item.Content)
	}
	if item.Metadata["session_id"] != "S1" || item.Metadata["event_count"] != "3" || item.Metadata["user"] != "U123" || item.Metadata["user_display_name"] != "Kim" {
		t.Fatalf("unexpected metadata: %#v", item.Metadata)
	}
	if len(item.ObservationScopes) != 1 {
		t.Fatalf("expected company observation scope, got %#v", item.ObservationScopes)
	}
}

func TestBuildEmployeeRetainItem_SkipsNoCheckpointAndSecrets(t *testing.T) {
	orgID := uuid.New()
	agentID := uuid.New()
	sandboxID := uuid.New()
	agent := &model.Agent{ID: agentID, OrgID: &orgID, Name: "Aria"}
	onlyUser := []model.EmployeeMemoryEvent{
		memoryEvent(t, orgID, agentID, sandboxID, "S1", "user.message.received", map[string]any{"text": "remember this later"}),
	}
	if _, ok := buildEmployeeRetainItem(agent, EmployeeMemoryRetainPayload{AgentID: agentID, SandboxID: sandboxID, SessionID: "S1"}, onlyUser); ok {
		t.Fatal("user event without checkpoint should not retain")
	}
	withSecret := append(onlyUser, memoryEvent(t, orgID, agentID, sandboxID, "S1", "agent.message.sent", map[string]any{"text": "api_key=sk-secret"}))
	if _, ok := buildEmployeeRetainItem(agent, EmployeeMemoryRetainPayload{AgentID: agentID, SandboxID: sandboxID, SessionID: "S1"}, withSecret); ok {
		t.Fatal("secret-looking transcript should not retain")
	}
}

func TestBuildEmployeeRetainItem_SkipsPureBanterWithoutWorkSignal(t *testing.T) {
	orgID := uuid.New()
	agentID := uuid.New()
	sandboxID := uuid.New()
	agent := &model.Agent{ID: agentID, OrgID: &orgID, Name: "Aria"}
	events := []model.EmployeeMemoryEvent{
		memoryEvent(t, orgID, agentID, sandboxID, "S1", "user.message.received", map[string]any{
			"source": "slack", "channel": "C123", "user_display_name": "Kim",
			"text": "Why did the database admin leave the party? Too many relationships.",
		}),
		memoryEvent(t, orgID, agentID, sandboxID, "S1", "agent.message.sent", map[string]any{
			"source": "slack", "text": "Painfully relational.",
		}),
	}
	if _, ok := buildEmployeeRetainItem(agent, EmployeeMemoryRetainPayload{AgentID: agentID, SandboxID: sandboxID, SessionID: "S1"}, events); ok {
		t.Fatal("pure banter without a work/tool signal should not retain")
	}
}

func TestBuildEmployeeRetainItem_PreservesExplicitRememberFactsWithoutTools(t *testing.T) {
	orgID := uuid.New()
	agentID := uuid.New()
	sandboxID := uuid.New()
	agent := &model.Agent{ID: agentID, OrgID: &orgID, Name: "Aria"}
	events := []model.EmployeeMemoryEvent{
		memoryEvent(t, orgID, agentID, sandboxID, "S1", "user.message.received", map[string]any{
			"source": "slack", "channel": "C123", "user_display_name": "Nora",
			"text": "Please remember this: Nora owns invoice-failure alerts, and billing answers must use live data when possible.",
		}),
		memoryEvent(t, orgID, agentID, sandboxID, "S1", "tool.invoked", map[string]any{
			"source": "slack", "tool": "bash", "result_summary": "Queried invoices table and found alert owner metadata.",
		}),
		memoryEvent(t, orgID, agentID, sandboxID, "S1", "agent.message.sent", map[string]any{
			"source": "slack", "text": "Remembered. Nora owns invoice-failure alerts, and billing answers should use live data when possible.",
		}),
	}

	item, ok := buildEmployeeRetainItem(agent, EmployeeMemoryRetainPayload{
		AgentID: agentID, SandboxID: sandboxID, SessionID: "S1", SourceEvent: "agent.message.sent",
	}, events)
	if !ok {
		t.Fatal("expected retain item")
	}
	for _, want := range []string{"Nora owns invoice-failure alerts", "billing answers must use live data", "Employee Aria"} {
		if !strings.Contains(item.Content, want) {
			t.Fatalf("retain content missing %q: %s", want, item.Content)
		}
	}
	if strings.Contains(item.Content, "Queried invoices") || strings.Contains(item.Content, "Tool ") || strings.Contains(item.Content, "bash") {
		t.Fatalf("retain content leaked tool execution trace: %s", item.Content)
	}
}
