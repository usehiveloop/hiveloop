package tasks

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/config"
	"github.com/usehiveloop/hiveloop/internal/crypto"
	"github.com/usehiveloop/hiveloop/internal/employeeruntime"
	"github.com/usehiveloop/hiveloop/internal/enqueue"
	"github.com/usehiveloop/hiveloop/internal/hindsight"
	"github.com/usehiveloop/hiveloop/internal/model"
)

func TestBuildEmployeeRetainItem_BundlesSessionAtCheckpoint(t *testing.T) {
	orgID := uuid.New()
	teamID := uuid.New()
	agentID := uuid.New()
	sandboxID := uuid.New()
	agent := &model.Agent{ID: agentID, OrgID: &orgID, TeamID: &teamID, Name: "Aria"}
	events := []model.EmployeeMemoryEvent{
		memoryEvent(t, orgID, agentID, sandboxID, "S1", "user.message.received", map[string]any{
			"source": "slack", "channel": "C123", "user": "U123", "user_display_name": "Kim",
			"text": "The Platform team requires rollback notes before deploys.",
		}),
		memoryEvent(t, orgID, agentID, sandboxID, "S1", "tool.invoked", map[string]any{
			"source": "slack", "tool": "bash", "result_summary": "Checked deployment docs.",
		}),
		memoryEvent(t, orgID, agentID, sandboxID, "S1", "agent.message.sent", map[string]any{
			"source": "slack", "text": "Done. I added rollback notes to the deploy plan.",
		}),
	}

	item, ok := buildEmployeeRetainItem(agent, EmployeeMemoryRetainPayload{
		AgentID: agentID, SandboxID: sandboxID, SessionID: "S1", SourceEvent: "agent.message.sent",
	}, events)
	if !ok {
		t.Fatal("expected retain item")
	}
	for _, want := range []string{
		"company:" + orgID.String(),
		"team:" + teamID.String(),
		"source:slack",
		"visibility:team",
		"memory_type:team_context",
		"channel:c123",
	} {
		if !hasTaskString(item.Tags, want) {
			t.Fatalf("missing tag %q in %#v", want, item.Tags)
		}
	}
	if !strings.Contains(item.Content, "rollback notes") || !strings.Contains(item.Content, "Employee Aria") {
		t.Fatalf("unexpected content: %q", item.Content)
	}
	if !strings.Contains(item.Content, "Teammate Kim (<@U123>)") {
		t.Fatalf("retain content should preserve Slack user identity: %q", item.Content)
	}
	if !strings.Contains(item.Context, "stable channel user IDs") || !strings.Contains(item.Content, "Do not retain active-conversation framing") {
		t.Fatalf("retain instructions should distinguish people facts from session state: context=%q content=%q", item.Context, item.Content)
	}
	if strings.Contains(item.Content, "Tool ") || strings.Contains(item.Content, "bash") || strings.Contains(item.Content, "Checked deployment docs") {
		t.Fatalf("retain content should not include raw tool calls: %q", item.Content)
	}
	if item.Metadata["session_id"] != "S1" || item.Metadata["event_count"] != "3" || item.Metadata["user"] != "U123" || item.Metadata["user_display_name"] != "Kim" {
		t.Fatalf("unexpected metadata: %#v", item.Metadata)
	}
	if len(item.ObservationScopes) != 2 {
		t.Fatalf("expected company and team observation scopes, got %#v", item.ObservationScopes)
	}
}

