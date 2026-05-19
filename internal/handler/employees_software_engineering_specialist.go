package handler

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/employeeruntime"
	"github.com/usehiveloop/hiveloop/internal/model"
)

func (h *EmployeeHandler) ensureSoftwareEngineeringSpecialistTx(ctx context.Context, tx *gorm.DB, employee *model.Agent) (*model.Agent, error) {
	if existing, err := findSoftwareEngineeringSpecialist(ctx, tx, employee.ID); err != nil {
		return nil, err
	} else if existing != nil {
		if err := updateSoftwareEngineeringSpecialist(ctx, tx, existing, employee); err != nil {
			return nil, err
		}
		return existing, nil
	}

	choice, err := pickEmployeeSubagentCredential(tx)
	if err != nil {
		choice = &employeeProviderChoice{model: employeeruntime.DefaultEmployeeSubagentModel}
	}
	desc := fmt.Sprintf("Software Engineering Specialist for %s. Handles implementation, debugging, codebase changes, and verification artifacts.", employee.Name)
	subagent := model.Agent{
		OrgID:          employee.OrgID,
		Description:    &desc,
		SystemPrompt:   softwareEngineeringSpecialistSystemPrompt,
		IdentityPrompt: softwareEngineeringSpecialistSystemPrompt,
		Model:          choice.model,
		Harness:        employeeCloudAgentHarness,
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
	if choice.cred != nil {
		subagent.CredentialID = &choice.cred.ID
	}
	baseSlug := employeeSoftwareEngineeringSpecialistBaseSlug(employee.Name)
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

func updateSoftwareEngineeringSpecialist(ctx context.Context, tx *gorm.DB, subagent *model.Agent, employee *model.Agent) error {
	desc := fmt.Sprintf("Software Engineering Specialist for %s. Handles implementation, debugging, codebase changes, and verification artifacts.", employee.Name)
	updates := map[string]any{
		"description":     desc,
		"category":        nil,
		"system_prompt":   softwareEngineeringSpecialistSystemPrompt,
		"identity_prompt": softwareEngineeringSpecialistSystemPrompt,
		"harness":         employeeCloudAgentHarness,
		"is_employee":     false,
		"status":          "active",
		"agent_config":    mergeAgentConfig(subagent.AgentConfig, softwareEngineeringSpecialistAgentConfig()),
	}
	if err := tx.WithContext(ctx).Model(subagent).Updates(updates).Error; err != nil {
		return err
	}
	subagent.Description = &desc
	subagent.Category = nil
	subagent.SystemPrompt = softwareEngineeringSpecialistSystemPrompt
	subagent.IdentityPrompt = softwareEngineeringSpecialistSystemPrompt
	subagent.Harness = employeeCloudAgentHarness
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

func employeeSoftwareEngineeringSpecialistBaseSlug(employeeName string) string {
	s := slugifyAgentName(employeeName)
	if s == "" {
		s = "hivy"
	}
	return s + "-software-engineering-specialist"
}
