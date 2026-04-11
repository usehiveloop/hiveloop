package handler_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/handler"
	"github.com/ziraloop/ziraloop/internal/mcp/catalog"
	"github.com/ziraloop/ziraloop/internal/middleware"
	"github.com/ziraloop/ziraloop/internal/model"
)

type triggerTestHarness struct {
	db      *gorm.DB
	catalog *catalog.Catalog
	router  *chi.Mux
	org     model.Org
	agent   model.Agent
	integ   model.Integration
	conn    model.Connection
}

func newTriggerHarness(t *testing.T) *triggerTestHarness {
	t.Helper()
	db := connectTestDB(t)
	// agent_triggers schema comes from model.AutoMigrate inside connectTestDB.
	// Drop stale legacy indexes from earlier test runs that may linger in the
	// shared test database.
	db.Exec(`DROP INDEX IF EXISTS idx_agent_triggers_unique`)

	actionsCatalog := catalog.Global()

	triggerHandler := handler.NewAgentTriggerHandler(db, actionsCatalog)

	router := chi.NewRouter()
	router.Route("/v1/agents/{agentID}/triggers", func(r chi.Router) {
		r.Post("/", triggerHandler.Create)
		r.Get("/", triggerHandler.List)
		r.Get("/{id}", triggerHandler.Get)
		r.Put("/{id}", triggerHandler.Update)
		r.Delete("/{id}", triggerHandler.Delete)
	})

	org := createTestOrg(t, db)

	// Create a GitHub integration and connection.
	integ := createTestIntegration(t, db, org.ID, "github")
	conn := createTestConnection(t, db, org.ID, integ.ID, "nango-trigger-test")

	// Create a test agent with the connection in its integrations.
	agentOrgID := org.ID
	agent := model.Agent{
		ID:           uuid.New(),
		OrgID:        &agentOrgID,
		Name:         fmt.Sprintf("trigger-test-agent-%s", uuid.New().String()[:8]),
		Integrations: model.JSON{conn.ID.String(): map[string]any{"actions": []string{}}},
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	t.Cleanup(func() {
		db.Where("agent_id = ?", agent.ID).Delete(&model.AgentTrigger{})
		db.Where("id = ?", agent.ID).Delete(&model.Agent{})
	})

	return &triggerTestHarness{
		db:      db,
		catalog: actionsCatalog,
		router:  router,
		org:     org,
		agent:   agent,
		integ:   integ,
		conn:    conn,
	}
}

func (h *triggerTestHarness) doRequest(t *testing.T, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var bodyReader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("encode body: %v", err)
		}
		bodyReader = bytes.NewReader(buf)
	}
	req := httptest.NewRequest(method, path, bodyReader)
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &h.org)
	recorder := httptest.NewRecorder()
	h.router.ServeHTTP(recorder, req)
	return recorder
}

func (h *triggerTestHarness) basePath() string {
	return "/v1/agents/" + h.agent.ID.String() + "/triggers"
}

func parseResponse(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var resp map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v (body: %s)", err, recorder.Body.String())
	}
	return resp
}

// ──────────────────────────────────────────────
// CREATE — happy path
// ──────────────────────────────────────────────

func TestAgentTrigger_Create_Success(t *testing.T) {
	h := newTriggerHarness(t)

	body := map[string]any{
		"connection_id": h.conn.ID.String(),
		"trigger_keys": []string{"issues.opened"},
	}
	recorder := h.doRequest(t, http.MethodPost, h.basePath(), body)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", recorder.Code, recorder.Body.String())
	}

	resp := parseResponse(t, recorder)
	triggerKeys := resp["trigger_keys"].([]any)
	if len(triggerKeys) != 1 || triggerKeys[0] != "issues.opened" {
		t.Fatalf("expected trigger_keys=[issues.opened], got %v", triggerKeys)
	}
	if resp["enabled"] != true {
		t.Fatalf("expected enabled=true, got %v", resp["enabled"])
	}
	if resp["provider"] != "github" {
		t.Fatalf("expected provider=github, got %v", resp["provider"])
	}
	if resp["agent_id"] != h.agent.ID.String() {
		t.Fatalf("expected agent_id=%s, got %v", h.agent.ID, resp["agent_id"])
	}
}

