package handler

type specialistTemplate struct {
	Slug              string
	Name              string
	Description       string
	SpecialistType    string
	Version           int
	DefaultSkillNames []string
}

var (
	defaultSpecialistTemplateSkillNames = []string{"agent-browser", "git-github", "asset-uploads"}

	specialistTemplates = []specialistTemplate{
		{
			Slug:              "business-research-specialist",
			Name:              "Business Research Specialist",
			Description:       "Runs source-grounded research, market scans, and report generation for the employee.",
			SpecialistType:    "business_research",
			Version:           1,
			DefaultSkillNames: defaultSpecialistTemplateSkillNames,
		},
		{
			Slug:              "software-engineering-specialist",
			Name:              "Software Engineering Specialist",
			Description:       "Handles implementation, debugging, codebase changes, verification, and PR-ready engineering work.",
			SpecialistType:    "software_engineering",
			Version:           1,
			DefaultSkillNames: defaultSpecialistTemplateSkillNames,
		},
	}
)

func specialistTemplateBySlug(slug string) *specialistTemplate {
	for i := range specialistTemplates {
		if specialistTemplates[i].Slug == slug {
			return &specialistTemplates[i]
		}
	}
	return nil
}
