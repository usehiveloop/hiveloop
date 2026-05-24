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
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/hindsight"
	"github.com/usehivy/hivy/internal/model"
)

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
	agent := model.Employee{ID: agentID, OrgID: &orgID, Name: "Aria", IsEmployee: true}
	if err := db.Create(&model.Org{ID: orgID, Name: "mem-org-" + uuid.NewString()[:8], Active: true}).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := db.Create(&model.Sandbox{ID: sandboxID, OrgID: &orgID, EmployeeID: &agentID, ExternalID: "sb", BridgeURL: "http://bridge", EncryptedBridgeAPIKey: []byte("x"), Status: "running"}).Error; err != nil {
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
	task, err := NewEmployeeMemoryRetainTask(EmployeeMemoryRetainPayload{EmployeeID: agentID, SandboxID: sandboxID, SessionID: "S1"})
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
		Where("employee_id = ? AND sandbox_id = ? AND session_id = ? AND retained_at IS NOT NULL", agentID, sandboxID, "S1").
		Count(&count).Error; err != nil {
		t.Fatalf("count retained: %v", err)
	}
	if count != 3 {
		t.Fatalf("retained event count = %d", count)
	}
	enq.AssertEnqueued(t, TypeEmployeeMemoryRefresh)
	var refreshedAgent model.Employee
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
	agent := model.Employee{ID: agentID, OrgID: &orgID, Name: "Aria", IsEmployee: true, Status: "active", Model: employeeruntime.DefaultEmployeeModel}
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
	if err := db.Create(&model.Sandbox{ID: sandboxID, OrgID: &orgID, EmployeeID: &agentID, ExternalID: "sb", BridgeURL: bridge.URL, EncryptedBridgeAPIKey: encryptedSecret, Status: "running"}).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}

	handler := NewEmployeeMemoryRefreshHandler(db, employeeruntime.CompileDeps{
		DB:     db,
		EncKey: encKey,
		Cfg:    &config.Config{},
	})
	task, err := NewEmployeeMemoryRefreshTask(EmployeeMemoryRefreshPayload{EmployeeID: agentID, SandboxID: sandboxID, Reason: "test"})
	if err != nil {
		t.Fatalf("task: %v", err)
	}
	if err := handler.Handle(context.Background(), task); err != nil {
		t.Fatalf("refresh success: %v", err)
	}
	var updated model.Employee
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
