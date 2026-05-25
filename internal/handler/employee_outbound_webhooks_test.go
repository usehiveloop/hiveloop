package handler

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	skillpkg "github.com/usehivy/hivy/internal/skills"
	"github.com/usehivy/hivy/internal/testdb"
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

func TestShouldStoreEmployeeSessionEvent_KeepsConversationTimeline(t *testing.T) {
	for _, eventType := range []string{
		"user.message.received",
		"agent.stream.token",
		"agent.stream.thinking",
		"agent.tool.call",
		"agent.tool.result",
		"agent.message.sent",
		"error.model",
		"skill.synced",
	} {
		if !shouldStoreEmployeeSessionEvent(eventType) {
			t.Fatalf("%s should be stored", eventType)
		}
	}
	for _, eventType := range []string{
		"session.created",
		"tool.invoked",
		"agent.final_message",
		"agent.run.turn.started",
		"agent.run.model.request.started",
		"agent.run.model.usage",
		"agent.run.turn.completed",
	} {
		if shouldStoreEmployeeSessionEvent(eventType) {
			t.Fatalf("%s should be skipped", eventType)
		}
	}
}

func TestEmployeeSessionEventFromOutbound_StoresTimelineEventTypes(t *testing.T) {
	orgID := uuid.New()
	agentID := uuid.New()
	sandboxID := uuid.New()
	employeeSessionID := uuid.New()
	eventAt := time.Date(2026, 5, 13, 9, 30, 0, 0, time.UTC)
	sb := &model.Sandbox{ID: sandboxID, OrgID: &orgID, EmployeeID: &agentID}
	for _, eventType := range []string{
		"user.message.received",
		"agent.stream.token",
		"agent.stream.thinking",
		"agent.tool.call",
		"agent.tool.result",
		"agent.message.sent",
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
		stored, ok := employeeSessionEventFromOutbound(sb, &employeeOutboundEvent{
			EventType: eventType,
			Payload:   body,
			At:        eventAt,
		}, payload, employeeSessionID, "slack-session-1")
		if !ok {
			t.Fatalf("%s was not stored", eventType)
		}
		if stored.EventType != eventType || stored.EmployeeSessionID != employeeSessionID || stored.SessionID != "slack-session-1" || stored.Source != "slack" {
			t.Fatalf("stored event mismatch: %#v", stored)
		}
	}
}

func TestEmployeeSessionEventFromOutbound_StoresEventWithoutSessionID(t *testing.T) {
	orgID := uuid.New()
	agentID := uuid.New()
	sandboxID := uuid.New()
	sb := &model.Sandbox{ID: sandboxID, OrgID: &orgID, EmployeeID: &agentID}
	payload := map[string]any{"source": "system"}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	stored, ok := employeeSessionEventFromOutbound(sb, &employeeOutboundEvent{
		EventType: "config.applied",
		Payload:   body,
		At:        time.Now().UTC(),
	}, payload, uuid.New(), "")
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

func TestIntegration_EmployeeSkillSync_UpsertsSkillAndAttachesAgent(t *testing.T) {
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
	agent1 := model.Employee{OrgID: &org1.ID, Name: "Aria", Model: "test"}
	agent2 := model.Employee{OrgID: &org2.ID, Name: "Aria", Model: "test"}
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
	sb1 := &model.Sandbox{ID: uuid.New(), OrgID: &org1.ID, EmployeeID: &agent1.ID}
	sb2 := &model.Sandbox{ID: uuid.New(), OrgID: &org2.ID, EmployeeID: &agent2.ID}
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
	db.Model(&model.EmployeeSkill{}).Where("employee_id = ? AND skill_id = ?", agent1.ID, skill.ID).Count(&links)
	if links != 1 {
		t.Fatalf("agent skill links = %d", links)
	}
	var bundle skillpkg.Bundle
	if err := json.Unmarshal(skill.Bundle, &bundle); err != nil {
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
	if err := db.First(&skill, "id = ?", skill.ID).Error; err != nil {
		t.Fatalf("reload updated skill: %v", err)
	}
	if !strings.Contains(string(skill.Bundle), "Check deployment logs first.") {
		t.Fatalf("bundle did not update: %s", string(skill.Bundle))
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
	db.Model(&model.EmployeeSkill{}).Where("employee_id = ? AND skill_id = ?", agent1.ID, skill.ID).Count(&links)
	if links != 0 {
		t.Fatalf("agent link should be detached, got %d", links)
	}
	if err := db.First(&model.Skill{}, "id = ?", skill.ID).Error; err != nil {
		t.Fatalf("skill should remain after detach-only delete: %v", err)
	}
}

func connectEmployeeSkillSyncTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://hivy:localdev@localhost:15432/hivy_test?sslmode=disable"
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
	testdb.ApplyMigrations(t, db)
	t.Cleanup(func() { sqlDB.Close() })
	return db
}