func TestAgentTrigger_Create_WithConditions(t *testing.T) {
	h := newTriggerHarness(t)

	body := map[string]any{
		"connection_id": h.conn.ID.String(),
		"trigger_keys": []string{"push"},
		"conditions": map[string]any{
			"mode": "all",
			"conditions": []map[string]any{
				{"path": "repository.full_name", "operator": "equals", "value": "myorg/myrepo"},
				{"path": "ref", "operator": "equals", "value": "refs/heads/main"},
			},
		},
	}
	recorder := h.doRequest(t, http.MethodPost, h.basePath(), body)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", recorder.Code, recorder.Body.String())
	}

	resp := parseResponse(t, recorder)
	conditions := resp["conditions"].(map[string]any)
	if conditions["mode"] != "all" {
		t.Fatalf("expected mode=all, got %v", conditions["mode"])
	}
	condList := conditions["conditions"].([]any)
	if len(condList) != 2 {
		t.Fatalf("expected 2 conditions, got %d", len(condList))
	}
}

func TestAgentTrigger_Create_WithContextActions(t *testing.T) {
	h := newTriggerHarness(t)

	body := map[string]any{
		"connection_id": h.conn.ID.String(),
		"trigger_keys": []string{"issues.opened"},
		"context_actions": []map[string]any{
			{
				"as": "issue_detail",
				"action": "issues_get",
				"params": map[string]string{
					"owner":        "{{ trigger.repository.owner.login }}",
					"repo":         "{{ trigger.repository.name }}",
					"issue_number": "{{ trigger.issue.number }}",
				},
			},
			{
				"as": "labels",
				"action": "issues_list_labels_on_issue",
				"params": map[string]string{
					"owner":        "{{ trigger.repository.owner.login }}",
					"repo":         "{{ trigger.repository.name }}",
					"issue_number": "{{ trigger.issue.number }}",
				},
			},
		},
	}
	recorder := h.doRequest(t, http.MethodPost, h.basePath(), body)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", recorder.Code, recorder.Body.String())
	}

	resp := parseResponse(t, recorder)
	contextActions := resp["context_actions"].([]any)
	if len(contextActions) != 2 {
		t.Fatalf("expected 2 context_actions, got %d", len(contextActions))
	}
}

func TestAgentTrigger_Create_DisabledByDefault(t *testing.T) {
	h := newTriggerHarness(t)

	body := map[string]any{
		"connection_id": h.conn.ID.String(),
		"trigger_keys": []string{"pull_request.opened"},
		"enabled":       false,
	}
	recorder := h.doRequest(t, http.MethodPost, h.basePath(), body)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", recorder.Code, recorder.Body.String())
	}

	resp := parseResponse(t, recorder)
	if resp["enabled"] != false {
		t.Fatalf("expected enabled=false, got %v", resp["enabled"])
	}
}

// ──────────────────────────────────────────────
// CREATE — validation failures
// ──────────────────────────────────────────────

func TestAgentTrigger_Create_MissingFields(t *testing.T) {
	h := newTriggerHarness(t)

	recorder := h.doRequest(t, http.MethodPost, h.basePath(), map[string]any{})
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", recorder.Code, recorder.Body.String())
	}
}

func TestAgentTrigger_Create_InvalidTriggerKey(t *testing.T) {
	h := newTriggerHarness(t)

	body := map[string]any{
		"connection_id": h.conn.ID.String(),
		"trigger_keys": []string{"nonexistent.event"},
	}
	recorder := h.doRequest(t, http.MethodPost, h.basePath(), body)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", recorder.Code, recorder.Body.String())
	}
	resp := parseResponse(t, recorder)
	errMsg := resp["error"].(string)
	if errMsg == "" {
		t.Fatal("expected error message about invalid trigger_keys")
	}
}

func TestAgentTrigger_Create_InvalidConnectionID(t *testing.T) {
	h := newTriggerHarness(t)

	body := map[string]any{
		"connection_id": uuid.New().String(),
		"trigger_keys": []string{"issues.opened"},
	}
	recorder := h.doRequest(t, http.MethodPost, h.basePath(), body)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", recorder.Code, recorder.Body.String())
	}
}

