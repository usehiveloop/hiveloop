package dispatch

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/mcp/catalog"
	"github.com/ziraloop/ziraloop/internal/model"
	"github.com/ziraloop/ziraloop/internal/trigger/zira"
)

// GormRouterTriggerStore is the production implementation backed by Postgres.
type GormRouterTriggerStore struct {
	db      *gorm.DB
	catalog *catalog.Catalog
}

// NewGormRouterTriggerStore returns a production RouterTriggerStore.
func NewGormRouterTriggerStore(db *gorm.DB, actionsCatalog *catalog.Catalog) *GormRouterTriggerStore {
	return &GormRouterTriggerStore{db: db, catalog: actionsCatalog}
}

func (store *GormRouterTriggerStore) FindMatchingTriggers(ctx context.Context, orgID, connectionID uuid.UUID, triggerKeys []string) ([]RouterTriggerWithRouter, error) {
	keys := pq.StringArray(triggerKeys)
	var triggers []model.RouterTrigger
	if err := store.db.WithContext(ctx).
		Where("org_id = ? AND connection_id = ? AND enabled = TRUE AND trigger_keys && ?",
			orgID, connectionID, keys).
		Order("id ASC").
		Find(&triggers).Error; err != nil {
		return nil, fmt.Errorf("finding matching router triggers: %w", err)
	}
	if len(triggers) == 0 {
		return nil, nil
	}

	// Load routers for matched triggers.
	routerIDs := make([]uuid.UUID, 0, len(triggers))
	for _, trigger := range triggers {
		routerIDs = append(routerIDs, trigger.RouterID)
	}
	var routers []model.Router
	if err := store.db.WithContext(ctx).Where("id IN ?", routerIDs).Find(&routers).Error; err != nil {
		return nil, fmt.Errorf("loading routers: %w", err)
	}
	routerByID := make(map[uuid.UUID]model.Router, len(routers))
	for _, router := range routers {
		routerByID[router.ID] = router
	}

	results := make([]RouterTriggerWithRouter, 0, len(triggers))
	for _, trigger := range triggers {
		router, ok := routerByID[trigger.RouterID]
		if !ok {
			continue
		}
		results = append(results, RouterTriggerWithRouter{Trigger: trigger, Router: router})
	}
	return results, nil
}

func (store *GormRouterTriggerStore) FindExistingConversation(ctx context.Context, orgID, connectionID uuid.UUID, resourceKey string) (*model.RouterConversation, error) {
	if resourceKey == "" {
		return nil, nil
	}
	var conv model.RouterConversation
	err := store.db.WithContext(ctx).
		Where("org_id = ? AND connection_id = ? AND resource_key = ? AND status = ?",
			orgID, connectionID, resourceKey, "active").
		Order("created_at DESC").
		First(&conv).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("finding existing conversation: %w", err)
	}
	return &conv, nil
}

func (store *GormRouterTriggerStore) LoadRulesForTrigger(ctx context.Context, triggerID uuid.UUID) ([]model.RoutingRule, error) {
	var rules []model.RoutingRule
	if err := store.db.WithContext(ctx).
		Where("router_trigger_id = ?", triggerID).
		Order("priority ASC, id ASC").
		Find(&rules).Error; err != nil {
		return nil, fmt.Errorf("loading routing rules: %w", err)
	}
	return rules, nil
}

func (store *GormRouterTriggerStore) LoadOrgAgents(ctx context.Context, orgID uuid.UUID) ([]model.Agent, error) {
	var agents []model.Agent
	if err := store.db.WithContext(ctx).
		Where("org_id = ? AND is_system = FALSE AND status = ? AND deleted_at IS NULL", orgID, "active").
		Find(&agents).Error; err != nil {
		return nil, fmt.Errorf("loading org agents: %w", err)
	}
	return agents, nil
}

func (store *GormRouterTriggerStore) LoadOrgConnections(ctx context.Context, orgID uuid.UUID, excludeConnID uuid.UUID) ([]zira.ConnectionWithActions, error) {
	var connections []model.Connection
	query := store.db.WithContext(ctx).Preload("Integration").
		Where("org_id = ? AND revoked_at IS NULL", orgID)
	if excludeConnID != uuid.Nil {
		query = query.Where("id != ?", excludeConnID)
	}
	if err := query.Find(&connections).Error; err != nil {
		return nil, fmt.Errorf("loading org connections: %w", err)
	}

	results := make([]zira.ConnectionWithActions, 0, len(connections))
	for _, conn := range connections {
		provider := conn.Integration.Provider

		// Build read-actions map from catalog.
		providerDef, ok := store.catalog.GetProvider(provider)
		if !ok {
			continue
		}
		readActions := make(map[string]catalog.ActionDef)
		for actionKey, actionDef := range providerDef.Actions {
			if actionDef.Access == "read" {
				readActions[actionKey] = actionDef
			}
		}

		results = append(results, zira.ConnectionWithActions{
			Connection:  conn,
			Provider:    provider,
			ReadActions: readActions,
		})
	}
	return results, nil
}

func (store *GormRouterTriggerStore) LoadRecentDecisions(ctx context.Context, orgID uuid.UUID, eventType string, limit int) ([]model.RoutingDecision, error) {
	var decisions []model.RoutingDecision
	if err := store.db.WithContext(ctx).
		Where("org_id = ? AND event_type = ?", orgID, eventType).
		Order("created_at DESC").
		Limit(limit).
		Find(&decisions).Error; err != nil {
		return nil, fmt.Errorf("loading recent decisions: %w", err)
	}
	return decisions, nil
}

func (store *GormRouterTriggerStore) StoreDecision(ctx context.Context, decision *model.RoutingDecision) error {
	return store.db.WithContext(ctx).Create(decision).Error
}

func (store *GormRouterTriggerStore) StoreConversation(ctx context.Context, conv *model.RouterConversation) error {
	return store.db.WithContext(ctx).Create(conv).Error
}
