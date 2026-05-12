package employeeruntime

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/config"
	"github.com/usehiveloop/hiveloop/internal/hindsight"
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

func TestCompile_PopulatesMemoryContextFromHindsight(t *testing.T) {
	orgID := uuid.New()
	teamID := uuid.New()
	agent := model.Agent{
		ID:     uuid.New(),
		OrgID:  &orgID,
		TeamID: &teamID,
		Name:   "Aria",
		Model:  DefaultEmployeeModel,
	}
	fake := &fakeMemoryRecall{response: &hindsight.RecallResponse{
		Results: []any{
			map[string]any{
				"content":     "The Platform team requires integration tests for employee-runtime changes.",
				"source":      "manual",
				"memory_type": "technical_context",
				"tags":        []any{"company:" + orgID.String(), "team:" + teamID.String()},
			},
		},
	}}

	def, err := Compile(context.Background(), CompileDeps{Hindsight: fake, Cfg: &config.Config{}}, &agent)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if fake.bankID != "org-"+orgID.String() {
		t.Fatalf("bank id = %q", fake.bankID)
	}
	if len(fake.request.TagGroups) != 1 {
		t.Fatalf("expected strict org/team tag group, got %#v", fake.request.TagGroups)
	}
	memory, ok := def.Context["memory"].(MemoryContext)
	if !ok {
		t.Fatalf("memory context missing or wrong type: %#v", def.Context["memory"])
	}
	if len(memory.Entries) != 1 {
		t.Fatalf("memory entries = %#v", memory.Entries)
	}
	if memory.Entries[0].MemoryType != "technical_context" {
		t.Fatalf("memory type = %q", memory.Entries[0].MemoryType)
	}
}

func TestCompile_SucceedsWhenHindsightRecallFails(t *testing.T) {
	orgID := uuid.New()
	agent := model.Agent{ID: uuid.New(), OrgID: &orgID, Name: "Aria", Model: DefaultEmployeeModel}

	def, err := Compile(context.Background(), CompileDeps{Hindsight: &fakeMemoryRecall{err: errors.New("offline")}, Cfg: &config.Config{}}, &agent)
	if err != nil {
		t.Fatalf("compile should not fail when memory recall fails: %v", err)
	}
	memory, ok := def.Context["memory"].(MemoryContext)
	if !ok {
		t.Fatalf("memory context missing or wrong type: %#v", def.Context["memory"])
	}
	if len(memory.Entries) != 0 {
		t.Fatalf("expected empty memory entries, got %#v", memory.Entries)
	}
}

func TestControlPlaneOutboundChannels_EmitsEmployeeWebhookSpec(t *testing.T) {
	sandboxID := uuid.New()
	channels := ControlPlaneOutboundChannels(&config.Config{BridgeHost: "api.hiveloop.test"}, sandboxID)
	if len(channels) != 1 {
		t.Fatalf("channels = %#v", channels)
	}
	channel, ok := channels[0].(map[string]any)
	if !ok {
		t.Fatalf("channel has wrong type: %#v", channels[0])
	}
	if channel["url"] != "https://api.hiveloop.test/internal/webhooks/employee/"+sandboxID.String() {
		t.Fatalf("url = %q", channel["url"])
	}
	if channel["secret_env"] != "RUNTIME_SECRET" {
		t.Fatalf("secret env = %q", channel["secret_env"])
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

type fakeMemoryRecall struct {
	bankID   string
	request  *hindsight.RecallRequest
	response *hindsight.RecallResponse
	err      error
}

func (f *fakeMemoryRecall) Recall(_ context.Context, bankID string, req *hindsight.RecallRequest) (*hindsight.RecallResponse, error) {
	f.bankID = bankID
	f.request = req
	if f.err != nil {
		return nil, f.err
	}
	if f.response == nil {
		return &hindsight.RecallResponse{}, nil
	}
	return f.response, nil
}
