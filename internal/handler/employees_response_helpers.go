package handler

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
)

func validateHarness(harness string) error {
	switch harness {
	case "", "claude", "open_code":
		return nil
	default:
		return fmt.Errorf("invalid harness %q (must be 'claude' or 'open_code')", harness)
	}
}

// loadEmployeeTriggers loads triggers configured for one or more employees.
// Returns a map from agent ID to trigger responses. Uses a single query with
// joins to avoid N+1.
func (h *EmployeeHandler) loadEmployeeTriggers(agentIDs ...uuid.UUID) map[uuid.UUID][]employeeTriggerResponse {
	if len(agentIDs) == 0 {
		return nil
	}

	type triggerRow struct {
		AgentID      uuid.UUID      `gorm:"column:agent_id"`
		TriggerID    uuid.UUID      `gorm:"column:trigger_id"`
		TriggerType  string         `gorm:"column:trigger_type"`
		ConnID       *uuid.UUID     `gorm:"column:conn_id"`
		Provider     *string        `gorm:"column:provider"`
		TriggerKeys  pq.StringArray `gorm:"column:trigger_keys;type:text[]"`
		Enabled      bool           `gorm:"column:enabled"`
		Conditions   model.RawJSON  `gorm:"column:conditions"`
		Instructions string         `gorm:"column:instructions"`
		SecretKey    string         `gorm:"column:secret_key"`
	}

	var rows []triggerRow
	h.db.Raw(`
		SELECT
			at.agent_id,
			at.id AS trigger_id,
			at.trigger_type,
			at.connection_id AS conn_id,
			ii.provider,
			at.trigger_keys,
			at.enabled,
			at.conditions,
			at.instructions,
			at.secret_key
		FROM employee_triggers at
		LEFT JOIN in_connections ic ON ic.id = at.connection_id
		LEFT JOIN in_integrations ii ON ii.id = ic.in_integration_id
		WHERE at.agent_id IN ?
		ORDER BY at.id ASC
	`, agentIDs).Scan(&rows)

	result := make(map[uuid.UUID][]employeeTriggerResponse, len(agentIDs))
	for _, row := range rows {
		var conditions any
		if len(row.Conditions) > 0 {
			var parsed model.TriggerMatch
			if err := json.Unmarshal(row.Conditions, &parsed); err == nil && len(parsed.Conditions) > 0 {
				conditions = parsed
			}
		}

		response := employeeTriggerResponse{
			ID:           row.TriggerID.String(),
			TriggerType:  row.TriggerType,
			TriggerKeys:  []string(row.TriggerKeys),
			Enabled:      row.Enabled,
			Conditions:   conditions,
			Instructions: row.Instructions,
			SecretSet:    row.SecretKey != "",
		}
		if row.ConnID != nil {
			response.ConnectionID = row.ConnID.String()
		}
		if row.Provider != nil {
			response.Provider = *row.Provider
		}

		result[row.AgentID] = append(result[row.AgentID], response)
	}
	return result
}

// loadEmployeeSkills batch-loads attached skill summaries for one or more employees.
func (h *EmployeeHandler) loadEmployeeSkills(agentIDs ...uuid.UUID) map[uuid.UUID][]employeeSkillSummary {
	if len(agentIDs) == 0 {
		return nil
	}
	var links []model.AgentSkill
	if err := h.db.Where("agent_id IN ?", agentIDs).Find(&links).Error; err != nil {
		return nil
	}
	if len(links) == 0 {
		return nil
	}
	skillIDs := make([]uuid.UUID, len(links))
	for index, link := range links {
		skillIDs[index] = link.SkillID
	}
	var skills []model.Skill
	if err := h.db.Select("id, name, description, source_type").Where("id IN ? AND hidden = false", skillIDs).Find(&skills).Error; err != nil {
		return nil
	}
	skillByID := make(map[uuid.UUID]model.Skill, len(skills))
	for _, skill := range skills {
		skillByID[skill.ID] = skill
	}
	result := make(map[uuid.UUID][]employeeSkillSummary, len(agentIDs))
	for _, link := range links {
		skill, ok := skillByID[link.SkillID]
		if !ok {
			continue
		}
		result[link.AgentID] = append(result[link.AgentID], employeeSkillSummary{
			ID:          skill.ID.String(),
			Name:        skill.Name,
			Description: skill.Description,
			SourceType:  skill.SourceType,
		})
	}
	return result
}

// errGitHubAppExclusive is returned when an agent's integrations payload
// attaches both GitHub Apps to the same agent. We restrict agents to a single
// GitHub App identity so it's unambiguous which app authored a given action
// (the primary opens PRs, the code-reviews app reviews them on a different
// agent).
var errGitHubAppExclusive = errors.New("an agent can connect to only one of github-app or github-app-code-reviews, not both")

// validateEmployeeIntegrationsExclusivity checks the proposed integrations map
// against mutually-exclusive provider rules. integrations is keyed by
// connection UUID (matching the JSONB shape on employees.integrations); we
// resolve those connections to providers via in_connections → in_integrations
// scoped to the org.
func validateEmployeeIntegrationsExclusivity(db *gorm.DB, orgID uuid.UUID, integrations model.JSON) error {
	if len(integrations) == 0 {
		return nil
	}
	connectionIDs := make([]uuid.UUID, 0, len(integrations))
	for key := range integrations {
		id, err := uuid.Parse(key)
		if err != nil {
			continue
		}
		connectionIDs = append(connectionIDs, id)
	}
	if len(connectionIDs) == 0 {
		return nil
	}

	type row struct {
		Provider string
	}
	var rows []row
	err := db.
		Table("in_connections").
		Select("DISTINCT in_integrations.provider AS provider").
		Joins("JOIN in_integrations ON in_integrations.id = in_connections.in_integration_id AND in_integrations.deleted_at IS NULL").
		Where("in_connections.id IN ? AND in_connections.org_id = ? AND in_connections.revoked_at IS NULL", connectionIDs, orgID).
		Scan(&rows).Error
	if err != nil {
		return fmt.Errorf("resolving integration providers: %w", err)
	}

	hasPrimary, hasReviews := false, false
	for _, r := range rows {
		switch r.Provider {
		case "github-app":
			hasPrimary = true
		case "github-app-code-reviews":
			hasReviews = true
		}
	}
	if hasPrimary && hasReviews {
		return errGitHubAppExclusive
	}
	return nil
}

func validateEmployeeModel(reg *registry.Registry, modelID string) error {
	if reg == nil || modelID == "" {
		return nil
	}
	return reg.ValidateCanonicalModel(modelID)
}
