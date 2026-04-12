package dispatch

import (
	"context"

	"github.com/google/uuid"

	"github.com/ziraloop/ziraloop/internal/model"
	"github.com/ziraloop/ziraloop/internal/trigger/zira"
)

// RouterTriggerWithRouter bundles a trigger with its parent router config.
type RouterTriggerWithRouter struct {
	Trigger model.RouterTrigger
	Router  model.Router
}

// RouterTriggerStore is the data access interface for the router dispatcher.
// Two implementations exist: gorm (production Postgres) and memory (tests).
type RouterTriggerStore interface {
	FindMatchingTriggers(ctx context.Context, orgID, connectionID uuid.UUID, triggerKeys []string) ([]RouterTriggerWithRouter, error)
	FindExistingConversation(ctx context.Context, orgID, connectionID uuid.UUID, resourceKey string) (*model.RouterConversation, error)
	LoadRulesForTrigger(ctx context.Context, triggerID uuid.UUID) ([]model.RoutingRule, error)
	LoadOrgAgents(ctx context.Context, orgID uuid.UUID) ([]model.Agent, error)
	LoadOrgConnections(ctx context.Context, orgID uuid.UUID, excludeConnID uuid.UUID) ([]zira.ConnectionWithActions, error)
	LoadRecentDecisions(ctx context.Context, orgID uuid.UUID, eventType string, limit int) ([]model.RoutingDecision, error)
	StoreDecision(ctx context.Context, decision *model.RoutingDecision) error
	StoreConversation(ctx context.Context, conv *model.RouterConversation) error
}
