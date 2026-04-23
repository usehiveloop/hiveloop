package handler

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
)

func (h *AgentHandler) loadAgentTriggers(agentIDs ...uuid.UUID) map[uuid.UUID][]agentTriggerResponse {
	if len(agentIDs) == 0 {
		return nil
	}

	type triggerRow struct {
		RuleID      uuid.UUID      `gorm:"column:rule_id"`
		AgentID     uuid.UUID      `gorm:"column:agent_id"`
		TriggerID   uuid.UUID      `gorm:"column:trigger_id"`
		ConnID      uuid.UUID      `gorm:"column:conn_id"`
		Provider    string         `gorm:"column:provider"`
		TriggerKeys pq.StringArray `gorm:"column:trigger_keys"`
		Enabled     bool           `gorm:"column:enabled"`
		Conditions  model.RawJSON  `gorm:"column:conditions"`
	}

	var rows []triggerRow
	h.db.Raw(`
		SELECT
			rr.id AS rule_id,
			rr.agent_id,
			rt.id AS trigger_id,
			rt.connection_id AS conn_id,
			ii.provider,
			rt.trigger_keys,
			rt.enabled,
			rr.conditions
		FROM routing_rules rr
		JOIN router_triggers rt ON rt.id = rr.router_trigger_id
		JOIN in_connections ic ON ic.id = rt.connection_id
		JOIN in_integrations ii ON ii.id = ic.in_integration_id
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

		result[row.AgentID] = append(result[row.AgentID], agentTriggerResponse{
			ID:           row.TriggerID.String(),
			ConnectionID: row.ConnID.String(),
			Provider:     row.Provider,
			TriggerKeys:  []string(row.TriggerKeys),
			Enabled:      row.Enabled,
			Conditions:   conditions,
		})
	}
	return result
}

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

func createAgentTriggers(tx *gorm.DB, orgID, agentID uuid.UUID, triggers []agentTriggerInput) error {
	if len(triggers) == 0 {
		return nil
	}

	var router model.Router
	if err := tx.Where("org_id = ?", orgID).FirstOrCreate(&router, model.Router{
		OrgID: orgID,
		Name:  "Zira",
	}).Error; err != nil {
		return fmt.Errorf("find or create router: %w", err)
	}

	for _, input := range triggers {
		connectionID, err := uuid.Parse(input.ConnectionID)
		if err != nil {
			return fmt.Errorf("invalid connection_id %q: %w", input.ConnectionID, err)
		}

		trigger := model.RouterTrigger{
			OrgID:        orgID,
			RouterID:     router.ID,
			ConnectionID: connectionID,
			TriggerKeys:  pq.StringArray(input.TriggerKeys),
			Enabled:      true,
			RoutingMode:  "rule",
		}
		if err := tx.Create(&trigger).Error; err != nil {
			return fmt.Errorf("create router trigger: %w", err)
		}

		var conditionsJSON model.RawJSON
		if input.Conditions != nil && len(input.Conditions.Conditions) > 0 {
			conditionsJSON, _ = json.Marshal(input.Conditions)
		}

		rule := model.RoutingRule{
			RouterTriggerID: trigger.ID,
			AgentID:         agentID,
			Priority:        1,
			Conditions:      conditionsJSON,
		}
		if err := tx.Create(&rule).Error; err != nil {
			return fmt.Errorf("create routing rule: %w", err)
		}
	}
	return nil
}

func deleteAgentTriggers(db *gorm.DB, agentID uuid.UUID) error {
	var triggerIDs []uuid.UUID
	if err := db.Model(&model.RoutingRule{}).
		Where("agent_id = ?", agentID).
		Pluck("router_trigger_id", &triggerIDs).Error; err != nil {
		return fmt.Errorf("find agent triggers: %w", err)
	}
	if len(triggerIDs) == 0 {
		return nil
	}
	if err := db.Where("id IN ?", triggerIDs).Delete(&model.RouterTrigger{}).Error; err != nil {
		return fmt.Errorf("delete agent triggers: %w", err)
	}
	return nil
}
