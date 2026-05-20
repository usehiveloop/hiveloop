package tasks

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func (h *EmployeeTriggerDispatchHandler) findOrCreateTriggerConversation(ctx context.Context, agent *model.Agent, sb *model.Sandbox, triggerID uuid.UUID, resourceKey, conversationID string) (*model.AgentConversation, error) {
	var conv model.AgentConversation
	err := h.db.WithContext(ctx).
		Where("org_id = ? AND agent_id = ? AND source = ? AND source_id = ? AND source_resource_key = ? AND status = ?",
			*agent.OrgID, agent.ID, triggerConversationSource, triggerID, resourceKey, "active").
		First(&conv).Error
	if err == nil {
		return &conv, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("load trigger conversation: %w", err)
	}

	conv = model.AgentConversation{
		OrgID:                 *agent.OrgID,
		AgentID:               agent.ID,
		SandboxID:             sb.ID,
		RuntimeConversationID: conversationID,
		Source:                triggerConversationSource,
		SourceID:              &triggerID,
		SourceResourceKey:     resourceKey,
		Status:                "active",
		Name:                  "Trigger: " + resourceKey,
	}
	if err := h.db.WithContext(ctx).Create(&conv).Error; err != nil {
		return nil, fmt.Errorf("create trigger conversation: %w", err)
	}
	return &conv, nil
}
