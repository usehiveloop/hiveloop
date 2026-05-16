package handler

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
)

func loadEmployeeSubagents(db *gorm.DB, agentIDs []uuid.UUID) map[uuid.UUID][]employeeSubagentSummary {
	if len(agentIDs) == 0 {
		return nil
	}
	var links []model.AgentSubagent
	if err := db.Where("agent_id IN ?", agentIDs).Find(&links).Error; err != nil {
		return nil
	}
	if len(links) == 0 {
		return nil
	}
	return summarizeEmployeeSubagents(db, agentIDs, links)
}

func summarizeEmployeeSubagents(db *gorm.DB, agentIDs []uuid.UUID, links []model.AgentSubagent) map[uuid.UUID][]employeeSubagentSummary {
	subIDs := make([]uuid.UUID, 0, len(links))
	for _, link := range links {
		subIDs = append(subIDs, link.SubagentID)
	}
	var subs []model.Agent
	if err := db.Select("id, name, avatar_url, description, status, agent_config, system_prompt, identity_prompt").
		Where("id IN ?", subIDs).
		Find(&subs).Error; err != nil {
		return nil
	}
	byID := make(map[uuid.UUID]model.Agent, len(subs))
	for _, sub := range subs {
		byID[sub.ID] = sub
	}
	out := make(map[uuid.UUID][]employeeSubagentSummary, len(agentIDs))
	for _, link := range links {
		sub, ok := byID[link.SubagentID]
		if ok {
			out[link.AgentID] = append(out[link.AgentID], employeeSubagentSummaryFromAgent(sub))
		}
	}
	return out
}

func loadLatestSandboxPerAgent(db *gorm.DB, orgID uuid.UUID, agentIDs []uuid.UUID) map[uuid.UUID]*employeeSandboxSummary {
	if len(agentIDs) == 0 {
		return nil
	}
	var rows []model.Sandbox
	if err := db.
		Where("org_id = ? AND agent_id IN ?", orgID, agentIDs).
		Order("created_at DESC").
		Find(&rows).Error; err != nil {
		return nil
	}
	out := make(map[uuid.UUID]*employeeSandboxSummary, len(agentIDs))
	for _, sb := range rows {
		if sb.AgentID == nil {
			continue
		}
		if _, seen := out[*sb.AgentID]; seen {
			continue
		}
		out[*sb.AgentID] = employeeSandboxSummaryFromSandbox(sb)
	}
	return out
}

func employeeSandboxSummaryFromSandbox(sb model.Sandbox) *employeeSandboxSummary {
	summary := &employeeSandboxSummary{
		ID:           sb.ID.String(),
		Status:       sb.Status,
		ExternalID:   sb.ExternalID,
		ErrorMessage: sb.ErrorMessage,
		CreatedAt:    sb.CreatedAt.Format(time.RFC3339),
		snapshotID:   sb.SnapshotID,
	}
	if sb.LastActiveAt != nil {
		t := sb.LastActiveAt.Format(time.RFC3339)
		summary.LastActiveAt = &t
	}
	return summary
}
