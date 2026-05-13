package handler

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
)

func TestIntegration_EmployeeScheduleEvents_UpdateSchedulesAndRuns(t *testing.T) {
	db := connectEmployeeSkillSyncTestDB(t)
	org := model.Org{Name: "schedule-sync-" + uuid.NewString()}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	agent := model.Agent{OrgID: &org.ID, Name: "Aria", Model: "test"}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	sb := employeeScheduleTestSandbox(t, db, org.ID, agent.ID)
	t.Cleanup(func() { db.Where("id = ?", org.ID).Delete(&model.Org{}) })

	h := NewEmployeeOutboundWebhookHandler(db, nil, nil)
	now := time.Date(2026, 5, 13, 10, 0, 0, 0, time.UTC)
	payload := employeeScheduleTestPayload(now)
	h.storeAndMaybeEnqueue(t.Context(), &sb, outboundTestEvent(t, "schedule.created", now, payload))
	h.storeAndMaybeEnqueue(t.Context(), &sb, outboundTestEvent(t, "schedule.paused", now.Add(time.Minute), payload))
	payload["state"] = "active"
	payload["run_key"] = "cron-1:" + now.Add(time.Hour).Format(time.RFC3339)
	payload["scheduled_at"] = now.Add(time.Hour).Format(time.RFC3339)
	payload["started_at"] = now.Add(time.Hour).Format(time.RFC3339)
	h.storeAndMaybeEnqueue(t.Context(), &sb, outboundTestEvent(t, "schedule.run_started", now.Add(time.Hour), payload))
	payload["repeat_completed"] = float64(1)
	payload["last_status"] = "completed"
	payload["completed_at"] = now.Add(time.Hour + time.Minute).Format(time.RFC3339)
	payload["duration_ms"] = float64(60000)
	h.storeAndMaybeEnqueue(t.Context(), &sb, outboundTestEvent(t, "schedule.run_completed", now.Add(time.Hour+time.Minute), payload))
	h.storeAndMaybeEnqueue(t.Context(), &sb, outboundTestEvent(t, "schedule.cancelled", now.Add(2*time.Hour), payload))
	h.storeAndMaybeEnqueue(t.Context(), &sb, outboundTestEvent(t, "schedule.cancelled", now.Add(2*time.Hour), payload))

	assertEmployeeScheduleMirror(t, db, agent.ID, payload)

	agent2 := model.Agent{OrgID: &org.ID, Name: "Mira", Model: "test"}
	if err := db.Create(&agent2).Error; err != nil {
		t.Fatalf("create agent2: %v", err)
	}
	sb2 := employeeScheduleTestSandbox(t, db, org.ID, agent2.ID)
	h.storeAndMaybeEnqueue(t.Context(), &sb2, outboundTestEvent(t, "schedule.created", now, payload))
	var scheduleCount int64
	db.Model(&model.EmployeeSchedule{}).Where("bridge_job_id = ?", "cron-1").Count(&scheduleCount)
	if scheduleCount != 2 {
		t.Fatalf("same bridge job id for different agents should create separate schedules, got %d", scheduleCount)
	}
}

func TestIntegration_EmployeeScheduleMalformedPayload_StoresEventOnly(t *testing.T) {
	db := connectEmployeeSkillSyncTestDB(t)
	org, agent, sb := employeeScheduleTestScope(t, db, "schedule-malformed-")
	t.Cleanup(func() { db.Where("id = ?", org.ID).Delete(&model.Org{}) })

	h := NewEmployeeOutboundWebhookHandler(db, nil, nil)
	h.storeAndMaybeEnqueue(t.Context(), &sb, outboundTestEvent(t, "schedule.created", time.Now().UTC(), map[string]any{"session_id": "C123-456.789"}))

	assertScheduleEventWithoutMirror(t, db, agent.ID)
}

func TestIntegration_EmployeeScheduleNonCronSource_StoresEventOnly(t *testing.T) {
	db := connectEmployeeSkillSyncTestDB(t)
	org, agent, sb := employeeScheduleTestScope(t, db, "schedule-non-cron-")
	t.Cleanup(func() { db.Where("id = ?", org.ID).Delete(&model.Org{}) })

	h := NewEmployeeOutboundWebhookHandler(db, nil, nil)
	h.storeAndMaybeEnqueue(t.Context(), &sb, outboundTestEvent(t, "schedule.created", time.Now().UTC(), map[string]any{
		"session_id": "C123-456.789",
		"job_id":     "delegate-1",
		"source":     "delegate",
	}))

	assertScheduleEventWithoutMirror(t, db, agent.ID)
}

