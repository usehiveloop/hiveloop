package handler_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/bridge"
	"github.com/usehiveloop/hiveloop/internal/crypto"
	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/model"
)

const cloudAgentTestDBURL = "postgres://hiveloop:localdev@localhost:5433/hiveloop_test?sslmode=disable" // #nosec G101 -- test fixture

type cloudAgentHarness struct {
	db         *gorm.DB
	router     *chi.Mux
	orgID      uuid.UUID
	agentID    uuid.UUID
	bridgeKey  string
	fakeBridge *fakeCloudAgentBridge
}

type sentCloudAgentMessage struct {
	ConversationID string
	Content        string
}

type fakeCloudAgentBridge struct {
	createdAgentIDs []string
	sentMessages    []sentCloudAgentMessage
	ended           []string
}

func (f *fakeCloudAgentBridge) CreateConversation(_ context.Context, agentID string) (*bridge.CreateConversationResponse, error) {
	f.createdAgentIDs = append(f.createdAgentIDs, agentID)
	return &bridge.CreateConversationResponse{
		ConversationId: fmt.Sprintf("bridge-created-%d", len(f.createdAgentIDs)),
		StreamUrl:      "http://bridge.local/stream",
	}, nil
}

func (f *fakeCloudAgentBridge) SendMessage(_ context.Context, convID string, content string) error {
	f.sentMessages = append(f.sentMessages, sentCloudAgentMessage{ConversationID: convID, Content: content})
	return nil
}

func (f *fakeCloudAgentBridge) EndConversation(_ context.Context, convID string) error {
	f.ended = append(f.ended, convID)
	return nil
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

	fakeBridge := &fakeCloudAgentBridge{}
	cloudAgentHandler := handler.NewCloudAgentHandlerWithHooks(database, encKey, handler.CloudAgentHandlerHooks{
		CreateDedicatedSandbox: func(_ context.Context, agent *model.Agent) (*model.Sandbox, error) {
			encryptedCloudKey, err := encKey.EncryptString("cloud-agent-bridge-key")
			if err != nil {
				return nil, err
			}
			sb := &model.Sandbox{
				ID:                    uuid.New(),
				OrgID:                 agent.OrgID,
				AgentID:               &agent.ID,
				EncryptedBridgeAPIKey: encryptedCloudKey,
				Status:                "running",
				ExternalID:            fmt.Sprintf("cloud-ext-%s", uuid.New().String()[:8]),
				BridgeURL:             "http://bridge.local",
			}
			if err := database.Create(sb).Error; err != nil {
				return nil, err
			}
			return sb, nil
		},
		PushAgentToSandbox: func(context.Context, *model.Agent, *model.Sandbox) error {
			return nil
		},
		GetBridgeClient: func(context.Context, *model.Sandbox) (handler.CloudAgentBridgeClient, error) {
			return fakeBridge, nil
		},
		StopSandbox: func(_ context.Context, sb *model.Sandbox) error {
			return database.Model(sb).Update("status", "stopped").Error
		},
	})

	router := chi.NewRouter()
	router.Get("/internal/employees/{employeeID}/cloud-agents/", cloudAgentHandler.ListCloudAgents)
	router.Get("/internal/employees/{employeeID}/cloud-agents/{agentID}/tasks", cloudAgentHandler.ListTasks)
	router.Get("/internal/employees/{employeeID}/cloud-agents/{agentID}/tasks/{taskID}", cloudAgentHandler.GetTask)
	router.Post("/internal/employees/{employeeID}/cloud-agents/{agentID}/tasks", cloudAgentHandler.CreateTask)
	router.Post("/internal/employees/{employeeID}/cloud-agents/{agentID}/tasks/{taskID}/message", cloudAgentHandler.SendTaskMessage)
	router.Post("/internal/employees/{employeeID}/cloud-agents/{agentID}/tasks/{taskID}", cloudAgentHandler.TerminateTask)

	t.Cleanup(func() {
		database.Where("org_id = ?", orgID).Delete(&model.CloudAgentTask{})
		database.Where("org_id = ?", orgID).Delete(&model.ConversationEvent{})
		database.Where("org_id = ?", orgID).Delete(&model.AgentConversation{})
		database.Where("org_id = ?", orgID).Delete(&model.Sandbox{})
		database.Where("agent_id = ? OR subagent_id = ?", employeeID, employeeID).Delete(&model.AgentSubagent{})
		database.Where("org_id = ?", orgID).Delete(&model.Agent{})
		database.Where("id = ?", employeeID).Delete(&model.Agent{})
		database.Where("id = ?", orgID).Delete(&model.Org{})
	})

	return &cloudAgentHarness{
		db:         database,
		router:     router,
		orgID:      orgID,
		agentID:    employeeID,
		bridgeKey:  bridgeKey,
		fakeBridge: fakeBridge,
	}
}

