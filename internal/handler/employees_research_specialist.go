package handler

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
)

const (
	defaultCloudAgentTypeKey                 = "default_cloud_agent_type"
	defaultCloudAgentVersionKey              = "default_cloud_agent_version"
	defaultBusinessResearchSpecialistType    = "business_research_specialist"
	defaultBusinessResearchSpecialistVersion = 1
)

func (h *EmployeeHandler) ensureBusinessResearchSpecialist(ctx context.Context, employee *model.Agent) (*model.Agent, error) {
	if employee == nil || employee.OrgID == nil {
		return nil, errors.New("employee must have org_id")
	}
	team, err := h.ensureEmployeeTeam(ctx, employee)
	if err != nil {
		return nil, err
	}
	var out *model.Agent
	err = h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		subagent, err := h.ensureBusinessResearchSpecialistTx(ctx, tx, employee, team)
		if err != nil {
			return err
		}
		out = subagent
		return nil
	})
	if err != nil {
		return nil, err
	}
	h.attachGlobalSkills(ctx, out.ID, defaultEmployeeSubagentSkills[employeeCategory(employee)])
	return out, nil
}

func (h *EmployeeHandler) ensureEmployeeTeam(ctx context.Context, employee *model.Agent) (*model.Team, error) {
	if employee.TeamID != nil {
		var team model.Team
		if err := h.db.WithContext(ctx).Where("id = ? AND org_id = ?", *employee.TeamID, *employee.OrgID).First(&team).Error; err == nil {
			return &team, nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("lookup employee team: %w", err)
		}
	}
	team, err := ensureEngineeringTeam(h.db.WithContext(ctx), *employee.OrgID)
	if err != nil {
		return nil, err
	}
	if employee.TeamID == nil || *employee.TeamID != team.ID || employee.Team == "" {
		updates := map[string]any{"team_id": team.ID, "team": team.Name}
		if err := h.db.WithContext(ctx).Model(employee).Updates(updates).Error; err != nil {
			return nil, fmt.Errorf("update employee team: %w", err)
		}
		employee.TeamID = &team.ID
		employee.Team = team.Name
	}
	return team, nil
}

func (h *EmployeeHandler) ensureBusinessResearchSpecialistTx(ctx context.Context, tx *gorm.DB, employee *model.Agent, team *model.Team) (*model.Agent, error) {
	if existing, err := findBusinessResearchSpecialist(ctx, tx, employee.ID); err != nil {
		return nil, err
	} else if existing != nil {
		if err := updateBusinessResearchSpecialist(ctx, tx, existing, employee, team); err != nil {
			return nil, err
		}
		return existing, nil
	}

	choice, err := pickEmployeeSubagentCredential(tx)
	if err != nil {
		return nil, err
	}
	desc := fmt.Sprintf("Business Research Specialist for %s. Handles broad research and writes source-grounded reports to employee assets.", employee.Name)
	category := employeeCategory(employee)
	subagent := model.Agent{
		OrgID:          employee.OrgID,
		Description:    &desc,
		Category:       &category,
		SystemPrompt:   researchSpecialistSystemPrompt,
		IdentityPrompt: researchSpecialistSystemPrompt,
		Model:          choice.model,
		CredentialID:   &choice.cred.ID,
		TeamID:         &team.ID,
		Team:           team.Name,
		Harness:        employeeHarness,
		IsEmployee:     false,
		Status:         "active",
		Tools:          model.JSON{},
		McpServers:     model.JSON{},
		Skills:         model.JSON{},
		Integrations:   model.JSON{},
		Resources:      model.JSON{},
		AgentConfig:    businessResearchSpecialistAgentConfig(),
		Permissions:    model.JSON{},
	}
	baseSlug := employeeBusinessResearchSpecialistBaseSlug(employee.Name, category)
	if err := createWithUniqueNameSlug(tx, &subagent, baseSlug); err != nil {
		return nil, err
	}
	if err := tx.WithContext(ctx).Create(&model.AgentSubagent{
		AgentID:    employee.ID,
		SubagentID: subagent.ID,
	}).Error; err != nil {
		return nil, err
	}
	return &subagent, nil
}

