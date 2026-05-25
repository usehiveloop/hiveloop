package employeeruntime

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/specialists"
)

func TestCompileSpecialist_IncludesDefinitionDefaultSkillsAndEmployeeSkills(t *testing.T) {
	db := connectCompileTestDB(t)
	org := model.Org{Name: "Specialist skills-" + uuid.NewString()}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	category := "engineering"
	agent := model.Employee{
		ID:            uuid.New(),
		OrgID:         &org.ID,
		Name:          "Aria",
		Category:      &category,
		Model:         DefaultEmployeeModel,
		Tools:         model.JSON{},
		McpServers:    model.JSON{},
		Skills:        model.JSON{},
		Integrations:  model.JSON{},
		Resources:     model.JSON{},
		RuntimeConfig: model.JSON{},
		Permissions:   model.JSON{},
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	suffix := uuid.NewString()
	employeeSkill := compileTestSkill("employee-skill-"+suffix, "Employee Skill "+suffix, &org.ID)
	defaultSkill := compileTestSkill("default-skill-"+suffix, "default-skill-"+suffix, nil)
	sharedDefault := compileTestSkill("shared-skill-"+suffix, "shared-skill-"+suffix, nil)
	for _, skill := range []model.Skill{employeeSkill, defaultSkill, sharedDefault} {
		if err := db.Create(&skill).Error; err != nil {
			t.Fatalf("create skill %s: %v", skill.Slug, err)
		}
	}
	if err := db.Create(&model.EmployeeSkill{EmployeeID: agent.ID, SkillID: employeeSkill.ID}).Error; err != nil {
		t.Fatalf("attach employee skill: %v", err)
	}
	if err := db.Create(&model.EmployeeSkill{EmployeeID: agent.ID, SkillID: sharedDefault.ID}).Error; err != nil {
		t.Fatalf("attach shared skill: %v", err)
	}

	def, err := CompileSpecialist(context.Background(), CompileDeps{DB: db, Cfg: &config.Config{}}, &agent, specialists.Definition{
		Slug:              "software-engineering-specialist",
		Name:              "Software Engineering Specialist",
		Description:       "Handles implementation work.",
		SpecialistType:    "software_engineering",
		Version:           1,
		DefaultModel:      "deepseek-v4-pro",
		DefaultSkillNames: []string{defaultSkill.Name, sharedDefault.Name, defaultSkill.Name},
		SystemPrompt:      "Do engineering work.",
	})
	if err != nil {
		t.Fatalf("compile specialist: %v", err)
	}
	got := map[string]int{}
	for _, skill := range def.Skills {
		got[skill.Name]++
	}
	for _, want := range []string{employeeSkill.Slug, defaultSkill.Slug, sharedDefault.Slug} {
		if got[want] != 1 {
			t.Fatalf("compiled skill %q count = %d, all skills = %#v", want, got[want], got)
		}
	}
}