func TestBuildEmployeeRetainItem_SkipsNoCheckpointAndSecrets(t *testing.T) {
	orgID := uuid.New()
	agentID := uuid.New()
	sandboxID := uuid.New()
	agent := &model.Agent{ID: agentID, OrgID: &orgID, Name: "Aria"}
	onlyUser := []model.EmployeeMemoryEvent{
		memoryEvent(t, orgID, agentID, sandboxID, "S1", "user.message.received", map[string]any{"text": "remember this later"}),
	}
	if _, ok := buildEmployeeRetainItem(agent, EmployeeMemoryRetainPayload{AgentID: agentID, SandboxID: sandboxID, SessionID: "S1"}, onlyUser); ok {
		t.Fatal("user event without checkpoint should not retain")
	}
	withSecret := append(onlyUser, memoryEvent(t, orgID, agentID, sandboxID, "S1", "agent.message.sent", map[string]any{"text": "api_key=sk-secret"}))
	if _, ok := buildEmployeeRetainItem(agent, EmployeeMemoryRetainPayload{AgentID: agentID, SandboxID: sandboxID, SessionID: "S1"}, withSecret); ok {
		t.Fatal("secret-looking transcript should not retain")
	}
}

func TestBuildEmployeeRetainItem_SkipsPureBanterWithoutWorkSignal(t *testing.T) {
	orgID := uuid.New()
	agentID := uuid.New()
	sandboxID := uuid.New()
	agent := &model.Agent{ID: agentID, OrgID: &orgID, Name: "Aria"}
	events := []model.EmployeeMemoryEvent{
		memoryEvent(t, orgID, agentID, sandboxID, "S1", "user.message.received", map[string]any{
			"source": "slack", "channel": "C123", "user_display_name": "Kim",
			"text": "Why did the database admin leave the party? Too many relationships.",
		}),
		memoryEvent(t, orgID, agentID, sandboxID, "S1", "agent.message.sent", map[string]any{
			"source": "slack", "text": "Painfully relational.",
		}),
	}
	if _, ok := buildEmployeeRetainItem(agent, EmployeeMemoryRetainPayload{AgentID: agentID, SandboxID: sandboxID, SessionID: "S1"}, events); ok {
		t.Fatal("pure banter without a work/tool signal should not retain")
	}
}

func TestBuildEmployeeRetainItem_PreservesExplicitRememberFactsWithoutTools(t *testing.T) {
	orgID := uuid.New()
	teamID := uuid.New()
	agentID := uuid.New()
	sandboxID := uuid.New()
	agent := &model.Agent{ID: agentID, OrgID: &orgID, TeamID: &teamID, Name: "Aria"}
	events := []model.EmployeeMemoryEvent{
		memoryEvent(t, orgID, agentID, sandboxID, "S1", "user.message.received", map[string]any{
			"source": "slack", "channel": "C123", "user_display_name": "Nora",
			"text": "Please remember this: Nora owns invoice-failure alerts, and billing answers must use live data when possible.",
		}),
		memoryEvent(t, orgID, agentID, sandboxID, "S1", "tool.invoked", map[string]any{
			"source": "slack", "tool": "bash", "result_summary": "Queried invoices table and found alert owner metadata.",
		}),
		memoryEvent(t, orgID, agentID, sandboxID, "S1", "agent.message.sent", map[string]any{
			"source": "slack", "text": "Remembered. Nora owns invoice-failure alerts, and billing answers should use live data when possible.",
		}),
	}

	item, ok := buildEmployeeRetainItem(agent, EmployeeMemoryRetainPayload{
		AgentID: agentID, SandboxID: sandboxID, SessionID: "S1", SourceEvent: "agent.message.sent",
	}, events)
	if !ok {
		t.Fatal("expected retain item")
	}
	for _, want := range []string{"Nora owns invoice-failure alerts", "billing answers must use live data", "Employee Aria"} {
		if !strings.Contains(item.Content, want) {
			t.Fatalf("retain content missing %q: %s", want, item.Content)
		}
	}
	if strings.Contains(item.Content, "Queried invoices") || strings.Contains(item.Content, "Tool ") || strings.Contains(item.Content, "bash") {
		t.Fatalf("retain content leaked tool execution trace: %s", item.Content)
	}
}

