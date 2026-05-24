package handler

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// validateEmployeeTriggers checks per-type required fields on each trigger input
// and returns the first validation error formatted as "triggers[i]: ...".
// Returns "" when every trigger is well-formed (or the slice is empty).
func validateEmployeeTriggers(db *gorm.DB, orgID uuid.UUID, triggers []employeeTriggerInput) string {
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
			var conn model.Connection
			if err := db.Where("id = ? AND org_id = ?", input.ConnectionID, orgID).First(&conn).Error; err != nil {
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

func replaceEmployeeTriggers(tx *gorm.DB, orgID, agentID uuid.UUID, triggers []employeeTriggerInput) error {
	existingSecrets := map[uuid.UUID]string{}
	var existing []model.EmployeeTrigger
	if err := tx.Where("employee_id = ?", agentID).Find(&existing).Error; err != nil {
		return err
	}
	for _, trigger := range existing {
		if trigger.SecretKey != "" {
			existingSecrets[trigger.ID] = trigger.SecretKey
		}
	}
	if err := deleteEmployeeTriggers(tx, agentID); err != nil {
		return err
	}
	return createEmployeeTriggersWithExistingSecrets(tx, orgID, agentID, triggers, existingSecrets)
}

// createEmployeeTriggers creates employee-owned EmployeeTrigger records inside an
// existing transaction. Connection IDs are connections IDs from the frontend.
func createEmployeeTriggers(tx *gorm.DB, orgID, agentID uuid.UUID, triggers []employeeTriggerInput) error {
	return createEmployeeTriggersWithExistingSecrets(tx, orgID, agentID, triggers, nil)
}

func createEmployeeTriggersWithExistingSecrets(tx *gorm.DB, orgID, agentID uuid.UUID, triggers []employeeTriggerInput, existingSecrets map[uuid.UUID]string) error {
	if len(triggers) == 0 {
		return nil
	}

	for _, input := range triggers {
		var existingID uuid.UUID
		if input.ID != "" {
			parsedID, parseErr := uuid.Parse(input.ID)
			if parseErr != nil {
				return fmt.Errorf("invalid trigger id %q: %w", input.ID, parseErr)
			}
			existingID = parsedID
		}

		triggerType := input.TriggerType
		if triggerType == "" {
			triggerType = "webhook"
		}

		trigger := model.EmployeeTrigger{
			OrgID:        orgID,
			EmployeeID:   agentID,
			Enabled:      true,
			TriggerType:  triggerType,
			Instructions: input.Instructions,
		}
		if existingID != uuid.Nil {
			trigger.ID = existingID
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
			} else if existingID != uuid.Nil && existingSecrets != nil {
				trigger.SecretKey = existingSecrets[existingID]
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

// deleteEmployeeTriggers removes all trigger records owned by an agent.
func deleteEmployeeTriggers(db *gorm.DB, agentID uuid.UUID) error {
	if err := db.Where("employee_id = ?", agentID).Delete(&model.EmployeeTrigger{}).Error; err != nil {
		return fmt.Errorf("delete agent triggers: %w", err)
	}
	return nil
}
