package employeeruntime

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/employeeprompts"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/specialists"
)

func TestBuildPromptSections_UsesTypedFields(t *testing.T) {
	orgID := uuid.New()
	description := managedEmployeeDescription
	agent := &model.Employee{
		ID:             uuid.New(),
		OrgID:          &orgID,
		Name:           "Aria",
		Description:    &description,
		SystemPrompt:   "raw system prompt must not be forwarded",
		IdentityPrompt: "Own engineering outcomes with evidence.",
	}

	fragments := buildPromptSections(context.Background(), nil, agent, description)

	if !strings.Contains(fragments.Identity.Content, managedEmployeeName) {
		t.Fatalf("identity fragment should include employee name: %#v", fragments.Identity)
	}
	if !strings.Contains(fragments.Identity.Content, description) {
		t.Fatalf("identity fragment should include description: %#v", fragments.Identity)
	}
	if strings.Contains(fragments.Identity.Content, agent.SystemPrompt) {
		t.Fatalf("typed fragments must not include raw system prompt")
	}
	if strings.Contains(fragments.Identity.Content, agent.IdentityPrompt) {
		t.Fatalf("identity fragment should not include user-editable identity prompt")
	}
}

func TestBuildPromptSections_UpgradesDefaultManagedIdentityPrompt(t *testing.T) {
	orgID := uuid.New()
	description := "Coordinates engineering work."
	category := "engineering"
	for _, tc := range []struct {
		name           string
		identityPrompt string
	}{
		{name: "blank", identityPrompt: ""},
		{name: "legacy default", identityPrompt: employeeprompts.LegacyEngineeringIdentityPromptV1},
		{name: "current default", identityPrompt: employeeprompts.EngineeringIdentityPrompt},
	} {
		t.Run(tc.name, func(t *testing.T) {
			agent := &model.Employee{
				ID:             uuid.New(),
				OrgID:          &orgID,
				Name:           "Higu",
				Description:    &description,
				Category:       &category,
				IdentityPrompt: tc.identityPrompt,
			}

			fragments := buildPromptSections(context.Background(), nil, agent, description)

			if !strings.Contains(fragments.Identity.Content, "Communication contract") {
				t.Fatalf("identity fragment missing current communication contract: %#v", fragments.Identity)
			}
			if !strings.Contains(fragments.Identity.Content, "Do not say \"specialist runtime\"") {
				t.Fatalf("identity fragment missing specialist runtime leakage guard: %#v", fragments.Identity)
			}
		})
	}
}

func TestBuildPromptSections_PreservesCustomIdentityPrompt(t *testing.T) {
	orgID := uuid.New()
	description := "Coordinates engineering work."
	custom := "Use the team's incident voice."
	agent := &model.Employee{
		ID:             uuid.New(),
		OrgID:          &orgID,
		Name:           "Higu",
		Description:    &description,
		IdentityPrompt: custom,
	}

	fragments := buildPromptSections(context.Background(), nil, agent, description)

	if strings.Contains(fragments.Identity.Content, custom) {
		t.Fatalf("identity fragment should ignore custom identity prompt: %#v", fragments.Identity)
	}
	if !strings.Contains(fragments.Identity.Content, "Communication contract") {
		t.Fatalf("identity fragment should use backend-owned default: %#v", fragments.Identity)
	}
}

func TestBuildEmployeeSystemPrompt_CompilesAllRuntimePromptSegments(t *testing.T) {
	fragments := PromptSections{
		Identity: PromptSection{
			Title:   "Your identity",
			Content: "You are the managed employee.",
		},
		Company: PromptSection{
			Title:   "About the company",
			Content: "Company name: ExampleCo",
		},
	}

	prompt := buildEmployeeSystemPrompt(fragments)
	cacheable := requireCacheableSegments(t, prompt)
	dynamic := requireDynamicSegments(t, prompt)

	if len(cacheable) != 3 {
		t.Fatalf("cacheable segment count = %d", len(cacheable))
	}
	base := requireStaticPromptSegment(t, cacheable[0])
	if !strings.Contains(requirePromptString(t, base.Content), "Your job is to drive real team work forward.") {
		t.Fatalf("base prompt missing employee contract: %#v", base)
	}
	if got := requireDynamicContextSegmentType(t, dynamic[0]); got != "dynamic_context" {
		t.Fatalf("first dynamic segment = %q", got)
	}
	if got := requireMemorySegmentType(t, dynamic[1]); got != "memory_context" {
		t.Fatalf("second dynamic segment = %q", got)
	}
	if got := requireListSegment3Type(t, dynamic[2]); got != "skill_catalog" {
		t.Fatalf("third dynamic segment = %q", got)
	}
	if len(dynamic) != 4 {
		t.Fatalf("dynamic segment count = %d", len(dynamic))
	}
	if got := requireListSegment4Type(t, dynamic[3]); got != "mcp_tools" {
		t.Fatalf("fourth dynamic segment = %q", got)
	}
}

