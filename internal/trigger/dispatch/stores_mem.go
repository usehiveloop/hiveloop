package dispatch

import (
	"context"
	"sort"

	"github.com/google/uuid"

	"github.com/ziraloop/ziraloop/internal/model"
)

// MemoryAgentTriggerStore is an in-memory AgentTriggerStore for tests. Match
// semantics mirror the production GORM query exactly:
//
//   - org_id and connection_id must equal the input
//   - Enabled must be true
//   - At least one of TriggerKeys OR TerminateEventKeys must equal one of the
//     input keys (array overlap on BOTH columns — the dispatcher then decides
//     per-trigger whether the event plays a normal or terminate role)
//   - Results sorted by trigger ID ascending (deterministic)
//   - Soft-deleted agents are filtered out (DeletedAt != nil)
//
// Tests build this with Add() before calling Dispatcher.Run().
type MemoryAgentTriggerStore struct {
	triggers []model.AgentTrigger
	agents   map[uuid.UUID]model.Agent
}

// NewMemoryAgentTriggerStore returns an empty in-memory store.
func NewMemoryAgentTriggerStore() *MemoryAgentTriggerStore {
	return &MemoryAgentTriggerStore{
		agents: make(map[uuid.UUID]model.Agent),
	}
}

// Add registers an agent and one trigger in a single call. The most common
// shape in tests is "one agent has one trigger".
func (s *MemoryAgentTriggerStore) Add(agent model.Agent, trigger model.AgentTrigger) {
	s.agents[agent.ID] = agent
	s.triggers = append(s.triggers, trigger)
}

// FindMatching mirrors gormAgentTriggerStore.FindMatching exactly so tests
// catch logic bugs that would also surface in production.
func (s *MemoryAgentTriggerStore) FindMatching(_ context.Context, orgID, connectionID uuid.UUID, triggerKeys []string) ([]TriggerWithAgent, error) {
	if len(triggerKeys) == 0 {
		return nil, nil
	}
	keyset := make(map[string]bool, len(triggerKeys))
	for _, key := range triggerKeys {
		keyset[key] = true
	}

	matched := make([]model.AgentTrigger, 0)
	for _, trigger := range s.triggers {
		if trigger.OrgID != orgID || trigger.ConnectionID != connectionID {
			continue
		}
		if !trigger.Enabled {
			continue
		}
		hit := false
		for _, key := range trigger.TriggerKeys {
			if keyset[key] {
				hit = true
				break
			}
		}
		if !hit {
			for _, key := range trigger.TerminateEventKeys {
				if keyset[key] {
					hit = true
					break
				}
			}
		}
		if !hit {
			continue
		}
		matched = append(matched, trigger)
	}

	sort.Slice(matched, func(i, j int) bool {
		return matched[i].ID.String() < matched[j].ID.String()
	})

	out := make([]TriggerWithAgent, 0, len(matched))
	for _, trigger := range matched {
		agent, ok := s.agents[trigger.AgentID]
		if !ok {
			continue
		}
		if agent.DeletedAt != nil {
			continue
		}
		out = append(out, TriggerWithAgent{Trigger: trigger, Agent: agent})
	}
	return out, nil
}
