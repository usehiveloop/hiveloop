package streaming

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/llmvault/llmvault/internal/model"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := "postgres://llmvault:localdev@localhost:5433/llmvault?sslmode=disable"
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
	identityID := uuid.New()
	credID := uuid.New()
	agentID := uuid.New()
	convID := uuid.New()

	suffix := uuid.New().String()[:8]

	org := model.Org{ID: orgID, Name: "test-flusher-" + suffix, Active: true}
	db.Create(&org)

	identity := model.Identity{ID: identityID, OrgID: orgID, ExternalID: "test-" + suffix}
	db.Create(&identity)

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
		ID: sandboxID, OrgID: &orgID, IdentityID: &identityID,
		SandboxType: "shared", Status: "running",
		ExternalID: "ext-" + suffix, BridgeURL: "https://test.local",
		EncryptedBridgeAPIKey: []byte("test"),
	}
	if err := db.Create(&sandbox).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}

	emptyJSON := model.JSON{}
	agent := model.Agent{
		ID: agentID, OrgID: &orgID, IdentityID: &identityID, CredentialID: &credID,
		Name: "test-agent-" + suffix, Model: "test",
		SystemPrompt: "test", SandboxType: "shared", Status: "active",
		Tools: emptyJSON, McpServers: emptyJSON, Skills: emptyJSON,
		Integrations: emptyJSON, Subagents: emptyJSON, AgentConfig: emptyJSON,
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
		db.Delete(&identity)
		db.Delete(&org)
	})

	return orgID, convID
}

func TestFlusher_BatchWritesToPostgres(t *testing.T) {
	bus, flusher, db, _ := setupFlusherTest(t)
	_, convID := createTestConversation(t, db)
	ctx := context.Background()

	// Publish 50 events
	for i := 0; i < 50; i++ {
		data := json.RawMessage(`{"n":` + string(rune('0'+i%10)) + `}`)
		bus.Publish(ctx, convID.String(), "response_chunk", data)
	}

	// Run one flush cycle
	flusher.flushStream(ctx, convID.String())

	// Verify in Postgres
	var count int64
	db.Model(&model.ConversationEvent{}).Where("conversation_id = ?", convID).Count(&count)
	if count != 50 {
		t.Fatalf("expected 50 events in Postgres, got %d", count)
	}
}

func TestFlusher_AcksAfterFlush(t *testing.T) {
	bus, flusher, db, rc := setupFlusherTest(t)
	_, convID := createTestConversation(t, db)
	ctx := context.Background()

	bus.Publish(ctx, convID.String(), "chunk", json.RawMessage(`{}`))
	flusher.flushStream(ctx, convID.String())

	// Check pending entries — should be 0 after ACK
	pending, err := rc.XPending(ctx, bus.streamKey(convID.String()), flusherGroup).Result()
	if err != nil {
		t.Fatalf("XPending: %v", err)
	}
	if pending.Count != 0 {
		t.Fatalf("expected 0 pending entries, got %d", pending.Count)
	}
}

func TestFlusher_DoesNotAckOnDBError(t *testing.T) {
	rc := setupTestRedis(t)
	// Use a bad DSN so Postgres writes fail
	badDB, err := gorm.Open(postgres.Open("postgres://bad:bad@localhost:1/bad?sslmode=disable"), &gorm.Config{Logger: logger.Discard})
	if err == nil {
		// Connection might succeed lazily — that's ok, the INSERT will fail
		_ = badDB
	} else {
		t.Skip("cannot test DB error scenario")
	}

	bus := NewEventBus(rc)
	flusher := NewFlusher(bus, badDB)
	ctx := context.Background()

	// We need a valid conversation ID in the format, but since DB is bad, flush will fail
	convID := uuid.New().String()
	bus.Publish(ctx, convID, "chunk", json.RawMessage(`{}`))

	// Ensure consumer group exists
	rc.XGroupCreateMkStream(ctx, bus.streamKey(convID), flusherGroup, "0")

	// Flush — should fail on DB insert, not ACK
	flusher.flushStream(ctx, convID)

	// Events should still be in the stream (not trimmed)
	length, _ := bus.StreamLen(ctx, convID)
	if length != 1 {
		t.Fatalf("expected 1 event still in stream, got %d", length)
	}
}

func TestFlusher_TrimsAfterFlush(t *testing.T) {
	bus, flusher, db, _ := setupFlusherTest(t)
	_, convID := createTestConversation(t, db)
	ctx := context.Background()

	// Publish more than trimMaxLen events
	for i := 0; i < 600; i++ {
		bus.Publish(ctx, convID.String(), "chunk", json.RawMessage(`{}`))
	}

	flusher.flushStream(ctx, convID.String())

	// Stream should be trimmed to ~500
	length, _ := bus.StreamLen(ctx, convID.String())
	if length > 550 { // approximate trim
		t.Fatalf("expected stream trimmed to ~500, got %d", length)
	}
}

func TestFlusher_GracefulShutdown(t *testing.T) {
	bus, flusher, db, _ := setupFlusherTest(t)
	_, convID := createTestConversation(t, db)
	ctx, cancel := context.WithCancel(context.Background())

	bus.Publish(ctx, convID.String(), "chunk", json.RawMessage(`{}`))

	// Start flusher in goroutine
	done := make(chan struct{})
	go func() {
		flusher.Run(ctx)
		close(done)
	}()

	// Let it run one cycle
	time.Sleep(3 * time.Second)

	// Cancel and wait for clean shutdown
	cancel()
	select {
	case <-done:
		// ok
	case <-time.After(5 * time.Second):
		t.Fatal("flusher did not shut down within 5 seconds")
	}
}