func TestBuildAvailableSpecialistsSection_ListsAttachedSpecialists(t *testing.T) {
	catalog, err := specialists.NewCatalog([]specialists.Definition{
		{
			Slug:           "software-engineering-specialist",
			Name:           "Software Engineering",
			Description:    "Build and verify code changes.",
			SpecialistType: "engineering",
			Version:        1,
			DefaultModel:   "deepseek-v4-pro",
			SystemPrompt:   "Do engineering work.",
		},
		{
			Slug:           "business-research-specialist",
			Name:           "Business Research",
			Description:    "Research business questions.",
			SpecialistType: "research",
			Version:        1,
			DefaultModel:   "deepseek-v4-pro",
			SystemPrompt:   "Do research work.",
		},
	})
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	agent := &model.Employee{
		AttachedSpecialists: []string{"software-engineering-specialist"},
	}

	section := buildAvailableSpecialistsSection(agent, catalog)

	if section.Title != "Available specialists" {
		t.Fatalf("section title = %q", section.Title)
	}
	if !strings.Contains(section.Content, "software-engineering-specialist: Build and verify code changes.") {
		t.Fatalf("attached specialist missing from section: %q", section.Content)
	}
	if strings.Contains(section.Content, "business-research-specialist") {
		t.Fatalf("unattached specialist leaked into section: %q", section.Content)
	}
	staleDelegationTerm := "sub" + "agent"
	if strings.Contains(section.Content, staleDelegationTerm) {
		t.Fatalf("section must use product terminology only: %q", section.Content)
	}
}

func TestDefaultRuntimeSurfaceDoesNotExposeGenericDelegation(t *testing.T) {
	for _, tool := range defaultTools() {
		if got, _ := tool["type"].(string); got == "builtin.delegate" || got == "builtin.check_delegated_status" {
			t.Fatalf("defaultTools exposed generic delegation tool %q", got)
		}
	}
	for key := range defaultLimits() {
		if strings.Contains(key, "sub"+"agent") {
			t.Fatalf("defaultLimits exposed stale key %q", key)
		}
	}
}

func requireCacheableSegments(t *testing.T, prompt SystemPromptConfig) []SystemPromptSegment {
	t.Helper()
	if prompt.CacheableSegments == nil {
		t.Fatal("cacheable segments is nil")
	}
	return *prompt.CacheableSegments
}

func requireDynamicSegments(t *testing.T, prompt SystemPromptConfig) []SystemPromptSegment {
	t.Helper()
	if prompt.DynamicSegments == nil {
		t.Fatal("dynamic segments is nil")
	}
	return *prompt.DynamicSegments
}

func requireStaticPromptSegment(t *testing.T, segment SystemPromptSegment) StaticPromptSegment {
	t.Helper()
	staticSegment, err := segment.AsSystemPromptSegment0()
	if err != nil {
		t.Fatalf("decode static prompt segment: %v", err)
	}
	if staticSegment.Type != "static_text" {
		t.Fatalf("static segment type = %q", staticSegment.Type)
	}
	return staticSegment.Config
}

func requireDynamicContextSegmentType(t *testing.T, segment SystemPromptSegment) string {
	t.Helper()
	dynamicSegment, err := segment.AsSystemPromptSegment1()
	if err != nil {
		t.Fatalf("decode dynamic context segment: %v", err)
	}
	return string(dynamicSegment.Type)
}

func requireMemorySegmentType(t *testing.T, segment SystemPromptSegment) string {
	t.Helper()
	memorySegment, err := segment.AsSystemPromptSegment2()
	if err != nil {
		t.Fatalf("decode memory segment: %v", err)
	}
	return string(memorySegment.Type)
}

func requireListSegment3Type(t *testing.T, segment SystemPromptSegment) string {
	t.Helper()
	listSegment, err := segment.AsSystemPromptSegment3()
	if err != nil {
		t.Fatalf("decode list segment: %v", err)
	}
	return string(listSegment.Type)
}

func requireListSegment4Type(t *testing.T, segment SystemPromptSegment) string {
	t.Helper()
	listSegment, err := segment.AsSystemPromptSegment4()
	if err != nil {
		t.Fatalf("decode mcp tools segment: %v", err)
	}
	return string(listSegment.Type)
}

func requirePromptString(t *testing.T, value *string) string {
	t.Helper()
	if value == nil {
		t.Fatal("prompt string is nil")
	}
	return *value
}

func TestBuildEmployeeMCPServer_DisabledWithoutRuntimeToken(t *testing.T) {
	orgID := uuid.New()
	agent := &model.Employee{ID: uuid.New(), OrgID: &orgID}

	if got := buildEmployeeMCPServer(context.Background(), CompileDeps{}, agent); got != nil {
		t.Fatalf("expected no MCP server without DB/config/token, got %#v", got)
	}
}

