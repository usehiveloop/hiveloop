package employeeruntime

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
)

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
