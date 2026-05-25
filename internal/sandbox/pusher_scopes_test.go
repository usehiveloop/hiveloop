package sandbox

import (
	"encoding/base64"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/testdb"
)

const pusherTestDBURL = testdb.DefaultDatabaseURL

func TestBuildScopesFromIntegrations(t *testing.T) {
	scopes := buildScopesFromIntegrations(model.JSON{})
	if scopes != nil {
		t.Errorf("empty integrations: expected nil, got %v", scopes)
	}

	integrations := model.JSON{
		"conn-github-123": map[string]any{
			"actions": []any{"repos.list", "issues.list", "pulls.create"},
		},
	}
	scopes = buildScopesFromIntegrations(integrations)
	if len(scopes) != 1 {
		t.Fatalf("expected 1 scope, got %d", len(scopes))
	}
	scopeActions, ok := scopes[0]["actions"].([]string)
	if !ok {
		t.Fatal("scope actions should be []string")
	}
	if len(scopeActions) != 3 {
		t.Errorf("expected 3 actions, got %d", len(scopeActions))
	}

	integrations = model.JSON{
		"conn-github-123": map[string]any{
			"actions": []any{"repos.list"},
		},
		"conn-slack-456": map[string]any{
			"actions": []any{"channels.list", "messages.send"},
		},
	}
	scopes = buildScopesFromIntegrations(integrations)
	if len(scopes) != 2 {
		t.Errorf("expected 2 scopes, got %d", len(scopes))
	}

	integrations = model.JSON{
		"conn-github-123": map[string]any{
			"other_key": "value",
		},
	}
	scopes = buildScopesFromIntegrations(integrations)
	if len(scopes) != 0 {
		t.Errorf("connection with no actions: expected 0 scopes, got %d", len(scopes))
	}
}

func TestMergeAgentIntegrationsForAccess_InheritsEmployeeAndAllowsSpecialistOverride(t *testing.T) {
	employee := &model.Employee{Integrations: model.JSON{
		"employee-conn": map[string]any{"actions": []any{"issues.read"}},
		"shared-conn":   map[string]any{"actions": []any{"parent.action"}},
	}}
	specialist := &model.Employee{Integrations: model.JSON{
		"shared-conn":     map[string]any{"actions": []any{"child.action"}},
		"specialist-conn": map[string]any{"actions": []any{"deploy.create"}},
	}}

	merged := mergeAgentIntegrationsForAccess(specialist, employee)
	scopes := buildScopesFromIntegrations(merged)
	if len(scopes) != 3 {
		t.Fatalf("scope count = %d, want 3: %#v", len(scopes), scopes)
	}
	actionsByConnection := map[string][]string{}
	for _, scope := range scopes {
		connectionID, _ := scope["connection_id"].(string)
		actions, _ := scope["actions"].([]string)
		actionsByConnection[connectionID] = actions
	}
	if got := actionsByConnection["employee-conn"]; len(got) != 1 || got[0] != "issues.read" {
		t.Fatalf("employee-conn actions = %#v, want issues.read", got)
	}
	if got := actionsByConnection["shared-conn"]; len(got) != 1 || got[0] != "child.action" {
		t.Fatalf("shared-conn actions = %#v, want child override", got)
	}
	if got := actionsByConnection["specialist-conn"]; len(got) != 1 || got[0] != "deploy.create" {
		t.Fatalf("specialist-conn actions = %#v, want deploy.create", got)
	}
}

func setupPusherTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := testdb.DatabaseURL("DATABASE_URL", "HIVY_DATABASE_URL", "TEST_DATABASE_URL")
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	sqlDB, _ := db.DB()
	testdb.ApplyMigrations(t, db)
	t.Cleanup(func() { sqlDB.Close() })
	return db
}

func testPusherEncKey(t *testing.T) *crypto.SymmetricKey {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 42)
	}
	sk, err := crypto.NewSymmetricKey(base64.StdEncoding.EncodeToString(key))
	if err != nil {
		t.Fatal(err)
	}
	return sk
}

func assertEqual(t *testing.T, field, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s: got %q, want %q", field, got, want)
	}
}

func assertContains(t *testing.T, field, haystack, needle string) {
	t.Helper()
	if len(haystack) == 0 {
		t.Errorf("%s: empty string", field)
		return
	}
	for i := 0; i <= len(haystack)-len(needle); i++ {
		if haystack[i:i+len(needle)] == needle {
			return
		}
	}
	t.Errorf("%s: does not contain %q (len=%d)", field, needle, len(haystack))
}

func assertSliceContains(t *testing.T, field string, slice []string, want string) {
	t.Helper()
	for _, s := range slice {
		if s == want {
			return
		}
	}
	t.Errorf("%s: %v does not contain %q", field, slice, want)
}
