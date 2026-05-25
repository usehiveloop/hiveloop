package specialisttasks

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
)

func TestSpecialistStatusToolReturnsCompactText(t *testing.T) {
	db := connectSpecialistTasksTestDB(t)
	catalog := specialistTestCatalog(t)
	org, employee, token := createSpecialistToolScope(t, db)
	t.Cleanup(func() { db.Where("id = ?", org.ID).Delete(&model.Org{}) })

	sb := model.Sandbox{
		ID:                    uuid.New(),
		OrgID:                 &org.ID,
		EmployeeID:            &employee.ID,
		ExternalID:            "specialist-status-sandbox",
		BridgeURL:             "http://localhost:7080",
		EncryptedBridgeAPIKey: []byte("encrypted"),
		Status:                "running",
	}
	if err := db.Create(&sb).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	session := model.EmployeeSession{
		ID:                    uuid.New(),
		OrgID:                 org.ID,
		EmployeeID:            employee.ID,
		SandboxID:             sb.ID,
		RuntimeConversationID: "runtime-session-1",
		Source:                "test",
		Status:                "active",
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("create employee session: %v", err)
	}
	task := model.SpecialistTask{
		ID:                     uuid.New(),
		OrgID:                  org.ID,
		EmployeeID:             employee.ID,
		SpecialistSlug:         "software-engineering-specialist",
		EmployeeSessionID:      session.RuntimeConversationID,
		SandboxID:              sb.ID,
		ParentConversationType: "employee_session",
		ParentConversationID:   session.RuntimeConversationID,
		Brief:                  "Say hello.",
		Status:                 "running",
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatalf("create specialist task: %v", err)
	}
	seedSpecialistStatusEvents(t, db, org.ID, employee.ID, sb.ID, session, task)

	service := NewService(db, &sandbox.Orchestrator{}, employeeruntimeCompileDepsForTest(), catalog)
	server := mcp.NewServer(&mcp.Implementation{Name: "specialist-test", Version: "v1"}, nil)
	NewToolsFunc(service)(server, &token)
	mcpSession, cleanup := connectSpecialistMCPTestSession(t, server)
	defer cleanup()

	result, err := mcpSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "specialist_task_status",
		Arguments: map[string]any{"task_id": task.ID.String()},
	})
	if err != nil {
		t.Fatalf("call specialist_task_status: %v", err)
	}
	if result.IsError {
		t.Fatalf("specialist_task_status returned error: %s", specialistToolText(t, result))
	}
	text := specialistToolText(t, result)
	for _, want := range []string{"Status: running", "Recent activity: 1 message(s), 1 tool call(s)", "Latest specialist message: Hello, I am the software engineering specialist."} {
		if !strings.Contains(text, want) {
			t.Fatalf("status text missing %q:\n%s", want, text)
		}
	}
	for _, forbidden := range []string{"recent_events", `"payload"`, `"event_type"`, "echo hello"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("status text leaked raw event detail %q:\n%s", forbidden, text)
		}
	}
}

func seedSpecialistStatusEvents(t *testing.T, db *gorm.DB, orgID, employeeID, sandboxID uuid.UUID, session model.EmployeeSession, task model.SpecialistTask) {
	t.Helper()
	now := time.Now().UTC()
	events := []model.EmployeeSessionEvent{
		specialistStatusEvent(orgID, employeeID, sandboxID, session, task, "agent.tool.call", model.RawJSON(`{"tool":"shell","args":{"cmd":"echo hello"}}`), now.Add(-2*time.Minute)),
		specialistStatusEvent(orgID, employeeID, sandboxID, session, task, "agent.message.sent", model.RawJSON(`{"text":"Hello, I am the software engineering specialist."}`), now.Add(-1*time.Minute)),
	}
	if err := db.Create(&events).Error; err != nil {
		t.Fatalf("create events: %v", err)
	}
}

func specialistStatusEvent(orgID, employeeID, sandboxID uuid.UUID, session model.EmployeeSession, task model.SpecialistTask, eventType string, payload model.RawJSON, eventAt time.Time) model.EmployeeSessionEvent {
	return model.EmployeeSessionEvent{
		OrgID:             orgID,
		EmployeeID:        employeeID,
		SandboxID:         sandboxID,
		EmployeeSessionID: session.ID,
		SessionID:         session.RuntimeConversationID,
		EventID:           uuid.NewString(),
		EventType:         eventType,
		Source:            "specialist",
		Mode:              "specialist",
		SpecialistSlug:    task.SpecialistSlug,
		SpecialistTaskID:  &task.ID,
		Payload:           payload,
		EventAt:           eventAt,
	}
}
