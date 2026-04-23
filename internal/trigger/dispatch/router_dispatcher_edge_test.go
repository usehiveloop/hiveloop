package dispatch

import (
	"context"
	"log/slog"
	"testing"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/mcp/catalog"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/trigger/hiveloop"
)

func TestDispatch_ThreadAffinity_ExistingConv(t *testing.T) {
	store := NewMemoryRouterTriggerStore()
	router := newTestRouter()
	triggerID := uuid.New()
	store.AddTrigger(newTestTrigger(triggerID, "rule", "app_mention"), router)

	existingConvID := "conv-existing-123"
	existingSandboxID := uuid.New()
	store.StoreConversation(context.Background(), &model.RouterConversation{
		OrgID:                testOrgID,
		AgentID:              testAgentA,
		ConnectionID:         testConnID,
		ResourceKey:          "slack:T123:C456:ts789",
		BridgeConversationID: existingConvID,
		SandboxID:            existingSandboxID,
		Status:               "active",
	})

	dispatcher := NewRouterDispatcher(store, catalog.Global(), nil, slog.Default())

	input := RouterDispatchInput{
		Provider: "slack", EventType: "app_mention",
		OrgID: testOrgID, ConnectionID: testConnID,
		Payload: map[string]any{},
	}
	dispatches, err := dispatcher.Run(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = dispatches
}

func TestDispatch_NoMatchingTrigger(t *testing.T) {
	store := NewMemoryRouterTriggerStore()
	dispatcher := NewRouterDispatcher(store, catalog.Global(), nil, slog.Default())

	dispatches, err := dispatcher.Run(context.Background(), baseInput())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dispatches) != 0 {
		t.Errorf("expected 0 dispatches for no matching triggers, got %d", len(dispatches))
	}
}

func TestDispatch_DisabledTrigger(t *testing.T) {
	store := NewMemoryRouterTriggerStore()
	trigger := newTestTrigger(uuid.New(), "rule", "pull_request.opened")
	trigger.Enabled = false
	store.AddTrigger(trigger, newTestRouter())

	dispatcher := NewRouterDispatcher(store, catalog.Global(), nil, slog.Default())
	dispatches, err := dispatcher.Run(context.Background(), baseInput())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dispatches) != 0 {
		t.Errorf("disabled trigger should be skipped, got %d dispatches", len(dispatches))
	}
}

func TestDispatch_StoresRoutingDecision(t *testing.T) {
	triggerID := uuid.New()
	store, dispatcher := setupRuleStore(triggerID, model.RoutingRule{AgentID: testAgentA, Priority: 1})

	_, err := dispatcher.Run(context.Background(), baseInput())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	decisions := store.StoredDecisions()
	if len(decisions) != 1 {
		t.Fatalf("expected 1 stored decision, got %d", len(decisions))
	}
	if decisions[0].RoutingMode != "rule" {
		t.Errorf("decision routing mode: got %q", decisions[0].RoutingMode)
	}
	if len(decisions[0].SelectedAgents) != 1 {
		t.Errorf("decision agents: got %d", len(decisions[0].SelectedAgents))
	}
}
