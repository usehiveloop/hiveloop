package handler

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/model"
)

func TestBuildEmployeeMemoryItem_TagsOrgTeamSlackSource(t *testing.T) {
	orgID := uuid.New()
	teamID := uuid.New()
	agentID := uuid.New()
	sb := &model.Sandbox{ID: uuid.New(), OrgID: &orgID, AgentID: &agentID}
	agent := &model.Agent{ID: agentID, OrgID: &orgID, TeamID: &teamID, Name: "Aria"}
	payload, _ := json.Marshal(map[string]any{
		"session_id":        "C123-456.789",
		"source":            "slack",
		"channel":           "C123",
		"thread_ts":         "456.789",
		"user_display_name": "Kim",
		"text":              "The Platform team now requires integration tests before releases.",
	})

	item, ok := buildEmployeeMemoryItem(sb, agent, &employeeOutboundEvent{
		EventType: "user.message.received",
		Payload:   payload,
		At:        time.Date(2026, 5, 12, 10, 0, 0, 0, time.UTC),
	})
	if !ok {
		t.Fatal("expected memory item")
	}
	for _, want := range []string{
		"company:" + orgID.String(),
		"team:" + teamID.String(),
		"source:slack",
		"visibility:team",
		"memory_type:team_context",
		"channel:C123",
	} {
		if !hasString(item.Tags, want) {
			t.Fatalf("missing tag %q in %#v", want, item.Tags)
		}
	}
	if item.Metadata["session_id"] != "C123-456.789" {
		t.Fatalf("session metadata missing: %#v", item.Metadata)
	}
	if len(item.ObservationScopes) != 2 || !hasString(item.ObservationScopes[1], "team:"+teamID.String()) {
		t.Fatalf("team observation scope missing: %#v", item.ObservationScopes)
	}
	if !strings.Contains(item.Content, "integration tests") {
		t.Fatalf("unexpected content: %q", item.Content)
	}
}

func TestBuildEmployeeMemoryItem_UsesExplicitSource(t *testing.T) {
	orgID := uuid.New()
	agentID := uuid.New()
	sb := &model.Sandbox{ID: uuid.New(), OrgID: &orgID, AgentID: &agentID}
	agent := &model.Agent{ID: agentID, OrgID: &orgID, Name: "Aria"}
	payload, _ := json.Marshal(map[string]any{
		"source": "github",
		"text":   "Repository deploys must include rollback notes.",
	})

	item, ok := buildEmployeeMemoryItem(sb, agent, &employeeOutboundEvent{
		EventType: "user.message.received",
		Payload:   payload,
		At:        time.Now(),
	})
	if !ok {
		t.Fatal("expected memory item")
	}
	if !hasString(item.Tags, "source:github") {
		t.Fatalf("expected explicit source tag, got %#v", item.Tags)
	}
}

func TestBuildEmployeeMemoryItem_AcceptsFutureGatewaySource(t *testing.T) {
	orgID := uuid.New()
	agentID := uuid.New()
	sb := &model.Sandbox{ID: uuid.New(), OrgID: &orgID, AgentID: &agentID}
	agent := &model.Agent{ID: agentID, OrgID: &orgID, Name: "Aria"}
	payload, _ := json.Marshal(map[string]any{
		"source": "WhatsApp Business",
		"text":   "Customer escalations should be summarized before handoff.",
	})

	item, ok := buildEmployeeMemoryItem(sb, agent, &employeeOutboundEvent{
		EventType: "user.message.received",
		Payload:   payload,
		At:        time.Now(),
	})
	if !ok {
		t.Fatal("expected memory item")
	}
	if !hasString(item.Tags, "source:whatsapp-business") {
		t.Fatalf("expected sanitized future source tag, got %#v", item.Tags)
	}
}

func TestBuildEmployeeMemoryItem_SkipsErrorsAndSecrets(t *testing.T) {
	orgID := uuid.New()
	agentID := uuid.New()
	sb := &model.Sandbox{ID: uuid.New(), OrgID: &orgID, AgentID: &agentID}
	agent := &model.Agent{ID: agentID, OrgID: &orgID, Name: "Aria"}
	secretPayload, _ := json.Marshal(map[string]any{
		"text": "api_key=sk-secret should not be retained",
	})
	if _, ok := buildEmployeeMemoryItem(sb, agent, &employeeOutboundEvent{EventType: "user.message.received", Payload: secretPayload, At: time.Now()}); ok {
		t.Fatal("secret-looking content should be skipped")
	}
	if _, ok := buildEmployeeMemoryItem(sb, agent, &employeeOutboundEvent{EventType: "error.model", At: time.Now()}); ok {
		t.Fatal("error-only events should be skipped")
	}
}

func hasString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
