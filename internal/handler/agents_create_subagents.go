package handler

import (
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
)

// parseSubagentIDs validates and de-duplicates raw subagent_id strings.
// Returns the parsed UUIDs and a non-empty error message on the first invalid id.
func parseSubagentIDs(raw []string) ([]uuid.UUID, string) {
	if len(raw) == 0 {
		return nil, ""
	}
	out := make([]uuid.UUID, 0, len(raw))
	seen := make(map[uuid.UUID]struct{}, len(raw))
	for _, s := range raw {
		parsed, err := uuid.Parse(s)
		if err != nil {
			return nil, fmt.Sprintf("invalid subagent_id %q", s)
		}
		if _, dup := seen[parsed]; dup {
			continue
		}
		seen[parsed] = struct{}{}
		out = append(out, parsed)
	}
	return out, ""
}

// attachSubagents validates and inserts AgentSubagent join rows in the caller's
// transaction. Subagents must be active, scoped to orgID, and not themselves
// employees (no nested employees).
func attachSubagents(tx *gorm.DB, orgID, agentID uuid.UUID, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	var visible []model.Agent
	if err := tx.
		Select("id", "is_employee").
		Where("id IN ? AND org_id = ? AND status = ?", ids, orgID, "active").
		Find(&visible).Error; err != nil {
		return fmt.Errorf("validate subagent_ids: %w", err)
	}
	if len(visible) != len(ids) {
		return fmt.Errorf("one or more subagent_ids are not active agents in this workspace")
	}
	for _, sub := range visible {
		if sub.IsEmployee {
			return fmt.Errorf("subagent_id %s refers to an employee; employees cannot be subagents", sub.ID)
		}
	}
	links := make([]model.AgentSubagent, len(visible))
	for i, sub := range visible {
		links[i] = model.AgentSubagent{AgentID: agentID, SubagentID: sub.ID}
	}
	if err := tx.Create(&links).Error; err != nil {
		return fmt.Errorf("attach subagents: %w", err)
	}
	return nil
}
