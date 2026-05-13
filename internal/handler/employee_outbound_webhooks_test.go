package handler

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/model"
)

func TestEmployeeOutboundMemoryCheckpoints(t *testing.T) {
	if shouldTriggerEmployeeMemoryCheckpoint("user.message.received") {
		t.Fatal("user message alone should not trigger retain")
	}
	if !shouldTriggerEmployeeMemoryCheckpoint("agent.message.sent") {
		t.Fatal("agent message should trigger retain checkpoint")
	}
	if shouldTriggerEmployeeMemoryCheckpoint("agent.stream.token") {
		t.Fatal("stream tokens should be stored but should not trigger retain")
	}
	if shouldTriggerEmployeeMemoryCheckpoint("error.model") {
		t.Fatal("errors should be stored but should not trigger retain")
	}
}

func TestEmployeeMemoryEventFromOutbound_StoresAllEventTypes(t *testing.T) {
	orgID := uuid.New()
	agentID := uuid.New()
	sandboxID := uuid.New()
	eventAt := time.Date(2026, 5, 13, 9, 30, 0, 0, time.UTC)
	sb := &model.Sandbox{ID: sandboxID, OrgID: &orgID, AgentID: &agentID}
	for _, eventType := range []string{
		"user.message.received",
		"agent.stream.token",
		"agent.stream.thinking",
		"agent.tool.call",
		"agent.tool.result",
		"agent.final_message",
		"agent.run.turn_completed",
		"error.model",
	} {
		payload := map[string]any{
			"session_id": "slack-session-1",
			"source":     "slack",
			"text":       "api_key=sk-secret should still be persisted for session sync",
		}
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
		stored, ok := employeeMemoryEventFromOutbound(sb, &employeeOutboundEvent{
			EventType: eventType,
			Payload:   body,
			At:        eventAt,
		}, payload, "slack-session-1")
		if !ok {
			t.Fatalf("%s was not stored", eventType)
		}
		if stored.EventType != eventType || stored.SessionID != "slack-session-1" || stored.Source != "slack" {
			t.Fatalf("stored event mismatch: %#v", stored)
		}
	}
}

func TestEmployeeMemoryEventFromOutbound_StoresEventWithoutSessionID(t *testing.T) {
	orgID := uuid.New()
	agentID := uuid.New()
	sandboxID := uuid.New()
	sb := &model.Sandbox{ID: sandboxID, OrgID: &orgID, AgentID: &agentID}
	payload := map[string]any{"source": "system"}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	stored, ok := employeeMemoryEventFromOutbound(sb, &employeeOutboundEvent{
		EventType: "config.applied",
		Payload:   body,
		At:        time.Now().UTC(),
	}, payload, "")
	if !ok {
		t.Fatal("event without session id should still be stored")
	}
	if stored.SessionID != "" || stored.EventType != "config.applied" {
		t.Fatalf("stored event mismatch: %#v", stored)
	}
}

func TestEmployeeEventSource_SanitizesFutureGateways(t *testing.T) {
	source := employeeEventSource(map[string]any{"source": "WhatsApp Business"})
	if source != "whatsapp-business" {
		t.Fatalf("source = %q", source)
	}
	if employeeEventSource(map[string]any{}) != "manual" {
		t.Fatal("missing source should fall back to manual")
	}
}

func TestPayloadLooksSensitive(t *testing.T) {
	if !payloadLooksSensitive(map[string]any{"text": "api_key=sk-secret"}) {
		t.Fatal("expected secret-looking payload to be rejected")
	}
	if payloadLooksSensitive(map[string]any{"text": "The team requires rollback notes."}) {
		t.Fatal("ordinary payload should not be rejected")
	}
}
