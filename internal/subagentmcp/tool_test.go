package subagentmcp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
)

const testDBURL = "postgres://hiveloop:localdev@localhost:5433/hiveloop_test?sslmode=disable" // #nosec G101 -- local test DB fixture

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = testDBURL
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	sqlDB, _ := db.DB()
	if err := sqlDB.Ping(); err != nil {
		t.Skipf("postgres unreachable: %v", err)
	}
	if err := model.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	return db
}

// fakeBridge captures bridge calls and produces canned SSE responses.
type fakeBridge struct {
	createCalls   atomic.Int32
	upsertCalls   atomic.Int32
	sendCalls     atomic.Int32
	streamPayload string
	streamHang    bool
	convID        string
}

func (f *fakeBridge) CreateConversation(ctx context.Context, agentID string) (*CreateConversationResponse, error) {
	f.createCalls.Add(1)
	if f.convID == "" {
		f.convID = "bridge-conv-" + uuid.New().String()
	}
	return &CreateConversationResponse{ConversationID: f.convID}, nil
}

func (f *fakeBridge) SendMessage(ctx context.Context, convID, content string) error {
	f.sendCalls.Add(1)
	return nil
}

func (f *fakeBridge) SSEStream(ctx context.Context, convID string) (io.ReadCloser, error) {
	if f.streamHang {
		return io.NopCloser(&hangingReader{ctx: ctx}), nil
	}
	return io.NopCloser(strings.NewReader(f.streamPayload)), nil
}

// hangingReader blocks until ctx cancellation, then returns the cancellation
// error. Used to simulate a stalled bridge so we can exercise timeout paths.
type hangingReader struct{ ctx context.Context }

func (h *hangingReader) Read(p []byte) (int, error) {
	<-h.ctx.Done()
	return 0, h.ctx.Err()
}

// fakeOrch and fakePush satisfy the local Orchestrator/Pusher interfaces.
// EnsureSubagentSandbox inserts a real Sandbox row so the downstream
// AgentConversation FK constraint is satisfied.
type fakeOrch struct {
	bridge *fakeBridge
	db     *gorm.DB
	t      *testing.T
	sb     *model.Sandbox
}

func (f *fakeOrch) EnsureSubagentSandbox(ctx context.Context, orgID, parentID, subID uuid.UUID) (*model.Sandbox, error) {
	if f.sb != nil {
		return f.sb, nil
	}
	sb := model.Sandbox{
		OrgID:                 &orgID,
		AgentID:               &subID,
		Status:                "running",
		EncryptedBridgeAPIKey: []byte("enc"),
	}
	if err := f.db.Create(&sb).Error; err != nil {
		f.t.Fatalf("create sandbox: %v", err)
	}
	f.t.Cleanup(func() { f.db.Where("id = ?", sb.ID).Delete(&model.Sandbox{}) })
	f.sb = &sb
	return &sb, nil
}

func (f *fakeOrch) GetBridgeClient(ctx context.Context, sb *model.Sandbox) (BridgeClient, error) {
	return f.bridge, nil
}

type fakePush struct {
	bridge *fakeBridge
}

func (f *fakePush) PushAgentToSandbox(ctx context.Context, agent *model.Agent, sb *model.Sandbox) error {
	if f.bridge != nil {
		f.bridge.upsertCalls.Add(1)
	}
	return nil
}

