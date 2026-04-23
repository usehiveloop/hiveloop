package streaming

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/usehiveloop/hiveloop/internal/model"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := "postgres://hiveloop:localdev@localhost:5433/hiveloop?sslmode=disable"
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Skipf("Postgres not available: %v", err)
	}
	if err := model.AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	return db
}

func setupFlusherTest(t *testing.T) (*EventBus, *Flusher, *gorm.DB, *redis.Client) {
	t.Helper()
	rc := setupTestRedis(t)
	db := setupTestDB(t)
	bus := NewEventBus(rc)
	flusher := NewFlusher(bus, db)
	return bus, flusher, db, rc
}

func createTestConversation(t *testing.T, db *gorm.DB) (uuid.UUID, uuid.UUID) {
	t.Helper()
	orgID := uuid.New()
	credID := uuid.New()
	agentID := uuid.New()
	convID := uuid.New()

	suffix := uuid.New().String()[:8]

	org := model.Org{ID: orgID, Name: "test-flusher-" + suffix, Active: true}
	db.Create(&org)

	cred := model.Credential{
		ID: credID, OrgID: orgID, ProviderID: "openrouter",
		EncryptedKey: []byte("test"), WrappedDEK: []byte("test"),
		BaseURL: "https://test.com", AuthScheme: "bearer",
	}
	if err := db.Create(&cred).Error; err != nil {
		t.Fatalf("create credential: %v", err)
	}

	sandboxID := uuid.New()

	sandbox := model.Sandbox{
		ID: sandboxID, SandboxType: "shared", Status: "running",
		ExternalID: "ext-" + suffix, BridgeURL: "https://test.local",
		EncryptedBridgeAPIKey: []byte("test"),
	}
	if err := db.Create(&sandbox).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}

	emptyJSON := model.JSON{}
	agent := model.Agent{
		ID: agentID, OrgID: &orgID,
		Name: "test-agent-" + suffix, Model: "test",
		CredentialID: &credID,
		SystemPrompt: "test", SandboxType: "shared", Status: "active",
		Tools: emptyJSON, McpServers: emptyJSON, Skills: emptyJSON,
		Integrations: emptyJSON, AgentConfig: emptyJSON,
		Permissions: emptyJSON,
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}

	conv := model.AgentConversation{
		ID: convID, OrgID: orgID, AgentID: agentID, SandboxID: sandboxID,
		BridgeConversationID: "bridge-" + suffix, Status: "active",
	}
	if err := db.Create(&conv).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	t.Cleanup(func() {
		db.Where("conversation_id = ?", convID).Delete(&model.ConversationEvent{})
		db.Delete(&conv)
		db.Delete(&agent)
		db.Delete(&cred)
		db.Delete(&sandbox)
		db.Delete(&org)
	})

	return orgID, convID
}

func TestFlusher_BatchWritesToPostgres(t *testing.T) {
	bus, flusher, db, _ := setupFlusherTest(t)
	_, convID := createTestConversation(t, db)
	ctx := context.Background()

	for i := 0; i < 50; i++ {
		data := json.RawMessage(`{"n":` + string(rune('0'+i%10)) + `}`)
		bus.Publish(ctx, convID.String(), "response_completed", data)
	}

	flusher.flushStream(ctx, convID.String())

	var count int64
	db.Model(&model.ConversationEvent{}).Where("conversation_id = ?", convID).Count(&count)
	if count != 50 {
		t.Fatalf("expected 50 events in Postgres, got %d", count)
	}
}

func TestFlusher_SkipsResponseChunks(t *testing.T) {
	bus, flusher, db, rc := setupFlusherTest(t)
	_, convID := createTestConversation(t, db)
	ctx := context.Background()

	for i := 0; i < 20; i++ {
		bus.Publish(ctx, convID.String(), "response_chunk", json.RawMessage(`{}`))
	}
	bus.Publish(ctx, convID.String(), "response_completed", json.RawMessage(`{}`))

	flusher.flushStream(ctx, convID.String())

	var count int64
	db.Model(&model.ConversationEvent{}).Where("conversation_id = ?", convID).Count(&count)
	if count != 1 {
		t.Fatalf("expected only response_completed persisted, got %d rows", count)
	}

	pending, err := rc.XPending(ctx, bus.streamKey(convID.String()), flusherGroup).Result()
	if err != nil {
		t.Fatalf("XPending: %v", err)
	}
	if pending.Count != 0 {
		t.Fatalf("expected all entries ACKed, got %d pending", pending.Count)
	}
}