func TestAgentTrigger_Create_AgentNotFound(t *testing.T) {
	h := newTriggerHarness(t)

	body := map[string]any{
		"connection_id": h.conn.ID.String(),
		"trigger_keys": []string{"issues.opened"},
	}
	fakeAgentPath := "/v1/agents/" + uuid.New().String() + "/triggers"
	recorder := h.doRequest(t, http.MethodPost, fakeAgentPath, body)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d; body: %s", recorder.Code, recorder.Body.String())
	}
}

func TestAgentTrigger_Create_InvalidConditionOperator(t *testing.T) {
	h := newTriggerHarness(t)

	body := map[string]any{
		"connection_id": h.conn.ID.String(),
		"trigger_keys": []string{"issues.opened"},
		"conditions": map[string]any{
			"mode": "all",
			"conditions": []map[string]any{
				{"path": "repository.full_name", "operator": "INVALID_OP", "value": "test"},
			},
		},
	}
	recorder := h.doRequest(t, http.MethodPost, h.basePath(), body)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", recorder.Code, recorder.Body.String())
	}
}

func TestAgentTrigger_Create_InvalidConditionMode(t *testing.T) {
	h := newTriggerHarness(t)

	body := map[string]any{
		"connection_id": h.conn.ID.String(),
		"trigger_keys": []string{"issues.opened"},
		"conditions": map[string]any{
			"mode": "invalid_mode",
			"conditions": []map[string]any{
				{"path": "ref", "operator": "equals", "value": "main"},
			},
		},
	}
	recorder := h.doRequest(t, http.MethodPost, h.basePath(), body)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", recorder.Code, recorder.Body.String())
	}
}

func TestAgentTrigger_Create_ConditionMissingPath(t *testing.T) {
	h := newTriggerHarness(t)

	body := map[string]any{
		"connection_id": h.conn.ID.String(),
		"trigger_keys": []string{"issues.opened"},
		"conditions": map[string]any{
			"mode": "all",
			"conditions": []map[string]any{
				{"path": "", "operator": "equals", "value": "test"},
			},
		},
	}
	recorder := h.doRequest(t, http.MethodPost, h.basePath(), body)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", recorder.Code, recorder.Body.String())
	}
}

func TestAgentTrigger_Create_ConditionMissingValueForNonExistsOperator(t *testing.T) {
	h := newTriggerHarness(t)

	body := map[string]any{
		"connection_id": h.conn.ID.String(),
		"trigger_keys": []string{"issues.opened"},
		"conditions": map[string]any{
			"mode": "all",
			"conditions": []map[string]any{
				{"path": "ref", "operator": "equals"},
			},
		},
	}
	recorder := h.doRequest(t, http.MethodPost, h.basePath(), body)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", recorder.Code, recorder.Body.String())
	}
}

func TestAgentTrigger_Create_ExistsOperatorNoValueRequired(t *testing.T) {
	h := newTriggerHarness(t)

	body := map[string]any{
		"connection_id": h.conn.ID.String(),
		"trigger_keys": []string{"release.published"},
		"conditions": map[string]any{
			"mode": "all",
			"conditions": []map[string]any{
				{"path": "release.body", "operator": "exists"},
			},
		},
	}
	recorder := h.doRequest(t, http.MethodPost, h.basePath(), body)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", recorder.Code, recorder.Body.String())
	}
}

func TestAgentTrigger_Create_ContextActionInvalidAction(t *testing.T) {
	h := newTriggerHarness(t)

	body := map[string]any{
		"connection_id": h.conn.ID.String(),
		"trigger_keys": []string{"issues.opened"},
		"context_actions": []map[string]any{
			{  "as": "data", "action": "nonexistent_action", "params": map[string]string{}},
		},
	}
	recorder := h.doRequest(t, http.MethodPost, h.basePath(), body)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", recorder.Code, recorder.Body.String())
	}
}

func TestAgentTrigger_Create_ContextActionWriteActionRejected(t *testing.T) {
	h := newTriggerHarness(t)

	body := map[string]any{
		"connection_id": h.conn.ID.String(),
		"trigger_keys": []string{"issues.opened"},
		"context_actions": []map[string]any{
			{
				"as": "create_issue",
				"action": "issues_create",
				"params": map[string]string{
					"owner": "test",
					"repo":  "test",
				},
			},
		},
	}
	recorder := h.doRequest(t, http.MethodPost, h.basePath(), body)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", recorder.Code, recorder.Body.String())
	}
	resp := parseResponse(t, recorder)
	errMsg := resp["error"].(string)
	if errMsg == "" {
		t.Fatal("expected error about write action not allowed")
	}
}

