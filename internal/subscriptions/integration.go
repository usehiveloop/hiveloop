package subscriptions

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/model"
)

// AgentProviderResolver returns the set of integration providers an agent has
// access to. It resolves agent.Integrations (keyed by connection_id) through
// in_connections → in_integrations to extract each connection's provider name.
//
// The result is a stable sorted slice (alphabetical) so callers can use it
// for deterministic error-message rendering.
type AgentProviderResolver struct {
	db *gorm.DB
}

// NewAgentProviderResolver constructs a resolver bound to the given DB.
func NewAgentProviderResolver(db *gorm.DB) *AgentProviderResolver {
	return &AgentProviderResolver{db: db}
}

// Providers returns the set of providers the agent has connections to.
// Empty slice when the agent has no integrations enabled.
func (resolver *AgentProviderResolver) Providers(ctx context.Context, agent *model.Agent) ([]string, error) {
	if agent == nil {
		return nil, errors.New("agent is required")
	}

	connectionIDs := extractConnectionIDs(agent.Integrations)
	if len(connectionIDs) == 0 {
		return nil, nil
	}

	type row struct {
		Provider string
	}
	var rows []row
	err := resolver.db.WithContext(ctx).
		Table("in_connections").
		Select("DISTINCT in_integrations.provider AS provider").
		Joins("JOIN in_integrations ON in_integrations.id = in_connections.in_integration_id").
		Where("in_connections.id IN ? AND in_connections.revoked_at IS NULL", connectionIDs).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("resolving agent providers: %w", err)
	}

	providers := make([]string, 0, len(rows))
	for _, r := range rows {
		if r.Provider != "" {
			providers = append(providers, r.Provider)
		}
	}
	sort.Strings(providers)
	return providers, nil
}

// HasProvider reports whether the agent has at least one active connection
// for the given provider. Convenience wrapper used by the subscribe service
// before permitting a resource subscription.
func (resolver *AgentProviderResolver) HasProvider(ctx context.Context, agent *model.Agent, provider string) (bool, error) {
	providers, err := resolver.Providers(ctx, agent)
	if err != nil {
		return false, err
	}
	return slices.Contains(providers, provider), nil
}

// extractConnectionIDs parses Agent.Integrations (JSONB keyed by connection UUID)
// and returns the set of valid connection IDs. Non-UUID keys are silently
// skipped — older rows may have stale or malformed keys we don't want to
// fail hard on.
func extractConnectionIDs(integrations model.JSON) []uuid.UUID {
	if len(integrations) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, 0, len(integrations))
	for key := range integrations {
		id, err := uuid.Parse(key)
		if err != nil {
			continue
		}
		ids = append(ids, id)
	}
	return ids
}