func findBusinessResearchSpecialist(ctx context.Context, tx *gorm.DB, employeeID uuid.UUID) (*model.Agent, error) {
	var links []model.AgentSubagent
	if err := tx.WithContext(ctx).Where("agent_id = ?", employeeID).Find(&links).Error; err != nil {
		return nil, err
	}
	if len(links) == 0 {
		return nil, nil
	}
	subIDs := make([]uuid.UUID, 0, len(links))
	for _, link := range links {
		subIDs = append(subIDs, link.SubagentID)
	}
	var agents []model.Agent
	if err := tx.WithContext(ctx).Where("id IN ?", subIDs).Find(&agents).Error; err != nil {
		return nil, err
	}
	for i := range agents {
		if isBusinessResearchSpecialist(&agents[i]) {
			return &agents[i], nil
		}
	}
	return nil, nil
}

func isBusinessResearchSpecialist(agent *model.Agent) bool {
	if agent == nil {
		return false
	}
	if agent.AgentConfig != nil {
		if value, ok := agent.AgentConfig[defaultCloudAgentTypeKey].(string); ok && value == defaultBusinessResearchSpecialistType {
			return true
		}
	}
	name := strings.ToLower(agent.Name)
	if strings.Contains(name, "research-specialist") {
		return true
	}
	return strings.Contains(agent.SystemPrompt, "Research Specialist") || strings.Contains(agent.IdentityPrompt, "Research Specialist")
}

func updateBusinessResearchSpecialist(ctx context.Context, tx *gorm.DB, subagent *model.Agent, employee *model.Agent, team *model.Team) error {
	desc := fmt.Sprintf("Business Research Specialist for %s. Handles broad research and writes source-grounded reports to employee assets.", employee.Name)
	category := employeeCategory(employee)
	updates := map[string]any{
		"description":     desc,
		"category":        category,
		"system_prompt":   researchSpecialistSystemPrompt,
		"identity_prompt": researchSpecialistSystemPrompt,
		"team_id":         team.ID,
		"team":            team.Name,
		"harness":         employeeHarness,
		"is_employee":     false,
		"status":          "active",
		"agent_config":    mergeAgentConfig(subagent.AgentConfig, businessResearchSpecialistAgentConfig()),
	}
	if err := tx.WithContext(ctx).Model(subagent).Updates(updates).Error; err != nil {
		return err
	}
	subagent.Description = &desc
	subagent.Category = &category
	subagent.SystemPrompt = researchSpecialistSystemPrompt
	subagent.IdentityPrompt = researchSpecialistSystemPrompt
	subagent.TeamID = &team.ID
	subagent.Team = team.Name
	subagent.Harness = employeeHarness
	subagent.IsEmployee = false
	subagent.Status = "active"
	subagent.AgentConfig = updates["agent_config"].(model.JSON)
	return nil
}

func businessResearchSpecialistAgentConfig() model.JSON {
	return model.JSON{
		defaultCloudAgentTypeKey:    defaultBusinessResearchSpecialistType,
		defaultCloudAgentVersionKey: defaultBusinessResearchSpecialistVersion,
		"asset_output_contract":     "research/{task_id}/report.md, research/{task_id}/sources.json, research/{task_id}/summary.md",
	}
}

func mergeAgentConfig(existing model.JSON, next model.JSON) model.JSON {
	out := model.JSON{}
	for k, v := range existing {
		out[k] = v
	}
	for k, v := range next {
		out[k] = v
	}
	return out
}

func employeeCategory(agent *model.Agent) string {
	if agent != nil && agent.Category != nil && strings.TrimSpace(*agent.Category) != "" {
		return strings.TrimSpace(*agent.Category)
	}
	return employeeCategoryEngineering
}

func employeeBusinessResearchSpecialistBaseSlug(employeeName, category string) string {
	s := slugifyAgentName(employeeName)
	if s == "" {
		s = category
	}
	return s + "-business-research-specialist"
}
