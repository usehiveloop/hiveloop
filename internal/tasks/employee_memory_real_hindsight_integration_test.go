package tasks

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/hindsight"
	"github.com/usehivy/hivy/internal/model"
)

func TestRealHindsightEmployeeMemoryCheckpointFlow(t *testing.T) {
	baseURL := os.Getenv("HIVY_HINDSIGHT_API_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8888"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	db := openTasksMemoryTestDB(t)
	orgID := uuid.New()
	agentID := uuid.New()
	sandboxID := uuid.New()
	sessionID := "slack-C123-" + uuid.NewString()
	marker := "marker-" + uuid.NewString()

	if err := db.Create(&model.Org{ID: orgID, Name: "real-memory-" + uuid.NewString()[:8], Active: true}).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	agent := model.Employee{ID: agentID, OrgID: &orgID, Name: "Aria", IsEmployee: true, Status: "active"}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := db.Create(&model.Sandbox{ID: sandboxID, OrgID: &orgID, EmployeeID: &agentID, ExternalID: "real-hindsight-sb", RuntimeURL: "http://runtime", EncryptedRuntimeSecret: []byte("x"), Status: "running"}).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	createMemorySession(t, db, orgID, agentID, sandboxID, sessionID)

	seededEvents := []model.EmployeeSessionEvent{
		memoryEvent(t, orgID, agentID, sandboxID, sessionID, "user.message.received", map[string]any{
			"source": "slack", "channel": "C123", "thread_ts": "1770000000.000001", "user_display_name": "Kim",
			"text": "For every production deploy, the Platform team must include rollback notes before merging. " + marker,
		}),
		memoryEvent(t, orgID, agentID, sandboxID, sessionID, "tool.invoked", map[string]any{
			"source": "slack", "tool": "read_file", "result_summary": "Confirmed the deploy checklist currently lacks rollback-note enforcement.",
		}),
		memoryEvent(t, orgID, agentID, sandboxID, sessionID, "agent.message.sent", map[string]any{
			"source": "slack", "text": "Done. I will treat rollback notes as required before production deploys.",
		}),
	}
	for _, event := range seededEvents {
		event.EventAt = time.Now().UTC().Add(-5 * time.Minute)
		if err := db.Create(&event).Error; err != nil {
			t.Fatalf("create memory event: %v", err)
		}
	}

	itemPreview, ok := buildEmployeeRetainItem(&agent, EmployeeMemoryRetainPayload{
		EmployeeID: agentID, SandboxID: sandboxID, SessionID: sessionID, SourceEvent: "agent.message.sent",
	}, seededEvents)
	if !ok {
		t.Fatal("expected retain item preview")
	}
	t.Logf("retained content:\n%s", itemPreview.Content)
	t.Logf("retained tags: %v", itemPreview.Tags)
	t.Logf("retained metadata: %s", mustJSONForLog(itemPreview.Metadata))

	client := hindsight.NewClient(baseURL)
	if err := client.ConfigureBank(ctx, hindsight.OrgBankID(orgID), hindsight.DefaultMemoryConfig().ToBankConfigUpdate()); err != nil {
		t.Fatalf("configure bank: %v", err)
	}
	handler := NewEmployeeMemoryRetainHandler(db, client, nil)
	task, err := NewEmployeeMemoryRetainTask(EmployeeMemoryRetainPayload{
		EmployeeID:  agentID,
		SandboxID:   sandboxID,
		SessionID:   sessionID,
		Reason:      "real_hindsight_test",
		SourceEvent: "agent.message.sent",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := handler.Handle(ctx, task); err != nil {
		t.Fatalf("retain handler: %v", err)
	}

	var retainedCount int64
	if err := db.Model(&model.EmployeeSessionEvent{}).
		Where("employee_id = ? AND sandbox_id = ? AND runtime_session_id = ? AND retained_at IS NOT NULL", agentID, sandboxID, sessionID).
		Count(&retainedCount).Error; err != nil {
		t.Fatalf("count retained events: %v", err)
	}
	if retainedCount != int64(len(seededEvents)) {
		t.Fatalf("retained count = %d, want %d", retainedCount, len(seededEvents))
	}

	if itemPreview.DocumentID != "employee-session:"+sandboxID.String()+":"+sessionID {
		t.Fatalf("preview document id = %q", itemPreview.DocumentID)
	}
}

func TestRealHindsightEmployeeMemoryProductionWorkload(t *testing.T) {
	baseURL := os.Getenv("HIVY_HINDSIGHT_API_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8888"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	db := openTasksMemoryTestDB(t)
	orgID := uuid.New()
	agentID := uuid.New()
	sandboxID := uuid.New()
	if err := db.Create(&model.Org{ID: orgID, Name: "prod-memory-" + uuid.NewString()[:8], Active: true}).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	agent := model.Employee{ID: agentID, OrgID: &orgID, Name: "Aria", IsEmployee: true, Status: "active"}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := db.Create(&model.Sandbox{ID: sandboxID, OrgID: &orgID, EmployeeID: &agentID, ExternalID: "production-workload-sb", RuntimeURL: "http://runtime", EncryptedRuntimeSecret: []byte("x"), Status: "running"}).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}

	sessions := productionMemorySessions()
	var inserted int
	var expectedRetained int
	for i, session := range sessions {
		sessionID := "slack-C123-prod-" + uuid.NewString()
		createMemorySession(t, db, orgID, agentID, sandboxID, sessionID)
		events := session.toEvents(t, orgID, agentID, sandboxID, sessionID, i)
		if _, ok := buildEmployeeRetainItem(&agent, EmployeeMemoryRetainPayload{
			EmployeeID: agentID, SandboxID: sandboxID, SessionID: sessionID, SourceEvent: "agent.message.sent",
		}, events); ok {
			expectedRetained += len(events)
		}
		for _, event := range events {
			event.EventAt = time.Now().UTC().Add(-5 * time.Minute)
			if err := db.Create(&event).Error; err != nil {
				t.Fatalf("create event: %v", err)
			}
			inserted++
		}
	}
	if inserted < 100 {
		t.Fatalf("seeded %d events, want at least 100", inserted)
	}

	client := hindsight.NewClient(baseURL)
	if err := client.ConfigureBank(ctx, hindsight.OrgBankID(orgID), hindsight.DefaultMemoryConfig().ToBankConfigUpdate()); err != nil {
		t.Fatalf("configure bank: %v", err)
	}
	handler := NewEmployeeMemoryRetainHandler(db, client, nil)
	for _, sessionID := range distinctProductionSessionIDs(t, db, agentID, sandboxID) {
		task, err := NewEmployeeMemoryRetainTask(EmployeeMemoryRetainPayload{
			EmployeeID:  agentID,
			SandboxID:   sandboxID,
			SessionID:   sessionID,
			Reason:      "production_workload_test",
			SourceEvent: "agent.message.sent",
		})
		if err != nil {
			t.Fatalf("task: %v", err)
		}
		if err := handler.Handle(ctx, task); err != nil {
			t.Fatalf("retain session %s: %v", sessionID, err)
		}
	}
	var retainedCount int64
	if err := db.Model(&model.EmployeeSessionEvent{}).
		Where("employee_id = ? AND sandbox_id = ? AND retained_at IS NOT NULL", agentID, sandboxID).
		Count(&retainedCount).Error; err != nil {
		t.Fatalf("count retained: %v", err)
	}
	if retainedCount != int64(expectedRetained) {
		t.Fatalf("retained %d events, want %d", retainedCount, expectedRetained)
	}
	if expectedRetained <= 0 {
		t.Fatalf("expected some production workload events to be retained")
	}
}