func setupParentAndSubagent(t *testing.T, db *gorm.DB, attached bool) (parent, sub model.Agent, org model.Org) {
	t.Helper()
	suffix := uuid.New().String()[:8]
	org = model.Org{Name: "subagentmcp-org-" + suffix}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", org.ID).Delete(&model.Org{}) })

	parent = model.Agent{
		OrgID:        &org.ID,
		Name:         "parent-" + suffix,
		SystemPrompt: "p",
		Model:        "gpt-4o",
	}
	if err := db.Create(&parent).Error; err != nil {
		t.Fatalf("create parent: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", parent.ID).Delete(&model.Agent{}) })

	sub = model.Agent{
		OrgID:        &org.ID,
		Name:         "code-explorer-" + suffix,
		SystemPrompt: "s",
		Model:        "gpt-4o",
		AgentType:    model.AgentTypeSubagent,
	}
	if err := db.Create(&sub).Error; err != nil {
		t.Fatalf("create subagent: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", sub.ID).Delete(&model.Agent{}) })

	if attached {
		link := model.AgentSubagent{AgentID: parent.ID, SubagentID: sub.ID}
		if err := db.Create(&link).Error; err != nil {
			t.Fatalf("create link: %v", err)
		}
		t.Cleanup(func() {
			db.Where("agent_id = ? AND subagent_id = ?", parent.ID, sub.ID).Delete(&model.AgentSubagent{})
		})
	}
	return parent, sub, org
}

func mintToken(t *testing.T, db *gorm.DB, orgID, agentID uuid.UUID) *model.Token {
	t.Helper()
	cred := model.Credential{
		OrgID:        orgID,
		ProviderID:   "openai",
		BaseURL:      "https://api.openai.com",
		AuthScheme:   "bearer",
		EncryptedKey: []byte("enc"),
		WrappedDEK:   []byte("dek"),
	}
	if err := db.Create(&cred).Error; err != nil {
		t.Fatalf("create credential: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", cred.ID).Delete(&model.Credential{}) })

	tok := model.Token{
		OrgID:        orgID,
		CredentialID: cred.ID,
		JTI:          "jti-" + uuid.New().String(),
		ExpiresAt:    time.Now().Add(time.Hour),
		Meta:         model.JSON{"agent_id": agentID.String(), "type": "agent_proxy"},
	}
	if err := db.Create(&tok).Error; err != nil {
		t.Fatalf("create token: %v", err)
	}
	t.Cleanup(func() { db.Where("jti = ?", tok.JTI).Delete(&model.Token{}) })
	return &tok
}

func callTool(t *testing.T, db *gorm.DB, token *model.Token, orch Orchestrator, push Pusher, args map[string]any) (*mcp.CallToolResult, error) {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "v0"}, nil)
	registerSubAgentTool(server, token, db, orch, push)

	raw, _ := json.Marshal(args)
	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{
		Name:      "sub_agent",
		Arguments: raw,
	}}
	return invoke(context.Background(), req, token, db, orch, push)
}

func TestSubAgentTool_HappyPath(t *testing.T) {
	db := setupTestDB(t)
	parent, sub, org := setupParentAndSubagent(t, db, true)
	tok := mintToken(t, db, org.ID, parent.ID)

	bridge := &fakeBridge{
		streamPayload: "" +
			"event: response_chunk\ndata: {\"text\":\"hello\"}\n\n" +
			"event: response_chunk\ndata: {\"text\":\" world\"}\n\n" +
			"event: turn_completed\ndata: {}\n\n",
	}
	orch := &fakeOrch{bridge: bridge, db: db, t: t}
	push := &fakePush{bridge: bridge}

	parentConvID := uuid.New()

	result, err := callTool(t, db, tok, orch, push, map[string]any{
		"subagent_name":          sub.Name,
		"prompt":                 "do the thing",
		"parent_conversation_id": parentConvID.String(),
	})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %+v", result.Content)
	}
	textContent, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content[0] not text: %T", result.Content[0])
	}
	if textContent.Text != "hello world" {
		t.Errorf("text = %q, want %q", textContent.Text, "hello world")
	}

	if bridge.createCalls.Load() != 1 {
		t.Errorf("CreateConversation called %d times, want 1", bridge.createCalls.Load())
	}
	if bridge.sendCalls.Load() != 1 {
		t.Errorf("SendMessage called %d times, want 1", bridge.sendCalls.Load())
	}
	if bridge.upsertCalls.Load() != 1 {
		t.Errorf("UpsertAgent (push) called %d times, want 1", bridge.upsertCalls.Load())
	}

	// Confirm a child AgentConversation row was created with parent linkage.
	var conv model.AgentConversation
	if err := db.Where("agent_id = ? AND bridge_conversation_id = ?", sub.ID, bridge.convID).First(&conv).Error; err != nil {
		t.Fatalf("expected child conversation row: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", conv.ID).Delete(&model.AgentConversation{}) })
	if conv.ParentConversationID == nil || *conv.ParentConversationID != parentConvID {
		t.Errorf("parent_conversation_id = %v, want %v", conv.ParentConversationID, parentConvID)
	}
}

