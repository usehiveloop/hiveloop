package handler

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
	skillpkg "github.com/usehiveloop/hiveloop/internal/skills"
)

func TestEmployeeOutboundMemoryCheckpoints(t *testing.T) {
	if shouldTriggerEmployeeMemoryCheckpoint("user.message.received") {
		t.Fatal("user message alone should not trigger retain")
	}
	if !shouldTriggerEmployeeMemoryCheckpoint("agent.message.sent") {
		t.Fatal("agent message should trigger retain checkpoint")
	}
	if shouldTriggerEmployeeMemoryCheckpoint("agent.stream.token") {
		t.Fatal("stream tokens should be stored but should not trigger retain")
	}
	if shouldTriggerEmployeeMemoryCheckpoint("error.model") {
		t.Fatal("errors should be stored but should not trigger retain")
	}
}

func TestEmployeeMemoryEventFromOutbound_StoresAllEventTypes(t *testing.T) {
	orgID := uuid.New()
	agentID := uuid.New()
	sandboxID := uuid.New()
	eventAt := time.Date(2026, 5, 13, 9, 30, 0, 0, time.UTC)
	sb := &model.Sandbox{ID: sandboxID, OrgID: &orgID, AgentID: &agentID}
	for _, eventType := range []string{
		"user.message.received",
		"agent.stream.token",
		"agent.stream.thinking",
		"agent.tool.call",
		"agent.tool.result",
		"agent.final_message",
		"agent.run.turn_completed",
		"error.model",
	} {
		payload := map[string]any{
			"session_id": "slack-session-1",
			"source":     "slack",
			"text":       "api_key=sk-secret should still be persisted for session sync",
		}
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
		stored, ok := employeeMemoryEventFromOutbound(sb, &employeeOutboundEvent{
			EventType: eventType,
			Payload:   body,
			At:        eventAt,
		}, payload, "slack-session-1")
		if !ok {
			t.Fatalf("%s was not stored", eventType)
		}
		if stored.EventType != eventType || stored.SessionID != "slack-session-1" || stored.Source != "slack" {
			t.Fatalf("stored event mismatch: %#v", stored)
		}
	}
}

func TestEmployeeMemoryEventFromOutbound_StoresEventWithoutSessionID(t *testing.T) {
	orgID := uuid.New()
	agentID := uuid.New()
	sandboxID := uuid.New()
	sb := &model.Sandbox{ID: sandboxID, OrgID: &orgID, AgentID: &agentID}
	payload := map[string]any{"source": "system"}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	stored, ok := employeeMemoryEventFromOutbound(sb, &employeeOutboundEvent{
		EventType: "config.applied",
		Payload:   body,
		At:        time.Now().UTC(),
	}, payload, "")
	if !ok {
		t.Fatal("event without session id should still be stored")
	}
	if stored.SessionID != "" || stored.EventType != "config.applied" {
		t.Fatalf("stored event mismatch: %#v", stored)
	}
}

func TestEmployeeEventSource_SanitizesFutureGateways(t *testing.T) {
	source := employeeEventSource(map[string]any{"source": "WhatsApp Business"})
	if source != "whatsapp-business" {
		t.Fatalf("source = %q", source)
	}
	if employeeEventSource(map[string]any{}) != "manual" {
		t.Fatal("missing source should fall back to manual")
	}
}

func TestPayloadLooksSensitive(t *testing.T) {
	if !payloadLooksSensitive(map[string]any{"text": "api_key=sk-secret"}) {
		t.Fatal("expected secret-looking payload to be rejected")
	}
	if payloadLooksSensitive(map[string]any{"text": "The team requires rollback notes."}) {
		t.Fatal("ordinary payload should not be rejected")
	}
}

