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

func TestCompile_EmitsControlPlaneSystemPromptWithoutRawAgentPrompt(t *testing.T) {
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
	if !strings.Contains(string(body), `"system_prompt"`) {
		t.Fatalf("employee config must include system_prompt: %s", string(body))
	}
	if strings.Contains(string(body), "raw system prompt must not be forwarded") {
		t.Fatalf("employee config forwarded raw agent system prompt: %s", string(body))
	}
	if strings.Contains(string(body), `"prompt_fragments"`) {
		t.Fatalf("employee config must not include legacy prompt_fragments: %s", string(body))
	}
	assertRuntimeSystemPromptPayloadShape(t, body)
	cacheable := requireCacheableSegments(t, def.SystemPrompt)
	dynamic := requireDynamicSegments(t, def.SystemPrompt)
	if len(cacheable) < 3 {
		t.Fatalf("expected base, identity, and company prompt segments: %#v", cacheable)
	}
	base := requireStaticPromptSegment(t, cacheable[0])
	if !strings.Contains(requirePromptString(t, base.Content), "Your job is to drive real team work forward.") {
		t.Fatalf("base system prompt missing from first cacheable segment: %#v", cacheable[0])
	}
	identity := requireStaticPromptSegment(t, cacheable[1])
	if requirePromptString(t, identity.Title) != "Your identity" {
		t.Fatalf("identity title = %q", requirePromptString(t, identity.Title))
	}
	if !strings.Contains(requirePromptString(t, identity.Content), "You are a "+orgName+" employee.") {
		t.Fatalf("identity content missing company sentence: %q", requirePromptString(t, identity.Content))
	}
	company := requireStaticPromptSegment(t, cacheable[2])
	if requirePromptString(t, company.Title) != "About the company" {
		t.Fatalf("company title = %q", requirePromptString(t, company.Title))
	}
	if len(dynamic) != 5 {
		t.Fatalf("dynamic segment count = %d", len(dynamic))
	}
	if got := requireDynamicContextSegmentType(t, dynamic[0]); got != "dynamic_context" {
		t.Fatalf("first dynamic segment type = %q", got)
	}
}

func assertRuntimeSystemPromptPayloadShape(t *testing.T, body []byte) {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("decode compiled config JSON: %v", err)
	}
	agent, _ := doc["agent"].(map[string]any)
	if _, ok := agent["system_prompt"]; ok {
		t.Fatalf("agent metadata must not contain raw system_prompt: %s", string(body))
	}
	systemPrompt, ok := doc["system_prompt"].(map[string]any)
	if !ok {
		t.Fatalf("system_prompt missing or wrong shape: %s", string(body))
	}
	cacheable, _ := systemPrompt["cacheable_segments"].([]any)
	if len(cacheable) == 0 {
		t.Fatalf("cacheable_segments missing: %s", string(body))
	}
	firstCacheable, _ := cacheable[0].(map[string]any)
	if firstCacheable["type"] != "static_text" {
		t.Fatalf("first cacheable segment type = %#v", firstCacheable["type"])
	}
	dynamic, _ := systemPrompt["dynamic_segments"].([]any)
	if len(dynamic) != 5 {
		t.Fatalf("dynamic_segments count = %d", len(dynamic))
	}
	firstDynamic, _ := dynamic[0].(map[string]any)
	if firstDynamic["type"] != "dynamic_context" {
		t.Fatalf("first dynamic segment type = %#v", firstDynamic["type"])
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
