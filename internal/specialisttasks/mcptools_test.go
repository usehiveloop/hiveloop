package specialisttasks

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/employeeruntime"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
	"github.com/usehivy/hivy/internal/specialists"
	"github.com/usehivy/hivy/internal/testdb"
)

const specialistTasksTestDBURL = "postgres://hivy:localdev@localhost:15432/hivy_test?sslmode=disable" // #nosec G101 -- test fixture, not a real secret

func TestSpecialistListToolReturnsAttachedSpecialists(t *testing.T) {
	db := connectSpecialistTasksTestDB(t)
	catalog := specialistTestCatalog(t)
	org, employee, token := createSpecialistToolScope(t, db)
	t.Cleanup(func() { db.Where("id = ?", org.ID).Delete(&model.Org{}) })

	service := NewService(db, &sandbox.Orchestrator{}, employeeruntimeCompileDepsForTest(), catalog)
	server := mcp.NewServer(&mcp.Implementation{Name: "specialist-test", Version: "v1"}, nil)
	NewToolsFunc(service)(server, &token)
	session, cleanup := connectSpecialistMCPTestSession(t, server)
	defer cleanup()

	tools, err := session.ListTools(context.Background(), &mcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	names := map[string]bool{}
	for _, tool := range tools.Tools {
		names[tool.Name] = true
	}
	for _, want := range []string{"specialist_list", "specialist_launch_task", "specialist_task_status", "specialist_task_send_message", "specialist_task_terminate"} {
		if !names[want] {
			t.Fatalf("tool %q missing from %#v", want, names)
		}
	}

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "specialist_list", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("call specialist_list: %v", err)
	}
	if result.IsError {
		t.Fatalf("specialist_list returned error: %s", specialistToolText(t, result))
	}
	var payload AvailableSpecialistsResponse
	if err := json.Unmarshal([]byte(specialistToolText(t, result)), &payload); err != nil {
		t.Fatalf("decode specialist_list: %v", err)
	}
	if payload.Count != 1 || len(payload.Specialists) != 1 {
		t.Fatalf("specialist count = %d/%d, payload=%#v", payload.Count, len(payload.Specialists), payload)
	}
	if got := payload.Specialists[0].Slug; got != employee.AttachedSpecialists[0] {
		t.Fatalf("slug = %q, want %q", got, employee.AttachedSpecialists[0])
	}
}

func TestSpecialistToolsHiddenForNonEmployeeRuntimeTokens(t *testing.T) {
	db := connectSpecialistTasksTestDB(t)
	catalog := specialistTestCatalog(t)
	org, _, token := createSpecialistToolScope(t, db)
	t.Cleanup(func() { db.Where("id = ?", org.ID).Delete(&model.Org{}) })

	for _, tc := range []struct {
		name string
		meta model.JSON
	}{
		{
			name: "specialist runtime",
			meta: model.JSON{
				model.TokenMetaType:        model.TokenTypeEmployeeProxy,
				model.TokenMetaRuntimeMode: model.TokenRuntimeModeSpecialist,
			},
		},
		{
			name: "missing runtime mode",
			meta: model.JSON{
				model.TokenMetaType: model.TokenTypeEmployeeProxy,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scopedToken := token
			scopedToken.Meta = tc.meta
			service := NewService(db, &sandbox.Orchestrator{}, employeeruntimeCompileDepsForTest(), catalog)
			server := mcp.NewServer(&mcp.Implementation{Name: "specialist-test", Version: "v1"}, nil)
			NewToolsFunc(service)(server, &scopedToken)
			session, cleanup := connectSpecialistMCPTestSession(t, server)
			defer cleanup()

			tools, err := session.ListTools(context.Background(), &mcp.ListToolsParams{})
			if err != nil {
				t.Fatalf("list tools: %v", err)
			}
			if len(tools.Tools) != 0 {
				t.Fatalf("specialist tools should be hidden, got %#v", tools.Tools)
			}
		})
	}
}

func connectSpecialistTasksTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = specialistTasksTestDBURL
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(3)
	sqlDB.SetMaxIdleConns(1)
	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("postgres not reachable: %v", err)
	}
	testdb.ApplyMigrations(t, db)
	t.Cleanup(func() { sqlDB.Close() })
	return db
}

func specialistTestCatalog(t *testing.T) *specialists.Catalog {
	t.Helper()
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
	return catalog
}

func createSpecialistToolScope(t *testing.T, db *gorm.DB) (model.Org, model.Employee, model.Token) {
	t.Helper()
	org := model.Org{ID: uuid.New(), Name: "specialist-tools-" + uuid.NewString()}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	employee := model.Employee{
		ID:                  uuid.New(),
		OrgID:               &org.ID,
		Model:               "test-model",
		AttachedSpecialists: pq.StringArray{"software-engineering-specialist"},
	}
	if err := db.Create(&employee).Error; err != nil {
		t.Fatalf("create employee: %v", err)
	}
	credential := model.Credential{
		ID:           uuid.New(),
		OrgID:        &org.ID,
		Label:        "test",
		BaseURL:      "http://localhost",
		AuthScheme:   "bearer",
		EncryptedKey: []byte("encrypted"),
		WrappedDEK:   []byte("wrapped"),
	}
	if err := db.Create(&credential).Error; err != nil {
		t.Fatalf("create credential: %v", err)
	}
	token := model.Token{
		ID:           uuid.New(),
		OrgID:        org.ID,
		CredentialID: credential.ID,
		JTI:          uuid.NewString(),
		ExpiresAt:    time.Now().Add(time.Hour),
		Scopes:       model.JSON{},
		Meta: model.JSON{
			model.TokenMetaType:        model.TokenTypeEmployeeProxy,
			model.TokenMetaRuntimeMode: model.TokenRuntimeModeEmployee,
			model.TokenMetaEmployeeID:  employee.ID.String(),
		},
	}
	if err := db.Create(&token).Error; err != nil {
		t.Fatalf("create token: %v", err)
	}
	return org, employee, token
}

func connectSpecialistMCPTestSession(t *testing.T, server *mcp.Server) (*mcp.ClientSession, func()) {
	t.Helper()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(context.Background(), serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "specialist-test-client", Version: "v1"}, nil)
	clientSession, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	return clientSession, func() {
		_ = clientSession.Close()
		_ = serverSession.Close()
	}
}

func specialistToolText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if result == nil || len(result.Content) == 0 {
		t.Fatal("tool result has no content")
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("tool result content type = %T", result.Content[0])
	}
	return text.Text
}

func employeeruntimeCompileDepsForTest() employeeruntime.CompileDeps {
	return employeeruntime.CompileDeps{}
}