func TestIntegration_EmployeeSkillSync_UpsertsVersionsAndAttachesAgent(t *testing.T) {
	db := connectEmployeeSkillSyncTestDB(t)
	org1 := model.Org{Name: "skill-sync-" + uuid.NewString()}
	org2 := model.Org{Name: "skill-sync-" + uuid.NewString()}
	if err := db.Create(&org1).Error; err != nil {
		t.Fatalf("create org1: %v", err)
	}
	if err := db.Create(&org2).Error; err != nil {
		t.Fatalf("create org2: %v", err)
	}
	t.Cleanup(func() {
		db.Where("id IN ?", []uuid.UUID{org1.ID, org2.ID}).Delete(&model.Org{})
	})
	agent1 := model.Agent{OrgID: &org1.ID, Name: "Aria", Model: "test"}
	agent2 := model.Agent{OrgID: &org2.ID, Name: "Aria", Model: "test"}
	if err := db.Create(&agent1).Error; err != nil {
		t.Fatalf("create agent1: %v", err)
	}
	if err := db.Create(&agent2).Error; err != nil {
		t.Fatalf("create agent2: %v", err)
	}
	global := model.Skill{Slug: "debug-deploys", Name: "debug-deploys", SourceType: model.SkillSourceInline, RepoRef: "main", Status: model.SkillStatusPublished}
	if err := db.Create(&global).Error; err != nil {
		t.Fatalf("create global skill with same slug: %v", err)
	}
	t.Cleanup(func() {
		db.Unscoped().Delete(&global)
	})

	h := NewEmployeeOutboundWebhookHandler(db, nil, nil)
	sb1 := &model.Sandbox{ID: uuid.New(), OrgID: &org1.ID, AgentID: &agent1.ID}
	sb2 := &model.Sandbox{ID: uuid.New(), OrgID: &org2.ID, AgentID: &agent2.ID}
	payload := map[string]any{
		"action":      "create",
		"name":        "debug-deploys",
		"description": "Debug deploy failures.",
		"tags":        []string{"deploy", "debug"},
		"content":     "---\nname: debug-deploys\ndescription: Debug deploy failures.\n---\n\n# Debug\nCheck logs first.",
		"files":       map[string]string{"references/errors.md": "# Errors"},
	}
	if err := h.syncSkillEvent(t.Context(), sb1, payload); err != nil {
		t.Fatalf("sync skill org1: %v", err)
	}
	var skill model.Skill
	if err := db.Where("org_id = ? AND slug = ?", org1.ID, "debug-deploys").First(&skill).Error; err != nil {
		t.Fatalf("load org skill: %v", err)
	}
	if skill.Status != model.SkillStatusPublished {
		t.Fatalf("status = %q", skill.Status)
	}
	var links int64
	db.Model(&model.AgentSkill{}).Where("agent_id = ? AND skill_id = ?", agent1.ID, skill.ID).Count(&links)
	if links != 1 {
		t.Fatalf("agent skill links = %d", links)
	}
	var version model.SkillVersion
	if err := db.First(&version, "id = ?", *skill.LatestVersionID).Error; err != nil {
		t.Fatalf("load latest version: %v", err)
	}
	var bundle skillpkg.Bundle
	if err := json.Unmarshal(version.Bundle, &bundle); err != nil {
		t.Fatalf("decode bundle: %v", err)
	}
	if bundle.Content != "\n# Debug\nCheck logs first." || bundle.Files["references/errors.md"] != "# Errors" {
		t.Fatalf("unexpected bundle: %#v", bundle)
	}

	payload["action"] = "patch"
	payload["content"] = "---\nname: debug-deploys\n---\n\n# Debug\nCheck deployment logs first."
	if err := h.syncSkillEvent(t.Context(), sb1, payload); err != nil {
		t.Fatalf("sync update: %v", err)
	}
	var versionCount int64
	db.Model(&model.SkillVersion{}).Where("skill_id = ?", skill.ID).Count(&versionCount)
	if versionCount != 2 {
		t.Fatalf("version count = %d", versionCount)
	}
	if err := h.syncSkillEvent(t.Context(), sb2, payload); err != nil {
		t.Fatalf("same slug in second org should be allowed: %v", err)
	}
	var org2Skill model.Skill
	if err := db.Where("org_id = ? AND slug = ?", org2.ID, "debug-deploys").First(&org2Skill).Error; err != nil {
		t.Fatalf("load org2 skill: %v", err)
	}
	if org2Skill.ID == skill.ID {
		t.Fatal("org2 skill reused org1 skill")
	}

	if err := h.syncSkillEvent(t.Context(), sb1, map[string]any{"action": "delete", "name": "debug-deploys", "deleted": true}); err != nil {
		t.Fatalf("sync delete: %v", err)
	}
	db.Model(&model.AgentSkill{}).Where("agent_id = ? AND skill_id = ?", agent1.ID, skill.ID).Count(&links)
	if links != 0 {
		t.Fatalf("agent link should be detached, got %d", links)
	}
	if err := db.First(&model.Skill{}, "id = ?", skill.ID).Error; err != nil {
		t.Fatalf("skill should remain after detach-only delete: %v", err)
	}
}

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
	sb := model.Sandbox{
		OrgID:                 &org.ID,
		AgentID:               &agent.ID,
		ExternalID:            "sandbox-" + uuid.NewString(),
		BridgeURL:             "https://bridge.test",
		EncryptedBridgeAPIKey: []byte("secret"),
		Status:                "running",
	}
	if err := db.Create(&sb).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	t.Cleanup(func() {
		db.Where("id = ?", org.ID).Delete(&model.Org{})
	})

	h := NewEmployeeOutboundWebhookHandler(db, nil, nil)
	now := time.Date(2026, 5, 13, 10, 0, 0, 0, time.UTC)
	payload := map[string]any{
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

	var eventCount int64
	db.Model(&model.EmployeeMemoryEvent{}).Where("agent_id = ? AND event_type LIKE ?", agent.ID, "schedule.%").Count(&eventCount)
	if eventCount != 6 {
		t.Fatalf("schedule event count = %d", eventCount)
	}
	var schedule model.EmployeeSchedule
	if err := db.Where("agent_id = ? AND bridge_job_id = ?", agent.ID, "cron-1").First(&schedule).Error; err != nil {
		t.Fatalf("load schedule: %v", err)
	}
	if schedule.Status != "cancelled" || schedule.CancelledAt == nil {
		t.Fatalf("schedule final state = %#v", schedule)
	}
	if schedule.RepeatCompleted != 1 || schedule.LastStatus != "completed" {
		t.Fatalf("schedule run fields = %#v", schedule)
	}
	var scheduleCount int64
	db.Model(&model.EmployeeSchedule{}).Where("agent_id = ? AND bridge_job_id = ?", agent.ID, "cron-1").Count(&scheduleCount)
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

	agent2 := model.Agent{OrgID: &org.ID, Name: "Mira", Model: "test"}
	if err := db.Create(&agent2).Error; err != nil {
		t.Fatalf("create agent2: %v", err)
	}
	sb2 := model.Sandbox{
		OrgID:                 &org.ID,
		AgentID:               &agent2.ID,
		ExternalID:            "sandbox-" + uuid.NewString(),
		BridgeURL:             "https://bridge.test",
		EncryptedBridgeAPIKey: []byte("secret"),
		Status:                "running",
	}
	if err := db.Create(&sb2).Error; err != nil {
		t.Fatalf("create sandbox2: %v", err)
	}
	h.storeAndMaybeEnqueue(t.Context(), &sb2, outboundTestEvent(t, "schedule.created", now, payload))
	db.Model(&model.EmployeeSchedule{}).Where("bridge_job_id = ?", "cron-1").Count(&scheduleCount)
	if scheduleCount != 2 {
		t.Fatalf("same bridge job id for different agents should create separate schedules, got %d", scheduleCount)
	}
}

