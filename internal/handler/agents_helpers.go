package handler

import (
	"encoding/json"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/usehiveloop/hiveloop/internal/model"
)

// loadAgentTriggers loads the routing triggers configured for one or more agents.
// Returns a map from agent ID to trigger responses. Uses a single query with
// joins to avoid N+1.
func (h *AgentHandler) loadAgentTriggers(agentIDs ...uuid.UUID) map[uuid.UUID][]agentTriggerResponse {
	if len(agentIDs) == 0 {
		return nil
	}

	type triggerRow struct {
		RuleID       uuid.UUID      `gorm:"column:rule_id"`
		AgentID      uuid.UUID      `gorm:"column:agent_id"`
		TriggerID    uuid.UUID      `gorm:"column:trigger_id"`
		TriggerType  string         `gorm:"column:trigger_type"`
		ConnID       *uuid.UUID     `gorm:"column:conn_id"`
		Provider     *string        `gorm:"column:provider"`
		TriggerKeys  pq.StringArray `gorm:"column:trigger_keys"`
		Enabled      bool           `gorm:"column:enabled"`
		Conditions   model.RawJSON  `gorm:"column:conditions"`
		CronSchedule string         `gorm:"column:cron_schedule"`
		Instructions string         `gorm:"column:instructions"`
	}

	var rows []triggerRow
	h.db.Raw(`
		SELECT
			rr.id AS rule_id,
			rr.agent_id,
			rt.id AS trigger_id,
			rt.trigger_type,
			rt.connection_id AS conn_id,
			ii.provider,
			rt.trigger_keys,
			rt.enabled,
			rr.conditions,
			rt.cron_schedule,
			rt.instructions
		FROM routing_rules rr
		JOIN router_triggers rt ON rt.id = rr.router_trigger_id
		LEFT JOIN in_connections ic ON ic.id = rt.connection_id
		LEFT JOIN in_integrations ii ON ii.id = ic.in_integration_id
		WHERE rr.agent_id IN ?
		ORDER BY rt.id ASC
	`, agentIDs).Scan(&rows)

	result := make(map[uuid.UUID][]agentTriggerResponse, len(agentIDs))
	for _, row := range rows {
		var conditions any
		if len(row.Conditions) > 0 {
			var parsed model.TriggerMatch
			if err := json.Unmarshal(row.Conditions, &parsed); err == nil && len(parsed.Conditions) > 0 {
				conditions = parsed
			}
		}

		response := agentTriggerResponse{
			ID:           row.TriggerID.String(),
			TriggerType:  row.TriggerType,
			TriggerKeys:  []string(row.TriggerKeys),
			Enabled:      row.Enabled,
			Conditions:   conditions,
			CronSchedule: row.CronSchedule,
			Instructions: row.Instructions,
		}
		if row.ConnID != nil {
			response.ConnectionID = row.ConnID.String()
		}
		if row.Provider != nil {
			response.Provider = *row.Provider
		}

		result[row.AgentID] = append(result[row.AgentID], response)
	}
	return result
}

// loadAgentSubagents batch-loads attached subagent summaries for one or more agents.
func (h *AgentHandler) loadAgentSubagents(agentIDs ...uuid.UUID) map[uuid.UUID][]agentSubagentSummary {
	if len(agentIDs) == 0 {
		return nil
	}
	var links []model.AgentSubagent
	if err := h.db.Where("agent_id IN ?", agentIDs).Find(&links).Error; err != nil {
		return nil
	}
	if len(links) == 0 {
		return nil
	}
	subIDs := make([]uuid.UUID, len(links))
	for index, link := range links {
		subIDs[index] = link.SubagentID
	}
	var subs []model.Agent
	if err := h.db.Select("id, name, description, model").Where("id IN ?", subIDs).Find(&subs).Error; err != nil {
		return nil
	}
	subByID := make(map[uuid.UUID]model.Agent, len(subs))
	for _, sub := range subs {
		subByID[sub.ID] = sub
	}
	result := make(map[uuid.UUID][]agentSubagentSummary, len(agentIDs))
	for _, link := range links {
		sub, ok := subByID[link.SubagentID]
		if !ok {
			continue
		}
		result[link.AgentID] = append(result[link.AgentID], agentSubagentSummary{
			ID:          sub.ID.String(),
			Name:        sub.Name,
			Description: sub.Description,
			Model:       sub.Model,
		})
	}
	return result
}

// loadAgentSkills batch-loads attached skill summaries for one or more agents.
func (h *AgentHandler) loadAgentSkills(agentIDs ...uuid.UUID) map[uuid.UUID][]agentSkillSummary {
	if len(agentIDs) == 0 {
		return nil
	}
	var links []model.AgentSkill
	if err := h.db.Where("agent_id IN ?", agentIDs).Find(&links).Error; err != nil {
		return nil
	}
	if len(links) == 0 {
		return nil
	}
	skillIDs := make([]uuid.UUID, len(links))
	for index, link := range links {
		skillIDs[index] = link.SkillID
	}
	var skills []model.Skill
	if err := h.db.Select("id, name, description, source_type").Where("id IN ?", skillIDs).Find(&skills).Error; err != nil {
		return nil
	}
	skillByID := make(map[uuid.UUID]model.Skill, len(skills))
	for _, skill := range skills {
		skillByID[skill.ID] = skill
	}
	result := make(map[uuid.UUID][]agentSkillSummary, len(agentIDs))
	for _, link := range links {
		skill, ok := skillByID[link.SkillID]
		if !ok {
			continue
		}
		result[link.AgentID] = append(result[link.AgentID], agentSkillSummary{
			ID:          skill.ID.String(),
			Name:        skill.Name,
			Description: skill.Description,
			SourceType:  skill.SourceType,
		})
	}
	return result
}


