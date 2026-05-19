package handler

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
)

type employeeAgentTemplate struct {
	Slug              string
	Name              string
	Description       string
	AgentType         string
	Version           int
	DefaultSkillNames []string
	Matches           func(*model.Agent) bool
	EnsureTx          func(context.Context, *EmployeeHandler, *gorm.DB, *model.Agent) (*model.Agent, error)
}

type employeeAgentTemplateResponse struct {
	Slug        string  `json:"slug"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	AgentType   string  `json:"agent_type"`
	Version     int     `json:"version"`
	Installed   bool    `json:"installed"`
	SubagentID  *string `json:"subagent_id,omitempty"`
}

type installEmployeeAgentTemplateResponse struct {
	Template employeeAgentTemplateResponse `json:"template"`
	Subagent employeeSubagentSummary       `json:"subagent"`
	Sync     syncEmployeeResponse          `json:"sync"`
}

var (
	defaultEmployeeTemplateSkillNames = []string{"agent-browser", "git-github", "asset-uploads"}

	employeeAgentTemplates = []employeeAgentTemplate{
		{
			Slug:              "business-research-specialist",
			Name:              "Business Research Specialist",
			Description:       "Runs source-grounded research, market scans, and report generation for the employee.",
			AgentType:         defaultBusinessResearchSpecialistType,
			Version:           defaultBusinessResearchSpecialistVersion,
			DefaultSkillNames: defaultEmployeeTemplateSkillNames,
			Matches:           isBusinessResearchSpecialist,
			EnsureTx: func(ctx context.Context, h *EmployeeHandler, tx *gorm.DB, employee *model.Agent) (*model.Agent, error) {
				return h.ensureBusinessResearchSpecialistTx(ctx, tx, employee)
			},
		},
		{
			Slug:              "software-engineering-specialist",
			Name:              "Software Engineering Specialist",
			Description:       "Handles implementation, debugging, codebase changes, verification, and PR-ready engineering work.",
			AgentType:         defaultSoftwareEngineeringSpecialistType,
			Version:           defaultSoftwareEngineeringSpecialistVersion,
			DefaultSkillNames: defaultEmployeeTemplateSkillNames,
			Matches:           isSoftwareEngineeringSpecialist,
			EnsureTx: func(ctx context.Context, h *EmployeeHandler, tx *gorm.DB, employee *model.Agent) (*model.Agent, error) {
				return h.ensureSoftwareEngineeringSpecialistTx(ctx, tx, employee)
			},
		},
	}
)

func employeeAgentTemplatesForEmployee() []*employeeAgentTemplate {
	out := make([]*employeeAgentTemplate, 0, len(employeeAgentTemplates))
	for i := range employeeAgentTemplates {
		out = append(out, &employeeAgentTemplates[i])
	}
	return out
}

func (h *EmployeeHandler) ensureEmployeeAgentTemplates(ctx context.Context, employee *model.Agent) ([]*model.Agent, error) {
	if employee == nil || employee.OrgID == nil {
		return nil, errors.New("employee must have org_id")
	}
	var out []*model.Agent
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		subagents, err := h.ensureEmployeeAgentTemplatesTx(ctx, tx, employee)
		if err != nil {
			return err
		}
		out = subagents
		return nil
	})
	if err != nil {
		return nil, err
	}
	h.attachEmployeeAgentTemplateSkills(ctx, out...)
	return out, nil
}

func (h *EmployeeHandler) ensureEmployeeAgentTemplatesTx(ctx context.Context, tx *gorm.DB, employee *model.Agent) ([]*model.Agent, error) {
	templates := employeeAgentTemplatesForEmployee()
	out := make([]*model.Agent, 0, len(templates))
	for _, template := range templates {
		subagent, err := template.EnsureTx(ctx, h, tx, employee)
		if err != nil {
			return nil, err
		}
		out = append(out, subagent)
	}
	return out, nil
}

func (h *EmployeeHandler) attachEmployeeAgentTemplateSkills(ctx context.Context, subagents ...*model.Agent) {
	for _, subagent := range subagents {
		if subagent == nil {
			continue
		}
		template := employeeAgentTemplateForSubagent(subagent)
		if template == nil {
			continue
		}
		h.attachGlobalSkills(ctx, subagent.ID, template.DefaultSkillNames)
	}
}

func employeeAgentTemplateResponses(installed map[string]*model.Agent) []employeeAgentTemplateResponse {
	out := make([]employeeAgentTemplateResponse, 0, len(employeeAgentTemplates))
	for i := range employeeAgentTemplates {
		t := &employeeAgentTemplates[i]
		out = append(out, t.toResponse(installed))
	}
	return out
}

func (t *employeeAgentTemplate) toResponse(installed map[string]*model.Agent) employeeAgentTemplateResponse {
	resp := employeeAgentTemplateResponse{
		Slug:        t.Slug,
		Name:        t.Name,
		Description: t.Description,
		AgentType:   t.AgentType,
		Version:     t.Version,
	}
	if subagent := installed[t.Slug]; subagent != nil {
		resp.Installed = true
		id := subagent.ID.String()
		resp.SubagentID = &id
	}
	return resp
}

func employeeAgentTemplateBySlug(slug string) *employeeAgentTemplate {
	for i := range employeeAgentTemplates {
		if employeeAgentTemplates[i].Slug == slug {
			return &employeeAgentTemplates[i]
		}
	}
	return nil
}

func employeeAgentTemplateForSubagent(agent *model.Agent) *employeeAgentTemplate {
	for i := range employeeAgentTemplates {
		if employeeAgentTemplates[i].Matches(agent) {
			return &employeeAgentTemplates[i]
		}
	}
	return nil
}

func loadEmployeeTemplateSubagents(ctx context.Context, db *gorm.DB, employeeID uuid.UUID) (map[string]*model.Agent, error) {
	var links []model.AgentSubagent
	if err := db.WithContext(ctx).Where("agent_id = ?", employeeID).Find(&links).Error; err != nil {
		return nil, err
	}
	if len(links) == 0 {
		return map[string]*model.Agent{}, nil
	}
	subIDs := make([]uuid.UUID, 0, len(links))
	for _, link := range links {
		subIDs = append(subIDs, link.SubagentID)
	}
	var agents []model.Agent
	if err := db.WithContext(ctx).
		Where("id IN ?", subIDs).
		Find(&agents).Error; err != nil {
		return nil, err
	}
	out := make(map[string]*model.Agent, len(agents))
	for i := range agents {
		template := employeeAgentTemplateForSubagent(&agents[i])
		if template == nil {
			continue
		}
		agent := agents[i]
		out[template.Slug] = &agent
	}
	return out, nil
}

func employeeSubagentSummaryFromAgent(agent model.Agent) employeeSubagentSummary {
	summary := employeeSubagentSummary{
		ID:          agent.ID.String(),
		Name:        agent.Name,
		AvatarURL:   agent.AvatarURL,
		Description: agent.Description,
		Status:      agent.Status,
	}
	if template := employeeAgentTemplateForSubagent(&agent); template != nil {
		slug := template.Slug
		agentType := template.AgentType
		summary.TemplateSlug = &slug
		summary.TemplateAgentType = &agentType
	}
	return summary
}
