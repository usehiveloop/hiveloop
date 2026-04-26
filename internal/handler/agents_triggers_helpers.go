package handler

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
)

// validateAgentTriggers checks per-type required fields on each trigger input
// and returns the first validation error formatted as "triggers[i]: ...".
// Returns "" when every trigger is well-formed (or the slice is empty).
//
// Webhook triggers must reference an in_connection that belongs to orgID.
// HTTP triggers have no required fields. Cron triggers must carry a non-empty
// cron_schedule; the expression itself is parsed in createAgentTriggers so the
// error path stays in one place.
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
		case "cron":
			if input.CronSchedule == "" {
				return fmt.Sprintf("triggers[%d]: cron_schedule is required", i)
			}
		default:
			return fmt.Sprintf("triggers[%d]: invalid trigger_type %q", i, triggerType)
		}
	}
	return ""
}

// createAgentTriggers creates RouterTrigger + RoutingRule records for an agent
// inside an existing transaction. The router is found or created for the org.
// Connection IDs are in_connections IDs from the frontend.
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
		triggerType := input.TriggerType
		if triggerType == "" {
			triggerType = "webhook"
		}

		trigger := model.RouterTrigger{
			OrgID:        orgID,
			RouterID:     router.ID,
			Enabled:      true,
			TriggerType:  triggerType,
			RoutingMode:  "rule",
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

		case "cron":
			if input.CronSchedule == "" {
				return fmt.Errorf("cron_schedule is required for cron triggers")
			}
			nextRun, parseErr := computeNextRun(input.CronSchedule)
			if parseErr != nil {
				return fmt.Errorf("invalid cron_schedule %q: %w", input.CronSchedule, parseErr)
			}
			trigger.CronSchedule = input.CronSchedule
			trigger.NextRunAt = &nextRun

		default:
			return fmt.Errorf("invalid trigger_type %q", triggerType)
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

// deleteAgentTriggers removes all RouterTrigger + RoutingRule records owned by
// an agent. CASCADE on the RouterTrigger FK deletes the RoutingRule rows.
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
