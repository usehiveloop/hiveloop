package sandbox

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	bridgepkg "github.com/usehiveloop/hiveloop/internal/bridge"
	"github.com/usehiveloop/hiveloop/internal/config"
	"github.com/usehiveloop/hiveloop/internal/model"
)

func TestPusherAgentConfig_HarnessFromAgent(t *testing.T) {
	for _, tc := range []struct {
		name   string
		stored string
		want   string
	}{
		{"empty defaults to open_code", "", string(bridgepkg.OpenCode)},
		{"claude is passed through", "claude", string(bridgepkg.Claude)},
		{"open_code is passed through", "open_code", string(bridgepkg.OpenCode)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := pushHarnessFixture(t, tc.stored)
			got, _ := body["harness"].(string)
			if got != tc.want {
				t.Errorf("harness on wire = %q, want %q", got, tc.want)
			}
		})
	}
}

func pushHarnessFixture(t *testing.T, storedHarness string) map[string]any {
	t.Helper()
	db := setupPusherTestDB(t)
	encKey := testPusherEncKey(t)

	org := model.Org{ID: uuid.New(), Name: "harness-org-" + uuid.New().String()[:8], Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", org.ID).Delete(&model.Org{}) })

	encrypted, err := encKey.EncryptString("sk-harness")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	cred := model.Credential{
		ID: uuid.New(), OrgID: org.ID,
		ProviderID: "openai", Label: "OpenAI",
		EncryptedKey: encrypted, WrappedDEK: []byte("test"),
		BaseURL: "https://api.openai.com", AuthScheme: "bearer",
	}
	if err := db.Create(&cred).Error; err != nil {
		t.Fatalf("create cred: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", cred.ID).Delete(&model.Credential{}) })

	agent := model.Agent{
		ID: uuid.New(), OrgID: &org.ID, CredentialID: &cred.ID,
		Name: "Harness Agent-" + uuid.New().String()[:8], Model: "gpt-4o",
		SystemPrompt: "test", Status: "active", Harness: storedHarness,
		Tools: model.JSON{}, McpServers: model.JSON{}, Skills: model.JSON{},
		Integrations: model.JSON{}, AgentConfig: model.JSON{}, Permissions: model.JSON{},
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", agent.ID).Delete(&model.Agent{}) })

	cfg := &config.Config{ProxyHost: "proxy.test", MCPBaseURL: "https://mcp.test"}
	pusher := NewPusher(db, nil, []byte("test-signing-key-for-harness"), cfg, nil)

	var captured []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/push/agents/") {
			body, _ := io.ReadAll(r.Body)
			captured = body
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	def := pusher.buildAgentDefinition(t.Context(), &agent, nil, &cred, "ptok_harness", uuid.New().String())
	client := bridgepkg.NewBridgeClient(srv.URL, "test-key")
	if err := client.UpsertAgent(context.Background(), agent.ID.String(), def); err != nil {
		t.Fatalf("UpsertAgent: %v", err)
	}

	var body map[string]any
	if err := json.Unmarshal(captured, &body); err != nil {
		t.Fatalf("decode body: %v\n--- raw ---\n%s", err, captured)
	}
	return body
}
