package tasks

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/employeeruntime"
	"github.com/usehivy/hivy/internal/mcp/catalog"
	"github.com/usehivy/hivy/internal/model"
)

func TestEmployeeTriggerCompileMessage_UsesCatalogRefsAndOmitsRawPayload(t *testing.T) {
	triggerID := uuid.New()
	trigger := model.EmployeeTrigger{
		ID:           triggerID,
		TriggerType:  "webhook",
		Instructions: "Summarize the issue and decide whether to create a task.",
	}
	payload := EmployeeTriggerDispatchPayload{
		Provider:    "github",
		EventType:   "issues",
		EventAction: "opened",
		DeliveryID:  "delivery-1",
	}
	raw := map[string]any{
		"repository": map[string]any{
			"name":           "hivy",
			"full_name":      "usehivy/hivy",
			"default_branch": "main",
			"owner": map[string]any{
				"login": "usehivy",
			},
		},
		"issue": map[string]any{
			"number":   float64(42),
			"title":    "Queue trigger events",
			"body":     "Please do not dump this whole body if it is not requested.",
			"html_url": "https://github.com/usehivy/hivy/issues/42",
			"user": map[string]any{
				"login": "bahdcoder",
			},
		},
		"sender": map[string]any{
			"login": "bahdcoder",
		},
		"installation": map[string]any{
			"token": "must-not-appear",
		},
	}

	compiled := (&EmployeeTriggerDispatchHandler{catalog: catalog.Global()}).
		compileMessage(payload, trigger, raw)

	if compiled.ResourceKey != "github/usehivy/hivy/issue/42" {
		t.Fatalf("resource key = %q", compiled.ResourceKey)
	}
	if compiled.ConversationID != stableTriggerConversationID(triggerID, compiled.ResourceKey) {
		t.Fatalf("conversation id = %q", compiled.ConversationID)
	}
	for _, want := range []string{
		"Instructions:",
		"Summarize the issue",
		"provider: github",
		"event_key: issues.opened",
		"issue_number: 42",
		"title: Queue trigger events",
	} {
		if !strings.Contains(compiled.Text, want) {
			t.Fatalf("compiled text missing %q:\n%s", want, compiled.Text)
		}
	}
	if strings.Contains(compiled.Text, "must-not-appear") {
		t.Fatalf("compiled text leaked raw payload: %s", compiled.Text)
	}
	if compiled.Raw["source"] != "trigger" {
		t.Fatalf("raw source = %#v", compiled.Raw["source"])
	}
	refs, ok := compiled.Raw["refs"].(map[string]string)
	if !ok {
		t.Fatalf("raw refs type = %T", compiled.Raw["refs"])
	}
	if refs["issue_number"] != "42" {
		t.Fatalf("raw refs issue_number = %q", refs["issue_number"])
	}
}

func TestEmployeeTriggerCompileMessage_HTTPIncludesSubmittedBody(t *testing.T) {
	trigger := model.EmployeeTrigger{
		ID:           uuid.New(),
		TriggerType:  "http",
		Instructions: "Handle this external alert.",
	}
	raw := map[string]any{
		"alert": "deploy failed",
		"count": float64(2),
	}

	compiled := (&EmployeeTriggerDispatchHandler{catalog: catalog.Global()}).
		compileMessage(EmployeeTriggerDispatchPayload{DeliveryID: "http-1"}, trigger, raw)

	if compiled.ResourceKey != "http-1" {
		t.Fatalf("resource key = %q", compiled.ResourceKey)
	}
	if !strings.Contains(compiled.Text, "HTTP payload:") || !strings.Contains(compiled.Text, `"alert": "deploy failed"`) {
		t.Fatalf("compiled HTTP text missing payload:\n%s", compiled.Text)
	}
}

func TestTriggerConditionsMatch(t *testing.T) {
	conditions, err := json.Marshal(model.TriggerMatch{
		Mode: "all",
		Conditions: []model.TriggerCondition{{
			Path:     "repository.full_name",
			Operator: "equals",
			Value:    "usehivy/hivy",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	trigger := model.EmployeeTrigger{Conditions: model.RawJSON(conditions)}

	ok, _ := triggerConditionsMatch(trigger, map[string]any{
		"repository": map[string]any{"full_name": "usehivy/hivy"},
	})
	if !ok {
		t.Fatal("expected matching payload to pass")
	}
	ok, _ = triggerConditionsMatch(trigger, map[string]any{
		"repository": map[string]any{"full_name": "other/repo"},
	})
	if ok {
		t.Fatal("expected non-matching payload to fail")
	}
}

func TestEmployeeTriggerDispatchSyncRuntime_PushesFullRuntimeConfig(t *testing.T) {
	db := openTasksMemoryTestDB(t)
	encKey := testTasksEncKey(t)
	orgID := uuid.New()
	agentID := uuid.New()
	sandboxID := uuid.New()
	if err := db.Create(&model.Org{ID: orgID, Name: "trigger-sync-" + uuid.NewString()[:8], Active: true}).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	cred := model.Credential{
		OrgID:        &orgID,
		Label:        "trigger-sync",
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
		ID:           agentID,
		OrgID:        &orgID,
		Name:         "Aria",
		IsEmployee:   true,
		Status:       "active",
		Model:        employeeruntime.DefaultEmployeeModel,
		CredentialID: &cred.ID,
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	encryptedSecret, err := encKey.EncryptString("runtime-secret")
	if err != nil {
		t.Fatalf("encrypt secret: %v", err)
	}
	sb := model.Sandbox{
		ID:                     sandboxID,
		OrgID:                  &orgID,
		EmployeeID:             &agentID,
		ExternalID:             "sb",
		EncryptedRuntimeSecret: encryptedSecret,
		Status:                 "running",
	}

	var received employeeruntime.ConfigUpdateRequest
	runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/readyz":
			w.WriteHeader(http.StatusOK)
		case "/config":
			if r.Header.Get("Authorization") != "Bearer runtime-secret" {
				t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
			}
			if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
				t.Fatalf("decode config: %v", err)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"applied":1}`))
		default:
			t.Fatalf("unexpected runtime path: %s", r.URL.Path)
		}
	}))
	defer runtime.Close()
	sb.RuntimeURL = runtime.URL
	if err := db.Create(&sb).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}

	handler := &EmployeeTriggerDispatchHandler{
		db: db,
		compileDeps: employeeruntime.CompileDeps{
			DB:         db,
			EncKey:     encKey,
			SigningKey: []byte("test-signing-key-32-bytes-long!!"),
			Cfg:        &config.Config{ProxyHost: "proxy.hivy.test"},
		},
	}
	client := employeeruntime.NewClient(runtime.URL, "runtime-secret")
	if err := handler.syncRuntime(context.Background(), &agent, &sb, client); err != nil {
		t.Fatalf("sync runtime: %v", err)
	}
	if received.Definition == nil {
		t.Fatalf("runtime config missing definition")
	}
	proxyToken := received.RuntimeEnv[employeeruntime.ProxyAPIKeyEnv]
	if !strings.HasPrefix(proxyToken, "ptok_") {
		t.Fatalf("runtime config missing proxy token env: %q", proxyToken)
	}
}
