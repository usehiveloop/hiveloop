package sandbox

import (
	"encoding/base64"
	"os"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/crypto"
	"github.com/usehiveloop/hiveloop/internal/model"
)

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

func setupPusherTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = pusherTestDBURL
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Skipf("DB not available: %v", err)
	}
	sqlDB, _ := db.DB()
	if err := sqlDB.Ping(); err != nil {
		t.Skipf("DB ping failed: %v", err)
	}
	if err := model.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
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