func employeeScheduleTestScope(t *testing.T, db interface {
	Create(value any) *gorm.DB
}, orgPrefix string) (model.Org, model.Agent, model.Sandbox) {
	t.Helper()
	org := model.Org{Name: orgPrefix + uuid.NewString()}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	agent := model.Agent{OrgID: &org.ID, Name: "Aria", Model: "test"}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	return org, agent, employeeScheduleTestSandbox(t, db, org.ID, agent.ID)
}

func employeeScheduleTestSandbox(t *testing.T, db interface {
	Create(value any) *gorm.DB
}, orgID uuid.UUID, agentID uuid.UUID) model.Sandbox {
	t.Helper()
	sb := model.Sandbox{
		OrgID:                 &orgID,
		AgentID:               &agentID,
		ExternalID:            "sandbox-" + uuid.NewString(),
		BridgeURL:             "https://bridge.test",
		EncryptedBridgeAPIKey: []byte("secret"),
		Status:                "running",
	}
	if err := db.Create(&sb).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	return sb
}

func employeeScheduleTestPayload(now time.Time) map[string]any {
	return map[string]any{
		"session_id":         "C123-456.789",
		"source":             "cron",
		"job_id":             "cron-1",
		"state":              "active",
		"channel":            "C123",
		"description":        "Deploy health",
		"task_prompt":        "Check deploy health",
		"interval_seconds":   float64(3600),
		"repeat_count":       float64(5),
		"repeat_completed":   float64(0),
		"next_run_at":        now.Add(time.Hour).Format(time.RFC3339),
		"created_by_session": "C123-456.789",
		"created_at":         now.Format(time.RFC3339),
	}
}

func assertEmployeeScheduleMirror(t *testing.T, db *gorm.DB, agentID uuid.UUID, payload map[string]any) {
	t.Helper()
	var eventCount int64
	db.Model(&model.EmployeeMemoryEvent{}).Where("agent_id = ? AND event_type LIKE ?", agentID, "schedule.%").Count(&eventCount)
	if eventCount != 6 {
		t.Fatalf("schedule event count = %d", eventCount)
	}
	var schedule model.EmployeeSchedule
	if err := db.Where("agent_id = ? AND bridge_job_id = ?", agentID, "cron-1").First(&schedule).Error; err != nil {
		t.Fatalf("load schedule: %v", err)
	}
	if schedule.Status != "cancelled" || schedule.CancelledAt == nil {
		t.Fatalf("schedule final state = %#v", schedule)
	}
	if schedule.RepeatCompleted != 1 || schedule.LastStatus != "completed" {
		t.Fatalf("schedule run fields = %#v", schedule)
	}
	var scheduleCount int64
	db.Model(&model.EmployeeSchedule{}).Where("agent_id = ? AND bridge_job_id = ?", agentID, "cron-1").Count(&scheduleCount)
	if scheduleCount != 1 {
		t.Fatalf("schedule count = %d", scheduleCount)
	}
	var run model.EmployeeScheduleRun
	if err := db.Where("schedule_id = ? AND run_key = ?", schedule.ID, payload["run_key"]).First(&run).Error; err != nil {
		t.Fatalf("load run: %v", err)
	}
	if run.Status != "completed" || run.DurationMS == nil || *run.DurationMS != 60000 {
		t.Fatalf("run = %#v", run)
	}
	var runCount int64
	db.Model(&model.EmployeeScheduleRun{}).Where("schedule_id = ?", schedule.ID).Count(&runCount)
	if runCount != 1 {
		t.Fatalf("run count = %d", runCount)
	}
}

func assertScheduleEventWithoutMirror(t *testing.T, db *gorm.DB, agentID uuid.UUID) {
	t.Helper()
	var eventCount int64
	db.Model(&model.EmployeeMemoryEvent{}).Where("agent_id = ? AND event_type = ?", agentID, "schedule.created").Count(&eventCount)
	if eventCount != 1 {
		t.Fatalf("event count = %d", eventCount)
	}
	var scheduleCount int64
	db.Model(&model.EmployeeSchedule{}).Where("agent_id = ?", agentID).Count(&scheduleCount)
	if scheduleCount != 0 {
		t.Fatalf("schedule count = %d", scheduleCount)
	}
}

func outboundTestEvent(t *testing.T, eventType string, at time.Time, payload map[string]any) *employeeOutboundEvent {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return &employeeOutboundEvent{EventType: eventType, Payload: body, At: at}
}