func TestAgentTrigger_Create_ContextActionDuplicateID(t *testing.T) {
	h := newTriggerHarness(t)

	body := map[string]any{
		"connection_id": h.conn.ID.String(),
		"trigger_keys": []string{"issues.opened"},
		"context_actions": []map[string]any{
			{"as": "same_id", "action": "issues_get", "params": map[string]string{}},
			{"as": "same_id", "action": "issues_list_comments", "params": map[string]string{}},
		},
	}
	recorder := h.doRequest(t, http.MethodPost, h.basePath(), body)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", recorder.Code, recorder.Body.String())
	}
}

func TestAgentTrigger_Create_ContextActionMissingID(t *testing.T) {
	h := newTriggerHarness(t)

	body := map[string]any{
		"connection_id": h.conn.ID.String(),
		"trigger_keys": []string{"issues.opened"},
		"context_actions": []map[string]any{
			{"as": "", "action": "issues_get", "params": map[string]string{}},
		},
	}
	recorder := h.doRequest(t, http.MethodPost, h.basePath(), body)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", recorder.Code, recorder.Body.String())
	}
}

func TestAgentTrigger_Create_ConnectionNotInAgentIntegrations(t *testing.T) {
	h := newTriggerHarness(t)

	// Create a second connection that is NOT in the agent's integrations.
	integ2 := createTestIntegration(t, h.db, h.org.ID, "slack")
	conn2 := createTestConnection(t, h.db, h.org.ID, integ2.ID, "nango-slack-test")

	body := map[string]any{
		"connection_id": conn2.ID.String(),
		"trigger_keys": []string{"message"},
	}
	recorder := h.doRequest(t, http.MethodPost, h.basePath(), body)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", recorder.Code, recorder.Body.String())
	}
	resp := parseResponse(t, recorder)
	errMsg := resp["error"].(string)
	if errMsg == "" {
		t.Fatal("expected error about connection not configured for agent")
	}
}

func TestAgentTrigger_Create_DuplicateConnection(t *testing.T) {
	h := newTriggerHarness(t)

	body := map[string]any{
		"connection_id": h.conn.ID.String(),
		"trigger_keys": []string{"issues.closed"},
	}
	recorder := h.doRequest(t, http.MethodPost, h.basePath(), body)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("first create: expected 201, got %d; body: %s", recorder.Code, recorder.Body.String())
	}

	// Same connection + agent should conflict (even with different trigger keys).
	body2 := map[string]any{
		"connection_id": h.conn.ID.String(),
		"trigger_keys": []string{"issues.opened"},
	}
	recorder = h.doRequest(t, http.MethodPost, h.basePath(), body2)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d; body: %s", recorder.Code, recorder.Body.String())
	}
}

// ──────────────────────────────────────────────
// LIST
// ──────────────────────────────────────────────

func TestAgentTrigger_List(t *testing.T) {
	h := newTriggerHarness(t)

	// Create a trigger with multiple keys.
	body := map[string]any{
		"connection_id": h.conn.ID.String(),
		"trigger_keys":  []string{"issues.opened", "pull_request.opened"},
	}
	recorder := h.doRequest(t, http.MethodPost, h.basePath(), body)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d; body: %s", recorder.Code, recorder.Body.String())
	}

	recorder = h.doRequest(t, http.MethodGet, h.basePath(), nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", recorder.Code, recorder.Body.String())
	}

	resp := parseResponse(t, recorder)
	data := resp["data"].([]any)
	if len(data) != 1 {
		t.Fatalf("expected 1 trigger, got %d", len(data))
	}
	triggerKeys := data[0].(map[string]any)["trigger_keys"].([]any)
	if len(triggerKeys) != 2 {
		t.Fatalf("expected 2 trigger_keys, got %d", len(triggerKeys))
	}
}

// ──────────────────────────────────────────────
// GET
// ──────────────────────────────────────────────

