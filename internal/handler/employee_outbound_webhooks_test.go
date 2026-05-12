package handler

import "testing"

func TestEmployeeOutboundMemoryCheckpoints(t *testing.T) {
	if !shouldStoreEmployeeMemoryEvent("user.message.received") {
		t.Fatal("user message should be stored")
	}
	if shouldTriggerEmployeeMemoryCheckpoint("user.message.received") {
		t.Fatal("user message alone should not trigger retain")
	}
	if !shouldStoreEmployeeMemoryEvent("agent.message.sent") {
		t.Fatal("agent message should be stored")
	}
	if !shouldTriggerEmployeeMemoryCheckpoint("agent.message.sent") {
		t.Fatal("agent message should trigger retain checkpoint")
	}
	if shouldStoreEmployeeMemoryEvent("error.model") {
		t.Fatal("errors should not be retained as memory events")
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
