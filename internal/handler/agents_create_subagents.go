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

// validateSubagents verifies that every id refers to an active, same-org agent
// that is not itself an employee. Returns the resolved agents on success. On
// validation failure (caller should respond 400) returns a non-empty message.
// On unexpected DB failure (caller should respond 500) returns a non-nil error.
func validateSubagents(db *gorm.DB, orgID uuid.UUID, ids []uuid.UUID) ([]model.Agent, string, error) {
	if len(ids) == 0 {
		return nil, "", nil
	}
	var visible []model.Agent
	if err := db.
		Select("id", "is_employee").
		Where("id IN ? AND org_id = ? AND status = ?", ids, orgID, "active").
		Find(&visible).Error; err != nil {
		return nil, "", fmt.Errorf("query subagents: %w", err)
	}
	if len(visible) != len(ids) {
		return nil, "one or more subagent_ids are not active agents in this workspace", nil
	}
	for _, sub := range visible {
		if sub.IsEmployee {
			return nil, fmt.Sprintf("subagent_id %s refers to an employee; employees cannot be subagents", sub.ID), nil
		}
	}
	return visible, "", nil
}

// attachSubagents inserts AgentSubagent join rows in the caller's transaction.
// Subagents must already have been validated via validateSubagents.
func attachSubagents(tx *gorm.DB, agentID uuid.UUID, subagents []model.Agent) error {
	if len(subagents) == 0 {
		return nil
	}
	links := make([]model.AgentSubagent, len(subagents))
	for i, sub := range subagents {
		links[i] = model.AgentSubagent{AgentID: agentID, SubagentID: sub.ID}
	}
	if err := tx.Create(&links).Error; err != nil {
		return fmt.Errorf("attach subagents: %w", err)
	}
	return nil
}
