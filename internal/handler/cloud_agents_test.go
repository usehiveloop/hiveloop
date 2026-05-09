package handler_test

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/crypto"
	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/model"
)

const cloudAgentTestDBURL = "postgres://hiveloop:localdev@localhost:5433/hiveloop_test?sslmode=disable" // #nosec G101 -- test fixture

type cloudAgentHarness struct {
	db        *gorm.DB
	router    *chi.Mux
	orgID     uuid.UUID
	agentID   uuid.UUID
	bridgeKey string
}

func newCloudAgentHarness(t *testing.T) *cloudAgentHarness {
	t.Helper()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = cloudAgentTestDBURL
	}
	database, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Skipf("cannot connect to test database: %v", err)
	}
	if err := model.AutoMigrate(database); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 42)
	}
	encKey, err := crypto.NewSymmetricKey(base64.StdEncoding.EncodeToString(key))
	if err != nil {
		t.Fatalf("create symmetric key: %v", err)
	}

	orgID := uuid.New()
	if err := database.Create(&model.Org{
		ID:        orgID,
		Name:      fmt.Sprintf("cloudagent-test-%s", uuid.New().String()[:8]),
		RateLimit: 1000,
		Active:    true,
	}).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}

	employeeID := uuid.New()
	if err := database.Create(&model.Agent{
		ID:         employeeID,
		OrgID:      &orgID,
		Name:       "test-employee",
		Status:     "active",
		IsEmployee: true,
	}).Error; err != nil {
		t.Fatalf("create employee agent: %v", err)
	}

	bridgeKey := "test-bridge-key-cloud-agents"
	encryptedKey, err := encKey.EncryptString(bridgeKey)
	if err != nil {
		t.Fatalf("encrypt bridge key: %v", err)
	}

	sandboxID := uuid.New()
	if err := database.Create(&model.Sandbox{
		ID:                    sandboxID,
		OrgID:                 &orgID,
		AgentID:               &employeeID,
		EncryptedBridgeAPIKey: encryptedKey,
		Status:                "running",
		ExternalID:            "mock-ext-id",
		BridgeURL:             "http://localhost:25434",
	}).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}

	cloudAgentHandler := handler.NewCloudAgentHandler(database, encKey, nil, nil)

	router := chi.NewRouter()
	router.Get("/internal/employees/{employeeID}/cloud-agents/", cloudAgentHandler.ListCloudAgents)
	router.Get("/internal/employees/{employeeID}/cloud-agents/{agentID}/tasks", cloudAgentHandler.ListTasks)
	router.Get("/internal/employees/{employeeID}/cloud-agents/{agentID}/tasks/{taskID}", cloudAgentHandler.GetTask)

	t.Cleanup(func() {
		database.Where("org_id = ?", orgID).Delete(&model.CloudAgentTask{})
		database.Where("id = ?", sandboxID).Delete(&model.Sandbox{})
		database.Where("id = ?", employeeID).Delete(&model.Agent{})
		database.Where("id = ?", orgID).Delete(&model.Org{})
	})

	return &cloudAgentHarness{
		db:        database,
		router:    router,
		orgID:     orgID,
		agentID:   employeeID,
		bridgeKey: bridgeKey,
	}
}

func (h *cloudAgentHarness) seedCloudAgent(t *testing.T) (cloudAgentID uuid.UUID) {
	t.Helper()
	cloudAgentID = uuid.New()
	if err := h.db.Create(&model.Agent{
		ID:     cloudAgentID,
		OrgID: &h.orgID,
		Name:   "test-cloud-agent",
		Status: "active",
	}).Error; err != nil {
		t.Fatalf("create cloud agent: %v", err)
	}
	if err := h.db.Create(&model.AgentSubagent{
		AgentID:    h.agentID,
		SubagentID: cloudAgentID,
	}).Error; err != nil {
		t.Fatalf("create subagent link: %v", err)
	}
	t.Cleanup(func() {
		h.db.Where("agent_id = ? AND subagent_id = ?", h.agentID, cloudAgentID).Delete(&model.AgentSubagent{})
		h.db.Where("id = ?", cloudAgentID).Delete(&model.Agent{})
	})
	return cloudAgentID
}

func (h *cloudAgentHarness) seedTask(t *testing.T, cloudAgentID uuid.UUID, brief string) model.CloudAgentTask {
	t.Helper()
	sandboxID := uuid.New()
	h.db.Create(&model.Sandbox{
		ID:                    sandboxID,
		OrgID:                 &h.orgID,
		AgentID:               &cloudAgentID,
		EncryptedBridgeAPIKey: []byte("fake-key"),
		Status:                "running",
		ExternalID:            fmt.Sprintf("ext-%s", uuid.New().String()[:8]),
		BridgeURL:             "http://localhost:25434",
	})

	convID := uuid.New()
	h.db.Create(&model.AgentConversation{
		ID:                   convID,
		OrgID:                h.orgID,
		AgentID:              cloudAgentID,
		SandboxID:            sandboxID,
		BridgeConversationID: fmt.Sprintf("bridge-%s", uuid.New().String()[:8]),
		Status:               "active",
	})

	task := model.CloudAgentTask{
		ID:                     uuid.New(),
		OrgID:                  h.orgID,
		EmployeeAgentID:        h.agentID,
		CloudAgentID:           cloudAgentID,
		SandboxID:              sandboxID,
		ConversationID:         convID,
		ParentConversationType: "agent_conversation",
		ParentConversationID:   uuid.New().String(),
		Brief:                  brief,
		Metadata:               model.JSON{"key": "value"},
	}
	if err := h.db.Create(&task).Error; err != nil {
		t.Fatalf("create task: %v", err)
	}
	t.Cleanup(func() {
		h.db.Where("id = ?", task.ID).Delete(&model.CloudAgentTask{})
		h.db.Where("id = ?", convID).Delete(&model.AgentConversation{})
		h.db.Where("id = ?", sandboxID).Delete(&model.Sandbox{})
	})
	return task
}

