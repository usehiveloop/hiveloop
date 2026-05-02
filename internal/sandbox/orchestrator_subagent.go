package sandbox

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
)

// EnsureSubagentSandbox finds-or-creates a dedicated sandbox for a subagent
// invocation. Subagents are independent Agent rows (AgentType="subagent"),
// so they get their own model.Sandbox keyed by AgentID — separate from the
// parent agent's sandbox.
//
// Behavior:
//  1. Look for an existing non-deleted Sandbox with agent_id = subagentID.
//  2. If found and not in a terminal error state, return it. Lifecycle
//     (waking from stopped, unarchiving, etc.) is handled by GetBridgeClient
//     callers via EnsureSandboxActive.
//  3. Otherwise call CreateDedicatedSandbox to provision a fresh one.
//
// orgID is taken from the parent agent's org and is used only for validation
// (the subagent must belong to the same org as the parent).
func (o *Orchestrator) EnsureSubagentSandbox(ctx context.Context, orgID, parentAgentID, subagentID uuid.UUID) (*model.Sandbox, error) {
	if subagentID == uuid.Nil {
		return nil, fmt.Errorf("subagentID is required")
	}

	var subagent model.Agent
	if err := o.db.WithContext(ctx).Where("id = ?", subagentID).First(&subagent).Error; err != nil {
		return nil, fmt.Errorf("loading subagent %s: %w", subagentID, err)
	}
	if subagent.AgentType != model.AgentTypeSubagent {
		return nil, fmt.Errorf("agent %s is not a subagent (type=%q)", subagentID, subagent.AgentType)
	}
	if subagent.OrgID == nil || *subagent.OrgID != orgID {
		return nil, fmt.Errorf("subagent %s does not belong to org %s", subagentID, orgID)
	}

	// Verify the parent-subagent attachment is real. This prevents a parent
	// from spawning arbitrary subagents that aren't on its allowlist.
	var link model.AgentSubagent
	err := o.db.WithContext(ctx).
		Where("agent_id = ? AND subagent_id = ?", parentAgentID, subagentID).
		First(&link).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("subagent %s is not attached to parent %s", subagentID, parentAgentID)
		}
		return nil, fmt.Errorf("checking subagent attachment: %w", err)
	}

	var existing model.Sandbox
	err = o.db.WithContext(ctx).
		Where("agent_id = ?", subagentID).
		Order("created_at DESC").
		First(&existing).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("looking up existing subagent sandbox: %w", err)
	}
	if err == nil && existing.Status != string(StatusError) {
		return &existing, nil
	}

	return o.CreateDedicatedSandbox(ctx, &subagent)
}
