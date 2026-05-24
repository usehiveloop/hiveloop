package employeeruntime

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
)

func TestCompile_EmitsTypedPromptFragmentsWithoutRawSystemPrompt(t *testing.T) {
	db := connectCompileTestDB(t)
	orgName := "Acme-" + uuid.NewString()
	org := model.Org{
		Name:          orgName,
		PromptCompany: "Acme builds deployment tools for engineering teams.",
	}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", org.ID).Delete(&model.Org{}) })
	description := "Coordinates platform engineering work."
	category := "engineering"
	agent := model.Employee{
		ID:                        uuid.New(),
		OrgID:                     &org.ID,
		Name:                      "Aria",
		Description:               &description,
		Category:                  &category,
		SystemPrompt:              "raw system prompt must not be forwarded",
		IdentityPrompt:            "Act like the Platform team's coordinator.",
		PromptOperatingPrinciples: "Prefer dispatching specialists for implementation.",
		Model:                     DefaultEmployeeModel,
		Tools:                     model.JSON{},
		McpServers:                model.JSON{},
		Skills:                    model.JSON{},
		Integrations:              model.JSON{},
		Resources:                 model.JSON{},
		RuntimeConfig:             model.JSON{},
		Permissions:               model.JSON{},
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}

	def, err := Compile(context.Background(), CompileDeps{DB: db, Cfg: &config.Config{}}, &agent)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	body, err := json.Marshal(def)
	if err != nil {
		t.Fatalf("marshal definition: %v", err)
	}
	if strings.Contains(string(body), "system_prompt") {
		t.Fatalf("employee config must not include system_prompt: %s", string(body))
	}
	if def.PromptFragments.Identity.Title != "Your identity" {
		t.Fatalf("identity title = %q", def.PromptFragments.Identity.Title)
	}
	if !strings.Contains(def.PromptFragments.Identity.Content, "You are a "+orgName+" employee.") {
		t.Fatalf("identity content missing company sentence: %q", def.PromptFragments.Identity.Content)
	}
	if def.PromptFragments.Company.Title != "About the company" {
		t.Fatalf("company title = %q", def.PromptFragments.Company.Title)
	}
	if def.PromptFragments.OperatingPrinciples.Title != "" || def.PromptFragments.OperatingPrinciples.Content != "" {
		t.Fatalf("operating principles should be backend-owned inside identity prompt: %#v", def.PromptFragments.OperatingPrinciples)
	}
}

func TestCompile_SerializesSkillOptionalArraysAsEmptyArrays(t *testing.T) {
	db := connectCompileTestDB(t)
	org := model.Org{Name: "Skills-" + uuid.NewString()}
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
	desc := "Upload generated artifacts."
	skill := model.Skill{
		ID:          uuid.New(),
		OrgID:       &org.ID,
		Slug:        "asset-uploads",
		Name:        "Asset uploads",
		Description: &desc,
		SourceType:  model.SkillSourceInline,
		RepoRef:     "main",
		Status:      model.SkillStatusPublished,
		Bundle:      model.RawJSON(`{"description":"Upload generated artifacts.","content":"Use the upload endpoint.","files":{}}`),
	}
	if err := db.Create(&skill).Error; err != nil {
		t.Fatalf("create skill: %v", err)
	}
	if err := db.Create(&model.EmployeeSkill{EmployeeID: agent.ID, SkillID: skill.ID}).Error; err != nil {
		t.Fatalf("attach skill: %v", err)
	}

	def, err := Compile(context.Background(), CompileDeps{DB: db, Cfg: &config.Config{}}, &agent)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	body, err := json.Marshal(def)
	if err != nil {
		t.Fatalf("marshal definition: %v", err)
	}
	if strings.Contains(string(body), `"related_skills":null`) {
		t.Fatalf("related_skills serialized as null: %s", string(body))
	}
	if !strings.Contains(string(body), `"related_skills":[]`) {
		t.Fatalf("related_skills did not serialize as empty array: %s", string(body))
	}
	if strings.Contains(string(body), `"required_environment_variables":null`) {
		t.Fatalf("required_environment_variables serialized as null: %s", string(body))
	}
	if strings.Contains(string(body), `"required_credential_files":null`) {
		t.Fatalf("required_credential_files serialized as null: %s", string(body))
	}
}

func TestCompile_PreservesSkillRequiredEnvironmentVariables(t *testing.T) {
	db := connectCompileTestDB(t)
	org := model.Org{Name: "Skill env-" + uuid.NewString()}
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
	desc := "Upload generated artifacts."
	skill := model.Skill{
		ID:          uuid.New(),
		OrgID:       &org.ID,
		Slug:        "asset-uploads",
		Name:        "Asset uploads",
		Description: &desc,
		SourceType:  model.SkillSourceInline,
		RepoRef:     "main",
		Status:      model.SkillStatusPublished,
		Bundle: model.RawJSON(`{
			"description":"Upload generated artifacts.",
			"content":"Use the upload endpoint.",
			"files":{},
			"required_environment_variables":["UPLOAD_BEARER","HIVY_DRIVE_UPLOAD_URL","UPLOAD_BEARER"]
		}`),
	}
	if err := db.Create(&skill).Error; err != nil {
		t.Fatalf("create skill: %v", err)
	}
	if err := db.Create(&model.EmployeeSkill{EmployeeID: agent.ID, SkillID: skill.ID}).Error; err != nil {
		t.Fatalf("attach skill: %v", err)
	}

	def, err := Compile(context.Background(), CompileDeps{DB: db, Cfg: &config.Config{}}, &agent)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if len(def.Skills) != 1 {
		t.Fatalf("skills = %#v", def.Skills)
	}
	want := []string{EmployeeEnvDriveUploadURL, EmployeeEnvUploadBearer}
	if !reflect.DeepEqual(def.Skills[0].RequiredEnvironmentVariables, want) {
		t.Fatalf("required env vars = %#v, want %#v", def.Skills[0].RequiredEnvironmentVariables, want)
	}
}