func (h *cloudAgentHarness) seedEvent(t *testing.T, convID uuid.UUID, eventType string) {
	t.Helper()
	h.db.Create(&model.ConversationEvent{
		ID:                   uuid.New(),
		OrgID:                h.orgID,
		ConversationID:       convID,
		EventID:              uuid.New().String(),
		EventType:            eventType,
		AgentID:              "test-agent",
		BridgeConversationID: "bridge-conv",
		Data:                 model.RawJSON(`{}`),
	})
}

func (h *cloudAgentHarness) doRequest(method, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("Authorization", "Bearer "+h.bridgeKey)
	h.router.ServeHTTP(rec, req)
	return rec
}

func TestListCloudAgents_Success(t *testing.T) {
	h := newCloudAgentHarness(t)
	cloudAgentID := h.seedCloudAgent(t)
	task := h.seedTask(t, cloudAgentID, "Fix the login bug")
	h.seedEvent(t, task.ConversationID, "message_received")
	h.seedEvent(t, task.ConversationID, "tool_call")

	rec := h.doRequest("GET", fmt.Sprintf("/internal/employees/%s/cloud-agents/", h.agentID))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var body struct {
		CloudAgents []struct {
			ID           string `json:"id"`
			Name         string `json:"name"`
			RecentTasks  []struct {
				ID           string `json:"id"`
				Brief        string `json:"brief"`
				RecentEvents []struct {
					EventType string `json:"event_type"`
				} `json:"recent_events"`
			} `json:"recent_tasks"`
		} `json:"cloud_agents"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.CloudAgents) != 1 {
		t.Fatalf("expected 1 cloud agent, got %d", len(body.CloudAgents))
	}
	if body.CloudAgents[0].ID != cloudAgentID.String() {
		t.Errorf("expected cloud agent ID %s, got %s", cloudAgentID, body.CloudAgents[0].ID)
	}
	if len(body.CloudAgents[0].RecentTasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(body.CloudAgents[0].RecentTasks))
	}
	if body.CloudAgents[0].RecentTasks[0].Brief != "Fix the login bug" {
		t.Errorf("expected brief 'Fix the login bug', got %q", body.CloudAgents[0].RecentTasks[0].Brief)
	}
	if len(body.CloudAgents[0].RecentTasks[0].RecentEvents) != 2 {
		t.Fatalf("expected 2 events, got %d", len(body.CloudAgents[0].RecentTasks[0].RecentEvents))
	}
}

func TestListTasks_Success(t *testing.T) {
	h := newCloudAgentHarness(t)
	cloudAgentID := h.seedCloudAgent(t)
	h.seedTask(t, cloudAgentID, "Task 1")
	h.seedTask(t, cloudAgentID, "Task 2")

	rec := h.doRequest("GET", fmt.Sprintf("/internal/employees/%s/cloud-agents/%s/tasks", h.agentID, cloudAgentID))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var body struct {
		Data []struct {
			ID    string `json:"id"`
			Brief string `json:"brief"`
		} `json:"data"`
		HasMore bool `json:"has_more"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Data) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(body.Data))
	}
	if body.Data[0].Brief != "Task 2" {
		t.Errorf("expected newest task first (Task 2), got %q", body.Data[0].Brief)
	}
}

func TestGetTask_Success(t *testing.T) {
	h := newCloudAgentHarness(t)
	cloudAgentID := h.seedCloudAgent(t)
	task := h.seedTask(t, cloudAgentID, "Specific task")

	rec := h.doRequest("GET", fmt.Sprintf("/internal/employees/%s/cloud-agents/%s/tasks/%s", h.agentID, cloudAgentID, task.ID))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var body struct {
		ID                     string `json:"id"`
		Brief                  string `json:"brief"`
		ParentConversationType string `json:"parent_conversation_type"`
		Metadata               map[string]any `json:"metadata"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.ID != task.ID.String() {
		t.Errorf("expected task ID %s, got %s", task.ID, body.ID)
	}
	if body.Brief != "Specific task" {
		t.Errorf("expected brief 'Specific task', got %q", body.Brief)
	}
	if body.ParentConversationType != "agent_conversation" {
		t.Errorf("expected parent type 'agent_conversation', got %q", body.ParentConversationType)
	}
	if body.Metadata["key"] != "value" {
		t.Errorf("expected metadata key=value, got %v", body.Metadata)
	}
}

func TestListTasks_Unauthorized(t *testing.T) {
	h := newCloudAgentHarness(t)
	cloudAgentID := h.seedCloudAgent(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", fmt.Sprintf("/internal/employees/%s/cloud-agents/%s/tasks", h.agentID, cloudAgentID), nil)
	h.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestListTasks_SubagentNotFound(t *testing.T) {
	h := newCloudAgentHarness(t)
	fakeAgentID := uuid.New()

	rec := h.doRequest("GET", fmt.Sprintf("/internal/employees/%s/cloud-agents/%s/tasks", h.agentID, fakeAgentID))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}