func TestIntegration_EmployeeScheduleMalformedPayload_StoresEventOnly(t *testing.T) {
	db := connectEmployeeSkillSyncTestDB(t)
	org := model.Org{Name: "schedule-malformed-" + uuid.NewString()}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	agent := model.Agent{OrgID: &org.ID, Name: "Aria", Model: "test"}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	sb := model.Sandbox{
		OrgID:                 &org.ID,
		AgentID:               &agent.ID,
		ExternalID:            "sandbox-" + uuid.NewString(),
		BridgeURL:             "https://bridge.test",
		EncryptedBridgeAPIKey: []byte("secret"),
		Status:                "running",
	}
	if err := db.Create(&sb).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", org.ID).Delete(&model.Org{}) })

	h := NewEmployeeOutboundWebhookHandler(db, nil, nil)
	h.storeAndMaybeEnqueue(t.Context(), &sb, outboundTestEvent(t, "schedule.created", time.Now().UTC(), map[string]any{"session_id": "C123-456.789"}))

	var eventCount int64
	db.Model(&model.EmployeeMemoryEvent{}).Where("agent_id = ? AND event_type = ?", agent.ID, "schedule.created").Count(&eventCount)
	if eventCount != 1 {
		t.Fatalf("event count = %d", eventCount)
	}
	var scheduleCount int64
	db.Model(&model.EmployeeSchedule{}).Where("agent_id = ?", agent.ID).Count(&scheduleCount)
	if scheduleCount != 0 {
		t.Fatalf("schedule count = %d", scheduleCount)
	}
}

func TestIntegration_EmployeeScheduleNonCronSource_StoresEventOnly(t *testing.T) {
	db := connectEmployeeSkillSyncTestDB(t)
	org := model.Org{Name: "schedule-non-cron-" + uuid.NewString()}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	agent := model.Agent{OrgID: &org.ID, Name: "Aria", Model: "test"}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	sb := model.Sandbox{
		OrgID:                 &org.ID,
		AgentID:               &agent.ID,
		ExternalID:            "sandbox-" + uuid.NewString(),
		BridgeURL:             "https://bridge.test",
		EncryptedBridgeAPIKey: []byte("secret"),
		Status:                "running",
	}
	if err := db.Create(&sb).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", org.ID).Delete(&model.Org{}) })

	h := NewEmployeeOutboundWebhookHandler(db, nil, nil)
	h.storeAndMaybeEnqueue(t.Context(), &sb, outboundTestEvent(t, "schedule.created", time.Now().UTC(), map[string]any{
		"session_id": "C123-456.789",
		"job_id":     "delegate-1",
		"source":     "delegate",
	}))

	var eventCount int64
	db.Model(&model.EmployeeMemoryEvent{}).Where("agent_id = ? AND event_type = ?", agent.ID, "schedule.created").Count(&eventCount)
	if eventCount != 1 {
		t.Fatalf("event count = %d", eventCount)
	}
	var scheduleCount int64
	db.Model(&model.EmployeeSchedule{}).Where("agent_id = ?", agent.ID).Count(&scheduleCount)
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

func connectEmployeeSkillSyncTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://hiveloop:localdev@localhost:5433/hiveloop_test?sslmode=disable"
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(3)
	sqlDB.SetMaxIdleConns(1)
	if err := sqlDB.Ping(); err != nil {
		sqlDB.Close()
		t.Skipf("postgres unavailable: %v", err)
	}
	if err := model.AutoMigrate(db); err != nil {
		sqlDB.Close()
		t.Fatalf("automigrate: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	return db
}
