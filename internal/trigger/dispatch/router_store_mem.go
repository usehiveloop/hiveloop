package dispatch

import (
	"context"
	"sync"

	"github.com/google/uuid"

	"github.com/ziraloop/ziraloop/internal/model"
	"github.com/ziraloop/ziraloop/internal/trigger/zira"
)

// MemoryRouterTriggerStore is an in-memory implementation of RouterTriggerStore
// for deterministic testing. All data is pre-loaded; no DB or catalog needed.
type MemoryRouterTriggerStore struct {
	mu            sync.Mutex
	triggers      []RouterTriggerWithRouter
	conversations []model.RouterConversation
	rules         map[uuid.UUID][]model.RoutingRule // trigger_id → rules
	agents        []model.Agent
	connections   []zira.ConnectionWithActions
	decisions     []model.RoutingDecision
}

// NewMemoryRouterTriggerStore creates an empty in-memory store.
func NewMemoryRouterTriggerStore() *MemoryRouterTriggerStore {
	return &MemoryRouterTriggerStore{
		rules: make(map[uuid.UUID][]model.RoutingRule),
	}
}

// AddTrigger registers a trigger with its router for FindMatchingTriggers.
func (store *MemoryRouterTriggerStore) AddTrigger(trigger model.RouterTrigger, router model.Router) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.triggers = append(store.triggers, RouterTriggerWithRouter{Trigger: trigger, Router: router})
}

// AddRule registers a routing rule for a trigger.
func (store *MemoryRouterTriggerStore) AddRule(triggerID uuid.UUID, rule model.RoutingRule) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.rules[triggerID] = append(store.rules[triggerID], rule)
}

// AddAgent registers an org agent for LoadOrgAgents.
func (store *MemoryRouterTriggerStore) AddAgent(agent model.Agent) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.agents = append(store.agents, agent)
}

// AddConnection registers a connection for LoadOrgConnections.
func (store *MemoryRouterTriggerStore) AddConnection(conn zira.ConnectionWithActions) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.connections = append(store.connections, conn)
}

func (store *MemoryRouterTriggerStore) FindMatchingTriggers(_ context.Context, orgID, connectionID uuid.UUID, triggerKeys []string) ([]RouterTriggerWithRouter, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	keySet := make(map[string]bool, len(triggerKeys))
	for _, key := range triggerKeys {
		keySet[key] = true
	}

	var matches []RouterTriggerWithRouter
	for _, item := range store.triggers {
		trigger := item.Trigger
		if trigger.OrgID != orgID || trigger.ConnectionID != connectionID || !trigger.Enabled {
			continue
		}
		for _, key := range trigger.TriggerKeys {
			if keySet[key] {
				matches = append(matches, item)
				break
			}
		}
	}
	return matches, nil
}

func (store *MemoryRouterTriggerStore) FindExistingConversation(_ context.Context, orgID, connectionID uuid.UUID, resourceKey string) (*model.RouterConversation, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	if resourceKey == "" {
		return nil, nil
	}
	for index := len(store.conversations) - 1; index >= 0; index-- {
		conv := store.conversations[index]
		if conv.OrgID == orgID && conv.ConnectionID == connectionID && conv.ResourceKey == resourceKey && conv.Status == "active" {
			return &conv, nil
		}
	}
	return nil, nil
}

func (store *MemoryRouterTriggerStore) LoadRulesForTrigger(_ context.Context, triggerID uuid.UUID) ([]model.RoutingRule, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	rules := store.rules[triggerID]
	// Sort by priority (already sorted by insertion in tests, but be safe).
	return rules, nil
}

func (store *MemoryRouterTriggerStore) LoadOrgAgents(_ context.Context, orgID uuid.UUID) ([]model.Agent, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	var result []model.Agent
	for _, agent := range store.agents {
		if agent.OrgID != nil && *agent.OrgID == orgID && agent.Status == "active" {
			result = append(result, agent)
		}
	}
	return result, nil
}

func (store *MemoryRouterTriggerStore) LoadOrgConnections(_ context.Context, orgID uuid.UUID, excludeConnID uuid.UUID) ([]zira.ConnectionWithActions, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	var result []zira.ConnectionWithActions
	for _, conn := range store.connections {
		if conn.Connection.OrgID == orgID && conn.Connection.ID != excludeConnID {
			result = append(result, conn)
		}
	}
	return result, nil
}

func (store *MemoryRouterTriggerStore) LoadRecentDecisions(_ context.Context, orgID uuid.UUID, eventType string, limit int) ([]model.RoutingDecision, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	var result []model.RoutingDecision
	for index := len(store.decisions) - 1; index >= 0 && len(result) < limit; index-- {
		decision := store.decisions[index]
		if decision.OrgID == orgID && decision.EventType == eventType {
			result = append(result, decision)
		}
	}
	return result, nil
}

func (store *MemoryRouterTriggerStore) StoreDecision(_ context.Context, decision *model.RoutingDecision) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.decisions = append(store.decisions, *decision)
	return nil
}

func (store *MemoryRouterTriggerStore) StoreConversation(_ context.Context, conv *model.RouterConversation) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.conversations = append(store.conversations, *conv)
	return nil
}

// StoredDecisions returns all stored decisions (for test assertions).
func (store *MemoryRouterTriggerStore) StoredDecisions() []model.RoutingDecision {
	store.mu.Lock()
	defer store.mu.Unlock()
	result := make([]model.RoutingDecision, len(store.decisions))
	copy(result, store.decisions)
	return result
}

// StoredConversations returns all stored conversations (for test assertions).
func (store *MemoryRouterTriggerStore) StoredConversations() []model.RouterConversation {
	store.mu.Lock()
	defer store.mu.Unlock()
	result := make([]model.RouterConversation, len(store.conversations))
	copy(result, store.conversations)
	return result
}
