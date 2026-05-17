package handler

import (
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
)

func deleteAgentNonCascadingReferences(db *gorm.DB, agentID uuid.UUID) error {
	if err := deleteAgentTriggers(db, agentID); err != nil {
		return err
	}
	if err := db.Exec(`DELETE FROM chat_messages WHERE session_id IN (SELECT id FROM chat_sessions WHERE agent_id = ?)`, agentID).Error; err != nil {
		return fmt.Errorf("delete chat messages: %w", err)
	}
	if err := db.Where("agent_id = ?", agentID).Delete(&model.ChatSession{}).Error; err != nil {
		return fmt.Errorf("delete chat sessions: %w", err)
	}
	if err := db.Where("agent_id = ?", agentID).Delete(&model.EmployeeAsset{}).Error; err != nil {
		return fmt.Errorf("delete employee assets: %w", err)
	}
	if err := db.Where("agent_id = ?", agentID).Delete(&model.HindsightBank{}).Error; err != nil {
		return fmt.Errorf("delete hindsight banks: %w", err)
	}
	if err := db.Where("agent_id = ?", agentID.String()).Delete(&model.ToolUsage{}).Error; err != nil {
		return fmt.Errorf("delete tool usage: %w", err)
	}
	if err := db.Exec(`DELETE FROM generations WHERE token_jti IN (SELECT jti FROM tokens WHERE meta->>'agent_id' = ?)`, agentID.String()).Error; err != nil {
		return fmt.Errorf("delete agent generations: %w", err)
	}
	if err := db.Where("meta->>'agent_id' = ?", agentID.String()).Delete(&model.Token{}).Error; err != nil {
		return fmt.Errorf("delete agent proxy tokens: %w", err)
	}
	return nil
}
