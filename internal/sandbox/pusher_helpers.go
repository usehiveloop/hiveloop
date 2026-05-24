package sandbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	bridgepkg "github.com/usehivy/hivy/internal/bridge"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

func harnessFromAgent(agentHarness string) bridgepkg.Harness {
	if agentHarness == "" {
		return bridgepkg.OpenCode
	}
	return bridgepkg.Harness(agentHarness)
}

func buildHivyMCPServer(mcpBaseURL, jti, token string) bridgepkg.McpServerDefinition {
	url := fmt.Sprintf("%s/%s", mcpBaseURL, jti)

	var transport bridgepkg.McpTransport
	httpTransport := bridgepkg.McpTransport1{
		Type: bridgepkg.StreamableHttp,
		Url:  url,
	}
	if token != "" {
		headers := map[string]string{"Authorization": "Bearer " + token}
		httpTransport.Headers = &headers
	}
	_ = transport.FromMcpTransport1(httpTransport)

	return bridgepkg.McpServerDefinition{
		Name:      "hivy",
		Transport: transport,
	}
}

func buildScopesFromIntegrations(integrations model.JSON) []map[string]any {
	if len(integrations) == 0 {
		return nil
	}

	var scopes []map[string]any
	for connectionID, config := range integrations {
		configMap, ok := config.(map[string]any)
		if !ok {
			continue
		}
		actionsRaw, ok := configMap["actions"]
		if !ok {
			continue
		}
		actionsSlice, ok := actionsRaw.([]any)
		if !ok {
			continue
		}
		actions := make([]string, 0, len(actionsSlice))
		for _, action := range actionsSlice {
			if actionStr, ok := action.(string); ok {
				actions = append(actions, actionStr)
			}
		}
		if len(actions) > 0 {
			scopes = append(scopes, map[string]any{
				"connection_id": connectionID,
				"actions":       actions,
			})
		}
	}

	return scopes
}

func (p *Pusher) loadOwningEmployee(ctx context.Context, agent *model.Employee) (*model.Employee, error) {
	if agent == nil || agent.IsEmployee || agent.OrgID == nil {
		return nil, nil
	}
	var employee model.Employee
	err := p.db.WithContext(ctx).
		Where("org_id = ? AND status <> ?", *agent.OrgID, "archived").
		Order("created_at ASC").
		First(&employee).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &employee, nil
}

func mergeAgentIntegrationsForAccess(agent *model.Employee, employee *model.Employee) model.JSON {
	if agent == nil {
		return model.JSON{}
	}
	if employee == nil {
		return agent.Integrations
	}
	return mergeJSONMaps(employee.Integrations, agent.Integrations)
}

func mergeAgentResourcesForContext(agent *model.Employee, employee *model.Employee) model.JSON {
	if agent == nil {
		return model.JSON{}
	}
	if employee == nil {
		return agent.Resources
	}
	return mergeJSONMaps(employee.Resources, agent.Resources)
}

func decodeJSONAs[T any](j model.JSON) *T {
	if len(j) == 0 {
		return nil
	}
	b, err := json.Marshal(j)
	if err != nil {
		return nil
	}
	var result T
	if err := json.Unmarshal(b, &result); err != nil {
		return nil
	}
	return &result
}

func (p *Pusher) loadBridgeSkills(ctx context.Context, agentID uuid.UUID, inheritedAgentIDs ...uuid.UUID) []bridgepkg.SkillDefinition {
	seenAgentIDs := map[uuid.UUID]bool{}
	agentIDs := appendUniqueUUIDs(nil, seenAgentIDs, inheritedAgentIDs...)
	agentIDs = appendUniqueUUIDs(agentIDs, seenAgentIDs, agentID)

	var links []model.EmployeeSkill
	if err := p.db.WithContext(ctx).Where("employee_id IN ?", agentIDs).Find(&links).Error; err != nil || len(links) == 0 {
		return nil
	}

	skillIDSeen := map[uuid.UUID]bool{}
	skillIDs := make([]uuid.UUID, 0, len(links))
	for _, link := range links {
		skillIDs = appendUniqueUUIDs(skillIDs, skillIDSeen, link.SkillID)
	}

	var skills []model.Skill
	if err := p.db.WithContext(ctx).Where("id IN ?", skillIDs).Find(&skills).Error; err != nil {
		return nil
	}

	var result []bridgepkg.SkillDefinition
	for _, skill := range skills {
		if len(skill.Bundle) == 0 {
			continue
		}
		var def bridgepkg.SkillDefinition
		if err := json.Unmarshal(skill.Bundle, &def); err != nil {
			logging.Capture(ctx, fmt.Errorf("unmarshal skill bundle %s: %w", skill.ID, err))
			continue
		}
		result = append(result, def)
	}

	return result
}
