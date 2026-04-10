package dispatch

import (
	"context"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/model"
)

// AgentTriggerStore loads enabled agent triggers for a connection that match
// at least one of the given trigger keys. Implementations must return results
// in deterministic order (sort by ID) so dispatcher tests are stable.
type AgentTriggerStore interface {
	FindMatching(ctx context.Context, orgID, connectionID uuid.UUID, triggerKeys []string) ([]TriggerWithAgent, error)
}

// TriggerWithAgent bundles an AgentTrigger with the small slice of Agent fields
// the dispatcher needs (sandbox decision, integrations preview). Loading the
// full Agent eagerly avoids a second query in the executor for the common case.
type TriggerWithAgent struct {
	Trigger model.AgentTrigger
	Agent   model.Agent
}

// gormAgentTriggerStore is the production implementation backed by Postgres.
// The query joins agent_triggers → agents and filters with PostgreSQL's
// array-overlap operator (`&&`) on two columns: trigger_keys (normal events
// that start or continue a run) and terminate_event_keys (events that close
// the conversation). An AgentTrigger matches if the incoming event key is in
// either list — the dispatcher decides per-trigger which role the event plays.
type gormAgentTriggerStore struct {
	db *gorm.DB
}

// NewGormAgentTriggerStore returns a production AgentTriggerStore.
func NewGormAgentTriggerStore(db *gorm.DB) AgentTriggerStore {
	return &gormAgentTriggerStore{db: db}
}

func (s *gormAgentTriggerStore) FindMatching(ctx context.Context, orgID, connectionID uuid.UUID, triggerKeys []string) ([]TriggerWithAgent, error) {
	if len(triggerKeys) == 0 {
		return nil, nil
	}
	keys := pq.StringArray(triggerKeys)
	var triggers []model.AgentTrigger
	err := s.db.WithContext(ctx).
		Where("org_id = ? AND connection_id = ? AND enabled = TRUE AND (trigger_keys && ? OR terminate_event_keys && ?)",
			orgID, connectionID, keys, keys).
		Order("id ASC").
		Find(&triggers).Error
	if err != nil {
		return nil, err
	}
	if len(triggers) == 0 {
		return nil, nil
	}

	agentIDs := make([]uuid.UUID, 0, len(triggers))
	for _, trigger := range triggers {
		agentIDs = append(agentIDs, trigger.AgentID)
	}
	var agents []model.Agent
	if err := s.db.WithContext(ctx).
		Where("id IN ? AND deleted_at IS NULL", agentIDs).
		Find(&agents).Error; err != nil {
		return nil, err
	}
	agentByID := make(map[uuid.UUID]model.Agent, len(agents))
	for _, agent := range agents {
		agentByID[agent.ID] = agent
	}

	out := make([]TriggerWithAgent, 0, len(triggers))
	for _, trigger := range triggers {
		agent, ok := agentByID[trigger.AgentID]
		if !ok {
			// Agent was soft-deleted between trigger creation and now. Skip.
			continue
		}
		out = append(out, TriggerWithAgent{Trigger: trigger, Agent: agent})
	}
	return out, nil
}