func (h *cloudAgentHarness) seedCloudAgent(t *testing.T) (cloudAgentID uuid.UUID) {
	t.Helper()
	cloudAgentID = uuid.New()
	if err := h.db.Create(&model.Agent{
		ID:     cloudAgentID,
		OrgID:  &h.orgID,
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
	var count int64
	h.db.Model(&model.ConversationEvent{}).Where("conversation_id = ?", convID).Count(&count)
	now := time.Now().UTC().Add(time.Duration(count) * time.Second)
	if err := h.db.Create(&model.ConversationEvent{
		ID:                   uuid.New(),
		OrgID:                h.orgID,
		ConversationID:       convID,
		EventID:              uuid.New().String(),
		EventType:            eventType,
		AgentID:              "test-agent",
		BridgeConversationID: "bridge-conv",
		Timestamp:            now,
		SequenceNumber:       count + 1,
		Data:                 model.RawJSON(`{}`),
	}).Error; err != nil {
		t.Fatalf("create event: %v", err)
	}
}

func (h *cloudAgentHarness) doRequest(method, path string) *httptest.ResponseRecorder {
	return h.doJSONRequest(method, path, nil)
}

func (h *cloudAgentHarness) doJSONRequest(method, path string, body any) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			panic(err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Authorization", "Bearer "+h.bridgeKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
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
			ID          string `json:"id"`
			Name        string `json:"name"`
			RecentTasks []struct {
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

func TestCloudAgentListTasks_Success(t *testing.T) {
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

func TestCloudAgentGetTask_Success(t *testing.T) {
	h := newCloudAgentHarness(t)
	cloudAgentID := h.seedCloudAgent(t)
	task := h.seedTask(t, cloudAgentID, "Specific task")

	rec := h.doRequest("GET", fmt.Sprintf("/internal/employees/%s/cloud-agents/%s/tasks/%s", h.agentID, cloudAgentID, task.ID))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var body struct {
		ID                     string         `json:"id"`
		Brief                  string         `json:"brief"`
		ParentConversationType string         `json:"parent_conversation_type"`
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

func TestCloudAgentGetTask_IncludesRecentEvents(t *testing.T) {
	h := newCloudAgentHarness(t)
	cloudAgentID := h.seedCloudAgent(t)
	task := h.seedTask(t, cloudAgentID, "Specific task")
	h.seedEvent(t, task.ConversationID, "AgentError")
	h.seedEvent(t, task.ConversationID, "ConversationEnded")

	rec := h.doRequest("GET", fmt.Sprintf("/internal/employees/%s/cloud-agents/%s/tasks/%s", h.agentID, cloudAgentID, task.ID))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var body struct {
		RecentEvents []struct {
			EventType string `json:"event_type"`
		} `json:"recent_events"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.RecentEvents) != 2 {
		t.Fatalf("expected 2 recent events, got %d", len(body.RecentEvents))
	}
	if body.RecentEvents[0].EventType != "ConversationEnded" {
		t.Fatalf("expected newest event first, got %q", body.RecentEvents[0].EventType)
	}
}

func TestCloudAgentCreateTask_Success_UsesBridgeContract(t *testing.T) {
	h := newCloudAgentHarness(t)
	cloudAgentID := h.seedCloudAgent(t)

	rec := h.doJSONRequest("POST", fmt.Sprintf("/internal/employees/%s/cloud-agents/%s/tasks", h.agentID, cloudAgentID), map[string]any{
		"brief":                    "Build the billing report.",
		"parent_conversation_type": "agent_conversation",
		"parent_conversation_id":   "session-123",
		"metadata": map[string]any{
			"description": "billing report",
			"session_id":  "session-123",
			"source":      "employee_bridge",
		},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(h.fakeBridge.createdAgentIDs) != 1 || h.fakeBridge.createdAgentIDs[0] != cloudAgentID.String() {
		t.Fatalf("expected bridge conversation for agent %s, got %#v", cloudAgentID, h.fakeBridge.createdAgentIDs)
	}
	if len(h.fakeBridge.sentMessages) != 1 || h.fakeBridge.sentMessages[0].Content != "Build the billing report." {
		t.Fatalf("expected brief to be sent exactly once, got %#v", h.fakeBridge.sentMessages)
	}

	var body struct {
		TaskID  string `json:"task_id"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.TaskID == "" {
		t.Fatal("expected task_id")
	}
	if !strings.Contains(body.Message, "cloud_agent_task_status") || !strings.Contains(body.Message, "cloud_agent_task_send_message") {
		t.Fatalf("response message should reference bridge tool names, got %q", body.Message)
	}

	var task model.CloudAgentTask
	if err := h.db.Where("id = ?", body.TaskID).First(&task).Error; err != nil {
		t.Fatalf("load created task: %v", err)
	}
	if task.Brief != "Build the billing report." {
		t.Fatalf("expected brief to be stored unchanged, got %q", task.Brief)
	}
	if task.Metadata["description"] != "billing report" {
		t.Fatalf("expected metadata description to be preserved, got %v", task.Metadata)
	}
}

func TestCloudAgentSendTaskMessage_Success(t *testing.T) {
	h := newCloudAgentHarness(t)
	cloudAgentID := h.seedCloudAgent(t)
	task := h.seedTask(t, cloudAgentID, "Specific task")

	rec := h.doJSONRequest("POST", fmt.Sprintf("/internal/employees/%s/cloud-agents/%s/tasks/%s/message", h.agentID, cloudAgentID, task.ID), map[string]any{
		"message": "Please narrow the scope.",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var conv model.AgentConversation
	if err := h.db.Where("id = ?", task.ConversationID).First(&conv).Error; err != nil {
		t.Fatalf("load conversation: %v", err)
	}
	if len(h.fakeBridge.sentMessages) != 1 {
		t.Fatalf("expected 1 sent message, got %#v", h.fakeBridge.sentMessages)
	}
	if got := h.fakeBridge.sentMessages[0]; got.ConversationID != conv.BridgeConversationID || got.Content != "Please narrow the scope." {
		t.Fatalf("unexpected sent message: %#v", got)
	}
}

func TestCloudAgentSendTaskMessage_RejectsEmptyMessage(t *testing.T) {
	h := newCloudAgentHarness(t)
	cloudAgentID := h.seedCloudAgent(t)
	task := h.seedTask(t, cloudAgentID, "Specific task")

	rec := h.doJSONRequest("POST", fmt.Sprintf("/internal/employees/%s/cloud-agents/%s/tasks/%s/message", h.agentID, cloudAgentID, task.ID), map[string]any{
		"message": "   ",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCloudAgentTerminateTask_Success(t *testing.T) {
	h := newCloudAgentHarness(t)
	cloudAgentID := h.seedCloudAgent(t)
	task := h.seedTask(t, cloudAgentID, "Specific task")
	h.seedEvent(t, task.ConversationID, "message_received")

	rec := h.doJSONRequest("POST", fmt.Sprintf("/internal/employees/%s/cloud-agents/%s/tasks/%s", h.agentID, cloudAgentID, task.ID), map[string]any{
		"reason": "no longer needed",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var conv model.AgentConversation
	if err := h.db.Where("id = ?", task.ConversationID).First(&conv).Error; err != nil {
		t.Fatalf("load conversation: %v", err)
	}
	if conv.Status != "ended" || conv.EndedAt == nil {
		t.Fatalf("expected conversation ended with ended_at, got status=%q ended_at=%v", conv.Status, conv.EndedAt)
	}
	if len(h.fakeBridge.ended) != 1 || h.fakeBridge.ended[0] != conv.BridgeConversationID {
		t.Fatalf("expected bridge conversation %s to be ended, got %#v", conv.BridgeConversationID, h.fakeBridge.ended)
	}

	var event model.ConversationEvent
	if err := h.db.Where("conversation_id = ? AND event_type = ?", task.ConversationID, "ConversationEnded").First(&event).Error; err != nil {
		t.Fatalf("expected ConversationEnded event: %v", err)
	}
	if !strings.Contains(string(event.Data), "no longer needed") {
		t.Fatalf("expected termination reason in event data, got %s", event.Data)
	}
}

func TestCloudAgentTerminateTask_UnknownTask(t *testing.T) {
	h := newCloudAgentHarness(t)
	cloudAgentID := h.seedCloudAgent(t)

	rec := h.doJSONRequest("POST", fmt.Sprintf("/internal/employees/%s/cloud-agents/%s/tasks/%s", h.agentID, cloudAgentID, uuid.New()), map[string]any{
		"reason": "no longer needed",
	})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCloudAgentTerminateTask_InvalidTaskID(t *testing.T) {
	h := newCloudAgentHarness(t)
	cloudAgentID := h.seedCloudAgent(t)

	rec := h.doJSONRequest("POST", fmt.Sprintf("/internal/employees/%s/cloud-agents/%s/tasks/not-a-uuid", h.agentID, cloudAgentID), map[string]any{
		"reason": "no longer needed",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCloudAgentListTasks_Unauthorized(t *testing.T) {
	h := newCloudAgentHarness(t)
	cloudAgentID := h.seedCloudAgent(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", fmt.Sprintf("/internal/employees/%s/cloud-agents/%s/tasks", h.agentID, cloudAgentID), nil)
	h.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestCloudAgentListTasks_SubagentNotFound(t *testing.T) {
	h := newCloudAgentHarness(t)
	fakeAgentID := uuid.New()

	rec := h.doRequest("GET", fmt.Sprintf("/internal/employees/%s/cloud-agents/%s/tasks", h.agentID, fakeAgentID))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}