func TestProxyTokensCarryRuntimeModeMetadata(t *testing.T) {
	db := connectCompileTestDB(t)
	agent := createCompileTokenAgent(t, db)
	deps := CompileDeps{
		DB:         db,
		SigningKey: []byte("test-signing-key-32-bytes-long!!"),
	}

	employeeToken, err := MintProxyToken(context.Background(), deps, &agent, uuid.Nil)
	if err != nil {
		t.Fatalf("mint employee token: %v", err)
	}
	if _, err := PrepareSpecialistStartup(context.Background(), deps, &agent, "software-engineering-specialist"); err != nil {
		t.Fatalf("prepare specialist startup: %v", err)
	}

	var employee model.Token
	if err := db.First(&employee, "jti = ?", employeeToken.JTI).Error; err != nil {
		t.Fatalf("load employee token: %v", err)
	}
	if employee.Meta[model.TokenMetaRuntimeMode] != model.TokenRuntimeModeEmployee {
		t.Fatalf("employee runtime_mode = %#v", employee.Meta[model.TokenMetaRuntimeMode])
	}

	var specialist model.Token
	if err := db.Where("meta->>? = ?", model.TokenMetaRuntimeMode, model.TokenRuntimeModeSpecialist).First(&specialist).Error; err != nil {
		t.Fatalf("load specialist token: %v", err)
	}
	if specialist.Meta[model.TokenMetaSpecialistSlug] != "software-engineering-specialist" {
		t.Fatalf("specialist slug = %#v", specialist.Meta[model.TokenMetaSpecialistSlug])
	}
}

func TestBuildHivyMCPServerSelectsRuntimeModeToken(t *testing.T) {
	db := connectCompileTestDB(t)
	agent := createCompileTokenAgent(t, db)
	deps := CompileDeps{
		DB:         db,
		Cfg:        &config.Config{MCPBaseURL: "https://mcp.hivy.test"},
		SigningKey: []byte("test-signing-key-32-bytes-long!!"),
	}

	employeeToken, err := MintProxyToken(context.Background(), deps, &agent, uuid.Nil)
	if err != nil {
		t.Fatalf("mint employee token: %v", err)
	}
	if _, err := PrepareSpecialistStartup(context.Background(), deps, &agent, "software-engineering-specialist"); err != nil {
		t.Fatalf("prepare specialist startup: %v", err)
	}
	var specialistToken model.Token
	if err := db.Where("meta->>? = ?", model.TokenMetaRuntimeMode, model.TokenRuntimeModeSpecialist).First(&specialistToken).Error; err != nil {
		t.Fatalf("load specialist token: %v", err)
	}

	employeeMCP := buildHivyMCPServer(context.Background(), deps, &agent, model.TokenRuntimeModeEmployee, "").(map[string]any)
	if got := employeeMCP["url"].(string); !strings.HasSuffix(got, "/"+employeeToken.JTI) {
		t.Fatalf("employee mcp url = %q, want suffix %q", got, employeeToken.JTI)
	}

	specialistMCP := buildHivyMCPServer(context.Background(), deps, &agent, model.TokenRuntimeModeSpecialist, "software-engineering-specialist").(map[string]any)
	if got := specialistMCP["url"].(string); !strings.HasSuffix(got, "/"+specialistToken.JTI) {
		t.Fatalf("specialist mcp url = %q, want suffix %q", got, specialistToken.JTI)
	}
}

func TestUpsertHivyMCPServer_ReplacesExistingHivyServer(t *testing.T) {
	servers := []any{
		map[string]any{"name": "hivy", "url": "old"},
		map[string]any{"name": "linear", "url": "keep"},
	}
	got := upsertHivyMCPServer(servers, map[string]any{"name": "hivy", "url": "new"})

	if len(got) != 2 {
		t.Fatalf("server count = %d, want 2", len(got))
	}
	if got[1].(map[string]any)["url"] != "new" {
		t.Fatalf("hivy server was not replaced: %#v", got)
	}
}

func createCompileTokenAgent(t *testing.T, db *gorm.DB) model.Employee {
	t.Helper()
	org := model.Org{Name: "compile-token-org-" + uuid.NewString(), Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	cred := model.Credential{
		OrgID:        &org.ID,
		Label:        "compile-token",
		BaseURL:      "https://proxy.test",
		AuthScheme:   "bearer",
		EncryptedKey: []byte("enc"),
		WrappedDEK:   []byte("dek"),
		ProviderID:   "openrouter",
	}
	if err := db.Create(&cred).Error; err != nil {
		t.Fatalf("create credential: %v", err)
	}
	agent := model.Employee{
		OrgID:        &org.ID,
		CredentialID: &cred.ID,
		Name:         "Hivy",
		Model:        DefaultEmployeeModel,
		Status:       "active",
		Tools:        model.JSON{},
		McpServers:   model.JSON{},
		Skills:       model.JSON{},
		Resources:    model.JSON{},
		Permissions:  model.JSON{},
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create employee: %v", err)
	}
	t.Cleanup(func() {
		db.Where("org_id = ?", org.ID).Delete(&model.Token{})
		db.Where("id = ?", agent.ID).Delete(&model.Employee{})
		db.Where("id = ?", cred.ID).Delete(&model.Credential{})
		db.Where("id = ?", org.ID).Delete(&model.Org{})
	})
	return agent
}
