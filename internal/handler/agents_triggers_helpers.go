package handler

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
)

// validateAgentTriggers checks per-type required fields on each trigger input
// and returns the first validation error formatted as "triggers[i]: ...".
// Returns "" when every trigger is well-formed (or the slice is empty).
func validateAgentTriggers(db *gorm.DB, orgID uuid.UUID, triggers []agentTriggerInput) string {
	for i, input := range triggers {
		triggerType := input.TriggerType
		if triggerType == "" {
			triggerType = "webhook"
		}
		switch triggerType {
		case "webhook":
			if input.ConnectionID == "" {
				return fmt.Sprintf("triggers[%d]: connection_id is required", i)
			}
			if _, err := uuid.Parse(input.ConnectionID); err != nil {
				return fmt.Sprintf("triggers[%d]: invalid connection_id", i)
			}
			if len(input.TriggerKeys) == 0 {
				return fmt.Sprintf("triggers[%d]: trigger_keys is required", i)
			}
			var inConn model.InConnection
			if err := db.Where("id = ? AND org_id = ?", input.ConnectionID, orgID).First(&inConn).Error; err != nil {
				return fmt.Sprintf("triggers[%d]: connection not found", i)
			}
		case "http":
			// No required fields.
		default:
			return fmt.Sprintf("triggers[%d]: invalid trigger_type %q", i, triggerType)
		}
	}
	return ""
}

// createAgentTriggers creates employee-owned AgentTrigger records inside an
// existing transaction. Connection IDs are in_connections IDs from the frontend.
func createAgentTriggers(tx *gorm.DB, orgID, agentID uuid.UUID, triggers []agentTriggerInput) error {
	if len(triggers) == 0 {
		return nil
	}

	for _, input := range triggers {
		triggerType := input.TriggerType
		if triggerType == "" {
			triggerType = "webhook"
		}

		trigger := model.AgentTrigger{
			OrgID:        orgID,
			AgentID:      agentID,
			Enabled:      true,
			TriggerType:  triggerType,
			Instructions: input.Instructions,
		}

		switch triggerType {
		case "webhook":
			connectionID, parseErr := uuid.Parse(input.ConnectionID)
			if parseErr != nil {
				return fmt.Errorf("invalid connection_id %q: %w", input.ConnectionID, parseErr)
			}
			trigger.ConnectionID = &connectionID
			trigger.TriggerKeys = pq.StringArray(input.TriggerKeys)

		case "http":
			trigger.TriggerKeys = pq.StringArray(input.TriggerKeys)
			if input.SecretKey != "" {
				hash, hashErr := bcrypt.GenerateFromPassword([]byte(input.SecretKey), bcrypt.DefaultCost)
				if hashErr != nil {
					return fmt.Errorf("hash trigger secret: %w", hashErr)
				}
				trigger.SecretKey = string(hash)
			}

		default:
			return fmt.Errorf("invalid trigger_type %q", triggerType)
		}

		if input.Conditions != nil && len(input.Conditions.Conditions) > 0 {
			conditionsJSON, _ := json.Marshal(input.Conditions)
			trigger.Conditions = conditionsJSON
		}

		if err := tx.Create(&trigger).Error; err != nil {
			return fmt.Errorf("create agent trigger: %w", err)
		}
	}
	return nil
}

// deleteAgentTriggers removes all trigger records owned by an agent.
func deleteAgentTriggers(db *gorm.DB, agentID uuid.UUID) error {
	if err := db.Where("agent_id = ?", agentID).Delete(&model.AgentTrigger{}).Error; err != nil {
		return fmt.Errorf("delete agent triggers: %w", err)
	}
	return nil
}
