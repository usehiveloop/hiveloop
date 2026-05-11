package employeeruntime

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/config"
	"github.com/usehiveloop/hiveloop/internal/model"
)

const compileTestDBURL = "postgres://hiveloop:localdev@localhost:5433/hiveloop_test?sslmode=disable" // #nosec G101 -- test fixture, not a real secret

func TestBuildPromptFragments_UsesTypedFields(t *testing.T) {
	orgID := uuid.New()
	description := "Coordinates engineering work."
	agent := &model.Agent{
		ID:             uuid.New(),
		OrgID:          &orgID,
		Name:           "Aria",
		Description:    &description,
		SystemPrompt:   "raw system prompt must not be forwarded",
		IdentityPrompt: "Own engineering outcomes with evidence.",
	}

	fragments := buildPromptFragments(context.Background(), nil, agent, description)

	if !strings.Contains(fragments.Identity.Content, "Aria") {
		t.Fatalf("identity fragment should include employee name: %#v", fragments.Identity)
	}
	if !strings.Contains(fragments.Identity.Content, description) {
		t.Fatalf("identity fragment should include description: %#v", fragments.Identity)
	}
	if strings.Contains(fragments.Identity.Content, agent.SystemPrompt) {
		t.Fatalf("typed fragments must not include raw system prompt")
	}
	if !strings.Contains(fragments.Identity.Content, agent.IdentityPrompt) {
		t.Fatalf("identity fragment should include employee identity prompt")
	}
}

func TestBuildEmployeeMCPServer_DisabledWithoutRuntimeToken(t *testing.T) {
	orgID := uuid.New()
	agent := &model.Agent{ID: uuid.New(), OrgID: &orgID}

	if got := buildEmployeeMCPServer(context.Background(), CompileDeps{}, agent); got != nil {
		t.Fatalf("expected no MCP server without DB/config/token, got %#v", got)
	}
}

func TestUpsertHiveloopMCPServer_ReplacesExistingHiveloopServer(t *testing.T) {
	servers := []any{
		map[string]any{"name": "hiveloop", "url": "old"},
		map[string]any{"name": "linear", "url": "keep"},
	}
	got := upsertHiveloopMCPServer(servers, map[string]any{"name": "hiveloop", "url": "new"})

	if len(got) != 2 {
		t.Fatalf("server count = %d, want 2", len(got))
	}
	if got[1].(map[string]any)["url"] != "new" {
		t.Fatalf("hiveloop server was not replaced: %#v", got)
	}
}

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
	team := model.Team{
		OrgID:      org.ID,
		Name:       "Platform",
		PromptTeam: "The Platform team owns reliability, deployment, and developer experience.",
	}
	if err := db.Create(&team).Error; err != nil {
		t.Fatalf("create team: %v", err)
	}
	description := "Coordinates platform engineering work."
	category := "engineering"
	agent := model.Agent{
		ID:                        uuid.New(),
		OrgID:                     &org.ID,
		Name:                      "Aria",
		Description:               &description,
		Category:                  &category,
		TeamID:                    &team.ID,
		Team:                      team.Name,
		SystemPrompt:              "raw system prompt must not be forwarded",
		IdentityPrompt:            "Act like the Platform team's coordinator.",
		PromptOperatingPrinciples: "Prefer dispatching cloud agents for implementation.",
		Model:                     DefaultEmployeeModel,
		Tools:                     model.JSON{},
		McpServers:                model.JSON{},
		Skills:                    model.JSON{},
		Integrations:              model.JSON{},
		Resources:                 model.JSON{},
		AgentConfig:               model.JSON{},
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
	if !strings.Contains(def.PromptFragments.Identity.Content, "You are a "+orgName+" employee working on the Platform team.") {
		t.Fatalf("identity content missing company/team sentence: %q", def.PromptFragments.Identity.Content)
	}
	if def.PromptFragments.Company.Title != "About the company" {
		t.Fatalf("company title = %q", def.PromptFragments.Company.Title)
	}
	if def.PromptFragments.Team.Title != "About your team" {
		t.Fatalf("team title = %q", def.PromptFragments.Team.Title)
	}
	if def.PromptFragments.OperatingPrinciples.Title != "Operating principles" {
		t.Fatalf("operating principles title = %q", def.PromptFragments.OperatingPrinciples.Title)
	}
	if !strings.Contains(def.PromptFragments.OperatingPrinciples.Content, "dispatching cloud agents") {
		t.Fatalf("operating principles not sourced from employee: %q", def.PromptFragments.OperatingPrinciples.Content)
	}
}

func connectCompileTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = compileTestDBURL
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("cannot connect to Postgres: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(3)
	sqlDB.SetMaxIdleConns(1)
	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("Postgres not reachable: %v", err)
	}
	if err := model.AutoMigrate(db); err != nil {
		t.Fatalf("migration failed: %v", err)
	}
	t.Cleanup(func() {
		sqlDB.Close()
	})
	return db
}