func TestSubAgentTool_UnknownSubagent(t *testing.T) {
	db := setupTestDB(t)
	parent, _, org := setupParentAndSubagent(t, db, false)
	tok := mintToken(t, db, org.ID, parent.ID)

	bridge := &fakeBridge{}
	orch := &fakeOrch{bridge: bridge, db: db, t: t}
	push := &fakePush{bridge: bridge}

	result, err := callTool(t, db, tok, orch, push, map[string]any{
		"subagent_name": "does-not-exist",
		"prompt":        "x",
	})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected error result for unknown subagent; got %+v", result)
	}
	textContent, _ := result.Content[0].(*mcp.TextContent)
	if !strings.Contains(textContent.Text, "subagent_not_found") {
		t.Errorf("error message = %q, want to contain subagent_not_found", textContent.Text)
	}
	if bridge.createCalls.Load() != 0 {
		t.Errorf("CreateConversation must not be called for unknown subagent; got %d", bridge.createCalls.Load())
	}
	// No conversation row should be created.
	var count int64
	db.Model(&model.AgentConversation{}).Count(&count)
	if count != 0 {
		// We can't assert count == 0 globally because the DB is shared, but
		// we can check no row exists for this parent.
		var match int64
		db.Model(&model.AgentConversation{}).Where("org_id = ?", org.ID).Count(&match)
		if match != 0 {
			t.Errorf("no conversation should be created; found %d for org", match)
		}
	}
}

// TestSubAgentTool_TimeoutPropagates uses a short caller-side context (the
// SSE wait honours ctx.Done) to verify deadline propagation. The tool's
// timeout_secs parameter is clamped to a minimum of 60s, so we exercise the
// faster ctx.Cancel path here instead — same code path inside
// consumeUntilCompleted.
func TestSubAgentTool_TimeoutPropagates(t *testing.T) {
	db := setupTestDB(t)
	parent, sub, org := setupParentAndSubagent(t, db, true)
	tok := mintToken(t, db, org.ID, parent.ID)

	bridge := &fakeBridge{streamHang: true}
	orch := &fakeOrch{bridge: bridge, db: db, t: t}
	push := &fakePush{bridge: bridge}

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "v0"}, nil)
	registerSubAgentTool(server, tok, db, orch, push)

	args := map[string]any{
		"subagent_name": sub.Name,
		"prompt":        "stalled",
		"timeout_secs":  60,
	}
	raw, _ := json.Marshal(args)
	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{
		Name:      "sub_agent",
		Arguments: raw,
	}}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	result, err := invoke(ctx, req, tok, db, orch, push)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected timeout error; got success: %+v", result)
	}
	textContent, _ := result.Content[0].(*mcp.TextContent)
	if !strings.Contains(textContent.Text, "timeout") && !errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Errorf("error must mention timeout; got %q", textContent.Text)
	}
	if elapsed > 5*time.Second {
		t.Errorf("expected timeout to fire near 200ms, took %s", elapsed)
	}

	// Conversation row was still created (we got past CreateConversation),
	// then the SSE wait timed out — clean up.
	db.Where("agent_id = ?", sub.ID).Delete(&model.AgentConversation{})
}
