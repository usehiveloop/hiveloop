package handler

import (
	"encoding/json"
	"net/http"
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

func TestShouldStoreEmployeeMemoryEvent_KeepsConversationTimeline(t *testing.T) {
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
		if !shouldStoreEmployeeMemoryEvent(eventType) {
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
		if shouldStoreEmployeeMemoryEvent(eventType) {
			t.Fatalf("%s should be skipped", eventType)
		}
	}
}

func TestEmployeeMemoryEventFromOutbound_StoresTimelineEventTypes(t *testing.T) {
	orgID := uuid.New()
	agentID := uuid.New()
	sandboxID := uuid.New()
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

func TestEmployeeOutboundModelUsage_WritesGenerationAndSkipsMemoryEvent(t *testing.T) {
	db := connectEmployeeSkillSyncTestDB(t)
	org := model.Org{Name: "runtime-usage-" + uuid.NewString(), RateLimit: 1000, Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	credential := model.Credential{
		Label:        "runtime-usage",
		BaseURL:      "https://openrouter.ai/api/v1",
		AuthScheme:   "bearer",
		EncryptedKey: []byte("encrypted"),
		WrappedDEK:   []byte("wrapped"),
		ProviderID:   "openrouter",
	}
	if err := db.Create(&credential).Error; err != nil {
		t.Fatalf("create credential: %v", err)
	}
	credentialID := credential.ID
	agent := model.Employee{OrgID: &org.ID, CredentialID: &credentialID, Name: "Aria", Model: "deepseek-v4-flash", IsEmployee: true}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	sandbox := model.Sandbox{OrgID: &org.ID, EmployeeID: &agent.ID, ExternalID: "runtime-usage-sandbox", BridgeURL: "http://localhost:7080", EncryptedBridgeAPIKey: []byte("key"), Status: "running"}
	if err := db.Create(&sandbox).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	token := model.Token{
		OrgID:        org.ID,
		CredentialID: credential.ID,
		JTI:          uuid.NewString(),
		ExpiresAt:    time.Now().Add(time.Hour),
		Meta: model.JSON{
			"employee_id": agent.ID.String(),
			"sandbox_id":  sandbox.ID.String(),
			"type":        "employee_proxy",
			"user":        "runtime-test-user",
		},
	}
	if err := db.Create(&token).Error; err != nil {
		t.Fatalf("create token: %v", err)
	}
	t.Cleanup(func() {
		db.Where("org_id = ?", org.ID).Delete(&model.Generation{})
		db.Where("id = ?", token.ID).Delete(&model.Token{})
		db.Where("id = ?", sandbox.ID).Delete(&model.Sandbox{})
		db.Where("id = ?", agent.ID).Delete(&model.Employee{})
		db.Where("id = ?", org.ID).Delete(&model.Org{})
		db.Where("id = ?", credential.ID).Delete(&model.Credential{})
	})

	payload := map[string]any{
		"session_id": "http-session-1",
		"source":     "http",
		"sequence":   float64(42),
		"agent_event": map[string]any{
			"kind":  "run_event",
			"event": "model_usage",
			"payload": map[string]any{
				"model":      "deepseek-v4-flash",
				"session_id": "http-session-1",
				"turn_id":    "turn-1",
				"usage": map[string]any{
					"prompt_tokens":      float64(11),
					"completion_tokens":  float64(7),
					"total_tokens":       float64(18),
					"cached_tokens":      float64(5),
					"reasoning_tokens":   float64(3),
					"cache_write_tokens": float64(0),
					"cost":               0.000123,
				},
			},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	h := NewEmployeeOutboundWebhookHandler(db, nil, nil)
	h.storeAndMaybeEnqueue(t.Context(), &sandbox, &employeeOutboundEvent{
		EventType: "agent.run.model.usage",
		Payload:   body,
		At:        time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC),
	})

	var gen model.Generation
	if err := db.Where("org_id = ?", org.ID).First(&gen).Error; err != nil {
		t.Fatalf("load generation: %v", err)
	}
	if gen.TokenJTI != token.JTI || gen.CredentialID != credential.ID || gen.ProviderID != "openrouter" {
		t.Fatalf("generation attribution mismatch: %#v", gen)
	}
	if gen.InputTokens != 11 || gen.OutputTokens != 7 || gen.CachedTokens != 5 || gen.ReasoningTokens != 3 {
		t.Fatalf("generation usage mismatch: %#v", gen)
	}
	if gen.Model != "deepseek-v4-flash" || !gen.IsStreaming || !gen.IsSystem || gen.UpstreamStatus != http.StatusOK {
		t.Fatalf("generation metadata mismatch: %#v", gen)
	}
	if gen.Cost != 0.000123 || gen.UserID != "runtime-test-user" {
		t.Fatalf("generation cost/user mismatch: %#v", gen)
	}
	if !strings.Contains(strings.Join(gen.Tags, ","), "session:http-session-1") {
		t.Fatalf("generation tags missing session: %#v", gen.Tags)
	}
	var eventCount int64
	db.Model(&model.EmployeeMemoryEvent{}).Where("sandbox_id = ?", sandbox.ID).Count(&eventCount)
	if eventCount != 0 {
		t.Fatalf("model usage memory event count = %d, want 0", eventCount)
	}
}

func TestEmployeeMemoryEventFromOutbound_StoresEventWithoutSessionID(t *testing.T) {
	orgID := uuid.New()
	agentID := uuid.New()
	sandboxID := uuid.New()
	sb := &model.Sandbox{ID: sandboxID, OrgID: &orgID, EmployeeID: &agentID}
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
