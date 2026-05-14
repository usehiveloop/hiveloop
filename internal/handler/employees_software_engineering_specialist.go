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

func (h *EmployeeHandler) ensureSoftwareEngineeringSpecialist(ctx context.Context, employee *model.Agent) (*model.Agent, error) {
	if employee == nil || employee.OrgID == nil {
		return nil, errors.New("employee must have org_id")
	}
	team, err := h.ensureEmployeeTeam(ctx, employee)
	if err != nil {
		return nil, err
	}
	var out *model.Agent
	err = h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		subagent, err := h.ensureSoftwareEngineeringSpecialistTx(ctx, tx, employee, team)
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

func (h *EmployeeHandler) ensureSoftwareEngineeringSpecialistTx(ctx context.Context, tx *gorm.DB, employee *model.Agent, team *model.Team) (*model.Agent, error) {
	if existing, err := findSoftwareEngineeringSpecialist(ctx, tx, employee.ID); err != nil {
		return nil, err
	} else if existing != nil {
		if err := updateSoftwareEngineeringSpecialist(ctx, tx, existing, employee, team); err != nil {
			return nil, err
		}
		return existing, nil
	}

	choice, err := pickEmployeeSubagentCredential(tx)
	if err != nil {
		return nil, err
	}
	desc := fmt.Sprintf("Software Engineering Specialist for %s. Handles implementation, debugging, codebase changes, and verification artifacts.", employee.Name)
	category := employeeCategory(employee)
	subagent := model.Agent{
		OrgID:          employee.OrgID,
		Description:    &desc,
		Category:       &category,
		SystemPrompt:   softwareEngineeringSpecialistSystemPrompt,
		IdentityPrompt: softwareEngineeringSpecialistSystemPrompt,
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
		AgentConfig:    softwareEngineeringSpecialistAgentConfig(),
		Permissions:    model.JSON{},
	}
	baseSlug := employeeSoftwareEngineeringSpecialistBaseSlug(employee.Name, category)
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

func findSoftwareEngineeringSpecialist(ctx context.Context, tx *gorm.DB, employeeID uuid.UUID) (*model.Agent, error) {
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
		if isSoftwareEngineeringSpecialist(&agents[i]) {
			return &agents[i], nil
		}
	}
	return nil, nil
}

func isSoftwareEngineeringSpecialist(agent *model.Agent) bool {
	if agent == nil {
		return false
	}
	if agent.AgentConfig != nil {
		if value, ok := agent.AgentConfig[defaultCloudAgentTypeKey].(string); ok && value == defaultSoftwareEngineeringSpecialistType {
			return true
		}
	}
	name := strings.ToLower(agent.Name)
	if strings.Contains(name, "software-engineering-specialist") {
		return true
	}
	return strings.Contains(agent.SystemPrompt, "Software Engineering Specialist") || strings.Contains(agent.IdentityPrompt, "Software Engineering Specialist")
}

func updateSoftwareEngineeringSpecialist(ctx context.Context, tx *gorm.DB, subagent *model.Agent, employee *model.Agent, team *model.Team) error {
	desc := fmt.Sprintf("Software Engineering Specialist for %s. Handles implementation, debugging, codebase changes, and verification artifacts.", employee.Name)
	category := employeeCategory(employee)
	updates := map[string]any{
		"description":     desc,
		"category":        category,
		"system_prompt":   softwareEngineeringSpecialistSystemPrompt,
		"identity_prompt": softwareEngineeringSpecialistSystemPrompt,
		"team_id":         team.ID,
		"team":            team.Name,
		"harness":         employeeHarness,
		"is_employee":     false,
		"status":          "active",
		"agent_config":    mergeAgentConfig(subagent.AgentConfig, softwareEngineeringSpecialistAgentConfig()),
	}
	if err := tx.WithContext(ctx).Model(subagent).Updates(updates).Error; err != nil {
		return err
	}
	subagent.Description = &desc
	subagent.Category = &category
	subagent.SystemPrompt = softwareEngineeringSpecialistSystemPrompt
	subagent.IdentityPrompt = softwareEngineeringSpecialistSystemPrompt
	subagent.TeamID = &team.ID
	subagent.Team = team.Name
	subagent.Harness = employeeHarness
	subagent.IsEmployee = false
	subagent.Status = "active"
	subagent.AgentConfig = updates["agent_config"].(model.JSON)
	return nil
}

func softwareEngineeringSpecialistAgentConfig() model.JSON {
	return model.JSON{
		defaultCloudAgentTypeKey:    defaultSoftwareEngineeringSpecialistType,
		defaultCloudAgentVersionKey: defaultSoftwareEngineeringSpecialistVersion,
	}
}

func employeeSoftwareEngineeringSpecialistBaseSlug(employeeName, category string) string {
	s := slugifyAgentName(employeeName)
	if s == "" {
		s = category
	}
	return s + "-software-engineering-specialist"
}