func TestAgentTrigger_Get(t *testing.T) {
	h := newTriggerHarness(t)

	body := map[string]any{
		"connection_id": h.conn.ID.String(),
		"trigger_keys": []string{"workflow_run.completed"},
	}
	createRecorder := h.doRequest(t, http.MethodPost, h.basePath(), body)
	if createRecorder.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", createRecorder.Code)
	}
	created := parseResponse(t, createRecorder)
	triggerID := created["id"].(string)

	recorder := h.doRequest(t, http.MethodGet, h.basePath()+"/"+triggerID, nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", recorder.Code, recorder.Body.String())
	}

	resp := parseResponse(t, recorder)
	triggerKeys := resp["trigger_keys"].([]any)
	if len(triggerKeys) != 1 || triggerKeys[0] != "workflow_run.completed" {
		t.Fatalf("expected trigger_keys=[workflow_run.completed], got %v", triggerKeys)
	}
}

func TestAgentTrigger_Get_NotFound(t *testing.T) {
	h := newTriggerHarness(t)

	recorder := h.doRequest(t, http.MethodGet, h.basePath()+"/"+uuid.New().String(), nil)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d; body: %s", recorder.Code, recorder.Body.String())
	}
}

// ──────────────────────────────────────────────
// UPDATE
// ──────────────────────────────────────────────

func TestAgentTrigger_Update_ToggleEnabled(t *testing.T) {
	h := newTriggerHarness(t)

	body := map[string]any{
		"connection_id": h.conn.ID.String(),
		"trigger_keys": []string{"create"},
	}
	createRecorder := h.doRequest(t, http.MethodPost, h.basePath(), body)
	if createRecorder.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", createRecorder.Code)
	}
	created := parseResponse(t, createRecorder)
	triggerID := created["id"].(string)

	// Disable.
	disabled := false
	updateBody := map[string]any{"enabled": disabled}
	recorder := h.doRequest(t, http.MethodPut, h.basePath()+"/"+triggerID, updateBody)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", recorder.Code, recorder.Body.String())
	}
	resp := parseResponse(t, recorder)
	if resp["enabled"] != false {
		t.Fatalf("expected enabled=false, got %v", resp["enabled"])
	}
}

func TestAgentTrigger_Update_ContextActionsValidated(t *testing.T) {
	h := newTriggerHarness(t)

	body := map[string]any{
		"connection_id": h.conn.ID.String(),
		"trigger_keys": []string{"delete"},
	}
	createRecorder := h.doRequest(t, http.MethodPost, h.basePath(), body)
	if createRecorder.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", createRecorder.Code)
	}
	created := parseResponse(t, createRecorder)
	triggerID := created["id"].(string)

	// Try to add a write action as context — should fail.
	updateBody := map[string]any{
		"context_actions": []map[string]any{
			{"as": "bad", "action": "issues_create", "params": map[string]string{}},
		},
	}
	recorder := h.doRequest(t, http.MethodPut, h.basePath()+"/"+triggerID, updateBody)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", recorder.Code, recorder.Body.String())
	}
}

func TestAgentTrigger_Update_NotFound(t *testing.T) {
	h := newTriggerHarness(t)

	recorder := h.doRequest(t, http.MethodPut, h.basePath()+"/"+uuid.New().String(), map[string]any{"enabled": false})
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d; body: %s", recorder.Code, recorder.Body.String())
	}
}

// ──────────────────────────────────────────────
// DELETE
// ──────────────────────────────────────────────

func TestAgentTrigger_Delete(t *testing.T) {
	h := newTriggerHarness(t)

	body := map[string]any{
		"connection_id": h.conn.ID.String(),
		"trigger_keys":  []string{"release.published"},
	}
	createRecorder := h.doRequest(t, http.MethodPost, h.basePath(), body)
	if createRecorder.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", createRecorder.Code)
	}
	created := parseResponse(t, createRecorder)
	triggerID := created["id"].(string)

	// Delete.
	recorder := h.doRequest(t, http.MethodDelete, h.basePath()+"/"+triggerID, nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", recorder.Code, recorder.Body.String())
	}

	// Verify it's gone.
	recorder = h.doRequest(t, http.MethodGet, h.basePath()+"/"+triggerID, nil)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d", recorder.Code)
	}
}

func TestAgentTrigger_Delete_NotFound(t *testing.T) {
	h := newTriggerHarness(t)

	recorder := h.doRequest(t, http.MethodDelete, h.basePath()+"/"+uuid.New().String(), nil)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d; body: %s", recorder.Code, recorder.Body.String())
	}
}