func TestEmployeeMemoryRetainHandler_CallsHindsight(t *testing.T) {
	var retained hindsight.RetainRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/config"):
			w.WriteHeader(http.StatusOK)
		case strings.HasSuffix(r.URL.Path, "/memories"):
			if err := json.NewDecoder(r.Body).Decode(&retained); err != nil {
				t.Fatalf("decode retain: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "items_count": 1, "async": true, "operation_id": "retain-op-1"})
		default:
			t.Fatalf("unexpected hindsight path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	orgID := uuid.New()
	agentID := uuid.New()
	sandboxID := uuid.New()
	db := openTasksMemoryTestDB(t)
	agent := model.Agent{ID: agentID, OrgID: &orgID, Name: "Aria", IsEmployee: true}
	if err := db.Create(&model.Org{ID: orgID, Name: "mem-org-" + uuid.NewString()[:8], Active: true}).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := db.Create(&model.Sandbox{ID: sandboxID, OrgID: &orgID, AgentID: &agentID, ExternalID: "sb", BridgeURL: "http://bridge", EncryptedBridgeAPIKey: []byte("x"), Status: "running"}).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	for _, event := range []model.EmployeeMemoryEvent{
		memoryEvent(t, orgID, agentID, sandboxID, "S1", "user.message.received", map[string]any{"source": "slack", "text": "We require rollback notes."}),
		memoryEvent(t, orgID, agentID, sandboxID, "S1", "tool.invoked", map[string]any{"source": "slack", "tool": "memory_retain", "result_summary": "retained deployment policy"}),
		memoryEvent(t, orgID, agentID, sandboxID, "S1", "agent.message.sent", map[string]any{"source": "slack", "text": "Done."}),
	} {
		if err := db.Create(&event).Error; err != nil {
			t.Fatalf("create event: %v", err)
		}
	}

	enq := &enqueue.MockClient{}
	handler := NewEmployeeMemoryRetainHandler(db, hindsight.NewClient(srv.URL), enq)
	task, err := NewEmployeeMemoryRetainTask(EmployeeMemoryRetainPayload{AgentID: agentID, SandboxID: sandboxID, SessionID: "S1"})
	if err != nil {
		t.Fatalf("task: %v", err)
	}
	if err := handler.Handle(context.Background(), task); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(retained.Items) != 1 || !retained.Async {
		t.Fatalf("unexpected retain request: %#v", retained)
	}
	var count int64
	if err := db.Model(&model.EmployeeMemoryEvent{}).
		Where("agent_id = ? AND sandbox_id = ? AND session_id = ? AND retained_at IS NOT NULL", agentID, sandboxID, "S1").
		Count(&count).Error; err != nil {
		t.Fatalf("count retained: %v", err)
	}
	if count != 3 {
		t.Fatalf("retained event count = %d", count)
	}
	enq.AssertEnqueued(t, TypeEmployeeMemoryRefresh)
	var refreshedAgent model.Agent
	if err := db.First(&refreshedAgent, "id = ?", agentID).Error; err != nil {
		t.Fatalf("load agent: %v", err)
	}
	if refreshedAgent.MemoryRefreshStatus != "queued" {
		t.Fatalf("memory refresh status = %q", refreshedAgent.MemoryRefreshStatus)
	}
	if refreshedAgent.MemoryRefreshError != "" {
		t.Fatalf("memory refresh error = %q", refreshedAgent.MemoryRefreshError)
	}
}

func TestEmployeeMemoryRefreshHandler_TracksSuccessAndFailure(t *testing.T) {
	orgID := uuid.New()
	agentID := uuid.New()
	sandboxID := uuid.New()
	db := openTasksMemoryTestDB(t)
	if err := db.Create(&model.Org{ID: orgID, Name: "refresh-org-" + uuid.NewString()[:8], Active: true}).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	agent := model.Agent{ID: agentID, OrgID: &orgID, Name: "Aria", IsEmployee: true, Status: "active", Model: employeeruntime.DefaultEmployeeModel}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	encKey := testTasksEncKey(t)
	encryptedSecret, err := encKey.EncryptString("runtime-secret")
	if err != nil {
		t.Fatalf("encrypt secret: %v", err)
	}
	var configCalls int
	bridge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz", "/readyz":
			w.WriteHeader(http.StatusOK)
		case "/config":
			configCalls++
			if r.Header.Get("Authorization") != "Bearer runtime-secret" {
				t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"applied":1}`))
		default:
			t.Fatalf("unexpected bridge path: %s", r.URL.Path)
		}
	}))
	defer bridge.Close()
	if err := db.Create(&model.Sandbox{ID: sandboxID, OrgID: &orgID, AgentID: &agentID, ExternalID: "sb", BridgeURL: bridge.URL, EncryptedBridgeAPIKey: encryptedSecret, Status: "running"}).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}

	handler := NewEmployeeMemoryRefreshHandler(db, employeeruntime.CompileDeps{
		DB:     db,
		EncKey: encKey,
		Cfg:    &config.Config{},
	})
	task, err := NewEmployeeMemoryRefreshTask(EmployeeMemoryRefreshPayload{AgentID: agentID, SandboxID: sandboxID, Reason: "test"})
	if err != nil {
		t.Fatalf("task: %v", err)
	}
	if err := handler.Handle(context.Background(), task); err != nil {
		t.Fatalf("refresh success: %v", err)
	}
	var updated model.Agent
	if err := db.First(&updated, "id = ?", agentID).Error; err != nil {
		t.Fatalf("load updated agent: %v", err)
	}
	if updated.MemoryRefreshStatus != "succeeded" || updated.LastMemoryRefreshedAt == nil || updated.MemoryRefreshError != "" {
		t.Fatalf("unexpected success memory status: status=%q refreshed=%v error=%q", updated.MemoryRefreshStatus, updated.LastMemoryRefreshedAt, updated.MemoryRefreshError)
	}
	if configCalls != 1 {
		t.Fatalf("config calls = %d", configCalls)
	}

	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bridge down", http.StatusBadGateway)
	}))
	defer failing.Close()
	if err := db.Model(&model.Sandbox{}).Where("id = ?", sandboxID).Update("bridge_url", failing.URL).Error; err != nil {
		t.Fatalf("update sandbox bridge url: %v", err)
	}
	if err := handler.Handle(context.Background(), task); err == nil {
		t.Fatal("expected refresh failure")
	}
	if err := db.First(&updated, "id = ?", agentID).Error; err != nil {
		t.Fatalf("reload updated agent: %v", err)
	}
	if updated.MemoryRefreshStatus != "failed" || updated.MemoryRefreshError == "" {
		t.Fatalf("unexpected failure memory status: status=%q error=%q", updated.MemoryRefreshStatus, updated.MemoryRefreshError)
	}
}

func memoryEvent(t *testing.T, orgID, agentID, sandboxID uuid.UUID, sessionID, eventType string, payload map[string]any) model.EmployeeMemoryEvent {
	t.Helper()
	payload["session_id"] = sessionID
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return model.EmployeeMemoryEvent{
		ID:        uuid.New(),
		OrgID:     orgID,
		AgentID:   agentID,
		SandboxID: sandboxID,
		SessionID: sessionID,
		EventType: eventType,
		Source:    "slack",
		Payload:   model.RawJSON(raw),
		EventAt:   time.Now().UTC(),
	}
}

func hasTaskString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func openTasksMemoryTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://hiveloop:localdev@localhost:5433/hiveloop_test?sslmode=disable" // #nosec G101 -- local test fixture
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(3)
	sqlDB.SetMaxIdleConns(1)
	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("ping postgres: %v", err)
	}
	if err := model.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func testTasksEncKey(t *testing.T) *crypto.SymmetricKey {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 31)
	}
	sk, err := crypto.NewSymmetricKey(base64.StdEncoding.EncodeToString(key))
	if err != nil {
		t.Fatalf("new symmetric key: %v", err)
	}
	return sk
}
