package handler

type employeeAgentTemplate struct {
	Slug              string
	Name              string
	Description       string
	AgentType         string
	Version           int
	DefaultSkillNames []string
}

var (
	defaultEmployeeTemplateSkillNames = []string{"agent-browser", "git-github", "asset-uploads"}

	employeeAgentTemplates = []employeeAgentTemplate{
		{
			Slug:              "business-research-specialist",
			Name:              "Business Research Specialist",
			Description:       "Runs source-grounded research, market scans, and report generation for the employee.",
			AgentType:         "business_research",
			Version:           1,
			DefaultSkillNames: defaultEmployeeTemplateSkillNames,
		},
		{
			Slug:              "software-engineering-specialist",
			Name:              "Software Engineering Specialist",
			Description:       "Handles implementation, debugging, codebase changes, verification, and PR-ready engineering work.",
			AgentType:         "software_engineering",
			Version:           1,
			DefaultSkillNames: defaultEmployeeTemplateSkillNames,
		},
	}
)

func employeeAgentTemplateBySlug(slug string) *employeeAgentTemplate {
	for i := range employeeAgentTemplates {
		if employeeAgentTemplates[i].Slug == slug {
			return &employeeAgentTemplates[i]
		}
	}
	return nil
}
