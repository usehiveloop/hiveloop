package handler

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func loadLatestSandboxPerAgent(db *gorm.DB, orgID uuid.UUID, agentIDs []uuid.UUID) map[uuid.UUID]*employeeSandboxSummary {
	out := make(map[uuid.UUID]*employeeSandboxSummary, len(agentIDs))
	if len(agentIDs) == 0 {
		return out
	}
	var sandboxes []model.Sandbox
	if err := db.
		Where("org_id = ? AND agent_id IN ?", orgID, agentIDs).
		Order("agent_id ASC, created_at DESC").
		Find(&sandboxes).Error; err != nil {
		return out
	}
	for _, sandbox := range sandboxes {
		if sandbox.AgentID == nil {
			continue
		}
		if _, exists := out[*sandbox.AgentID]; exists {
			continue
		}
		out[*sandbox.AgentID] = sandboxSummary(sandbox)
	}
	return out
}

func sandboxSummary(sandbox model.Sandbox) *employeeSandboxSummary {
	createdAt := sandbox.CreatedAt.UTC().Format(time.RFC3339)
	summary := &employeeSandboxSummary{
		ID:           sandbox.ID.String(),
		Status:       sandbox.Status,
		ExternalID:   sandbox.ExternalID,
		ErrorMessage: sandbox.ErrorMessage,
		CreatedAt:    createdAt,
		snapshotID:   sandbox.SnapshotID,
	}
	if sandbox.LastActiveAt != nil {
		lastActiveAt := sandbox.LastActiveAt.UTC().Format(time.RFC3339)
		summary.LastActiveAt = &lastActiveAt
	}
	return summary
}
