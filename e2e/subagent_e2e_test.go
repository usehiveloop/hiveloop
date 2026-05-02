// Wave 3 e2e: subagent invocation via the hiveloop MCP `sub_agent` tool
// against fakebridges.
//
// Required infra:
//   DATABASE_URL  → Postgres reachable
// Tests skip via the existing harness if Postgres is unavailable. Redis
// is not required by these tests (the SSE drain is fakebridge-direct via
// the bridge client's SSEStream call).
package e2e

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/usehiveloop/hiveloop/e2e/fakebridge"
	"github.com/usehiveloop/hiveloop/internal/config"
	"github.com/usehiveloop/hiveloop/internal/crypto"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/sandbox"
	"github.com/usehiveloop/hiveloop/internal/subagentmcp"
)

// subagentHarness sets up a parent agent + 2 attached subagents, each
// with its own pre-created sandbox pointing at a dedicated fakebridge.
type subagentHarness struct {
	*testHarness
	encKey       *crypto.SymmetricKey
	parent       model.Agent
	parentSB     model.Sandbox
	parentBridge *fakebridge.Server
	subA         model.Agent
	subASB       model.Sandbox
	subABridge   *fakebridge.Server
	subB         model.Agent
	subBSB       model.Sandbox
	subBBridge   *fakebridge.Server
	org          model.Org
	cred         model.Credential
	parentToken  *model.Token
	orch         *sandbox.Orchestrator
	pusher       *sandbox.Pusher
}

func newSubagentHarness(t *testing.T) *subagentHarness {
	t.Helper()
	h := newHarness(t)
	suffix := uuid.New().String()[:8]

	keyBytes := make([]byte, 32)
	for i := range keyBytes {
		keyBytes[i] = byte(i + 41)
	}
	encKey, err := crypto.NewSymmetricKey(base64.StdEncoding.EncodeToString(keyBytes))
	if err != nil {
		t.Fatalf("symmetric key: %v", err)
	}

	org := model.Org{Name: "sa-org-" + suffix}
	h.db.Create(&org)
	t.Cleanup(func() { h.db.Where("id = ?", org.ID).Delete(&model.Org{}) })

	cred := model.Credential{
		OrgID: org.ID, BaseURL: "https://api.openai.com", AuthScheme: "bearer",
		ProviderID: "openai", EncryptedKey: []byte("enc"), WrappedDEK: []byte("dek"),
	}
	h.db.Create(&cred)
	t.Cleanup(func() { h.db.Where("id = ?", cred.ID).Delete(&model.Credential{}) })

	parent := model.Agent{
		OrgID: &org.ID, Name: "sa-parent-" + suffix,
		CredentialID: &cred.ID, SystemPrompt: "parent", Model: "gpt-4o",
		AgentType: "agent",
	}
	h.db.Create(&parent)
	t.Cleanup(func() { h.db.Where("id = ?", parent.ID).Delete(&model.Agent{}) })

	subA := model.Agent{
		OrgID: &org.ID, Name: "researcher-" + suffix,
		CredentialID: &cred.ID, SystemPrompt: "researcher", Model: "gpt-4o",
		AgentType: model.AgentTypeSubagent,
	}
	subB := model.Agent{
		OrgID: &org.ID, Name: "summarizer-" + suffix,
		CredentialID: &cred.ID, SystemPrompt: "summarizer", Model: "gpt-4o",
		AgentType: model.AgentTypeSubagent,
	}
	h.db.Create(&subA)
	h.db.Create(&subB)
	t.Cleanup(func() {
		h.db.Where("id IN ?", []uuid.UUID{subA.ID, subB.ID}).Delete(&model.Agent{})
	})

	for _, sub := range []model.Agent{subA, subB} {
		link := model.AgentSubagent{AgentID: parent.ID, SubagentID: sub.ID}
		h.db.Create(&link)
	}
	t.Cleanup(func() { h.db.Where("agent_id = ?", parent.ID).Delete(&model.AgentSubagent{}) })

	parentBridge := fakebridge.New(t)
	subABridge := fakebridge.New(t)
	subBBridge := fakebridge.New(t)

	mkSB := func(agentID uuid.UUID, fbURL, suffix string) model.Sandbox {
		bridgeSecret := "sa-secret-" + suffix
		ek, _ := encKey.EncryptString(bridgeSecret)
		expiresAt := time.Now().Add(24 * time.Hour)
		sb := model.Sandbox{
			OrgID:                 &org.ID,
			AgentID:               &agentID,
			ExternalID:            "sa-ext-" + suffix,
			BridgeURL:             fbURL,
			BridgeURLExpiresAt:    &expiresAt,
			EncryptedBridgeAPIKey: ek,
			Status:                "running",
		}
		h.db.Create(&sb)
		t.Cleanup(func() { h.db.Where("id = ?", sb.ID).Delete(&model.Sandbox{}) })
		return sb
	}
	parentSB := mkSB(parent.ID, parentBridge.URL, "p-"+suffix)
	subASB := mkSB(subA.ID, subABridge.URL, "a-"+suffix)
	subBSB := mkSB(subB.ID, subBBridge.URL, "b-"+suffix)

	// Parent token (proxy token) carries agent_id in meta.
	tok := model.Token{
		OrgID:        org.ID,
		CredentialID: cred.ID,
		JTI:          "jti-" + uuid.New().String(),
		ExpiresAt:    time.Now().Add(time.Hour),
		Meta:         model.JSON{"agent_id": parent.ID.String(), "type": "agent_proxy"},
	}
	h.db.Create(&tok)
	t.Cleanup(func() { h.db.Where("jti = ?", tok.JTI).Delete(&model.Token{}) })

	cfg := &config.Config{
		ProxyHost:  "proxy.test",
		MCPBaseURL: "https://mcp.test",
		BridgeHost: "bridge.test",
	}
	orch := sandbox.NewOrchestrator(h.db, nil, nil, encKey, cfg)
	pusher := sandbox.NewPusher(h.db, orch, h.signingKey, cfg, nil)

	return &subagentHarness{
		testHarness:  h,
		encKey:       encKey,
		parent:       parent,
		parentSB:     parentSB,
		parentBridge: parentBridge,
		subA:         subA,
		subASB:       subASB,
		subABridge:   subABridge,
		subB:         subB,
		subBSB:       subBSB,
		subBBridge:   subBBridge,
		org:          org,
		cred:         cred,
		parentToken:  &tok,
		orch:         orch,
		pusher:       pusher,
	}
}

// callSubAgentTool wraps the registration + invoke through the public
// RegisterTools surface so we exercise the same path the real MCP server
// uses. Returns the tool's CallToolResult and any transport-level error.
func callSubAgentTool(t *testing.T, sh *subagentHarness, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "v0"}, nil)
	register := subagentmcp.RegisterTools(
		subagentmcp.NewOrchestratorAdapter(sh.orch),
		subagentmcp.NewPusherAdapter(sh.pusher),
	)
	register(server, sh.parentToken, sh.db)

	raw, _ := json.Marshal(args)
	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{
		Name:      "sub_agent",
		Arguments: raw,
	}}

	// Server.CallTool isn't directly invokable in the public API; the
	// canonical way the SDK exposes invocation in tests is through the
	// transport, but for unit-style assertions other tests in this repo
	// (see internal/subagentmcp/tool_test.go) use a side-door reflection
	// approach. To stay on a stable surface, drive the tool by exposing
	// it via an internal session-less call: walk the registered tool's
	// handler.
	//
	// The mcp-go-sdk doesn't expose a direct handler-fetch on the public
	// Server type. We work around it by re-using the same tool function
	// pattern as the existing tool_test.go: call the unexported invoke
	// helper through the registered surface. The simplest stable way is
	// to start the server's in-process transport; instead we replicate
	// the pattern by re-invoking the tool's logic via the same code path
	// the registration uses — call subagentmcp.Register and then drive a
	// Run() loop. To keep this from spiraling, we use a tiny in-process
	// transport pair.
	//
	// However, the existing internal/subagentmcp/tool_test.go imports the
	// unexported `invoke` directly. From this external package, we don't
	// have that access. Drive via the SDK's in-memory client/server pair.
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	go func() {
		_ = server.Run(context.Background(), serverTransport)
	}()
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v0"}, nil)
	clientSession, err := mcpClient.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = clientSession.Close() })

	resp, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "sub_agent",
		Arguments: req.Params.Arguments,
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	return resp
}

// scriptedFinalAnswer makes a fakebridge return a "final answer" turn.
func scriptedFinalAnswer(text string) func(string, string) []fakebridge.BridgeEvent {
	return func(agentID, convID string) []fakebridge.BridgeEvent {
		now := time.Now()
		return []fakebridge.BridgeEvent{
			{
				EventID: "ev1", EventType: "message_received",
				AgentID: agentID, ConversationID: convID,
				Timestamp: now, SequenceNumber: 1, Data: json.RawMessage(`{}`),
			},
			{
				EventID: "ev2", EventType: "response_chunk",
				AgentID: agentID, ConversationID: convID,
				Timestamp: now.Add(10 * time.Millisecond), SequenceNumber: 2,
				Data: json.RawMessage(`{"text":"` + text + `"}`),
			},
			{
				EventID: "ev3", EventType: "turn_completed",
				AgentID: agentID, ConversationID: convID,
				Timestamp: now.Add(20 * time.Millisecond), SequenceNumber: 3,
				Data: json.RawMessage(`{"stop_reason":"end_turn"}`),
			},
		}
	}
}

// TestSubAgentTool_EndToEndViaHiveloopMCP exercises the full sub_agent
// MCP-tool path: parent invokes researcher → researcher's fakebridge
// receives push, create-conversation, message; final SSE returns text.
func TestSubAgentTool_EndToEndViaHiveloopMCP(t *testing.T) {
	sh := newSubagentHarness(t)

	// Subagent's fakebridge produces the "final answer" turn. The
	// fakebridge SSE handler will stream these on the GET /stream call.
	sh.subABridge.ScriptedSSE = scriptedFinalAnswer("final answer")

	parentConvID := uuid.New()
	result := callSubAgentTool(t, sh, map[string]any{
		"subagent_name":          sh.subA.Name,
		"prompt":                 "do thing",
		"parent_conversation_id": parentConvID.String(),
	})
	if result.IsError {
		t.Fatalf("tool result is error: %+v", result.Content)
	}
	if len(result.Content) == 0 {
		t.Fatal("empty tool result")
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content[0] not text: %T", result.Content[0])
	}
	if tc.Text != "final answer" {
		t.Errorf("text = %q, want %q", tc.Text, "final answer")
	}

	// Subagent's fakebridge should have seen UpsertAgent, Create, and
	// Send. Parent's fakebridge should be untouched.
	subACap := sh.subABridge.CapturedSnapshot()
	if len(subACap.UpsertAgents) == 0 {
		t.Errorf("subagent fakebridge: expected UpsertAgent, got none")
	}
	if len(subACap.CreateConversations) == 0 {
		t.Errorf("subagent fakebridge: expected CreateConversation, got none")
	}
	if len(subACap.Messages) == 0 {
		t.Errorf("subagent fakebridge: expected SendMessage, got none")
	}
	parentCap := sh.parentBridge.CapturedSnapshot()
	if len(parentCap.UpsertAgents) != 0 {
		t.Errorf("parent fakebridge should not have received UpsertAgent; got %d", len(parentCap.UpsertAgents))
	}
	if len(parentCap.CreateConversations) != 0 {
		t.Errorf("parent fakebridge should not have received CreateConversation; got %d", len(parentCap.CreateConversations))
	}
	subBCap := sh.subBBridge.CapturedSnapshot()
	if len(subBCap.UpsertAgents) != 0 {
		t.Errorf("other subagent's fakebridge should not have received UpsertAgent; got %d", len(subBCap.UpsertAgents))
	}

	// Confirm child AgentConversation row with parent linkage.
	var conv model.AgentConversation
	err := sh.db.
		Where("agent_id = ? AND parent_conversation_id = ?", sh.subA.ID, parentConvID).
		First(&conv).Error
	if err != nil {
		t.Fatalf("expected child conversation row: %v", err)
	}
	t.Cleanup(func() { sh.db.Where("id = ?", conv.ID).Delete(&model.AgentConversation{}) })
	if conv.ParentConversationID == nil || *conv.ParentConversationID != parentConvID {
		t.Errorf("parent_conversation_id mismatch")
	}

	// Wire shape check: the subagent UpsertAgent must NOT contain dead
	// fields like `subagents`, `tools`, `immortal`.
	for _, raw := range subACap.UpsertAgentsRaw {
		var m map[string]any
		_ = json.Unmarshal(raw, &m)
		for _, dead := range []string{"subagents", "tools", "immortal"} {
			if _, ok := m[dead]; ok {
				t.Errorf("subagent upsert body contains forbidden field %q", dead)
			}
		}
		if _, ok := m["harness"]; !ok {
			t.Errorf("subagent upsert body missing required `harness` field; got keys: %v", mapKeys(m))
		}
	}
}

// TestSubAgentTool_UnknownSubagent_E2E asserts that invoking a name not
// attached to the parent returns an MCP error and never reaches any
// fakebridge.
func TestSubAgentTool_UnknownSubagent_E2E(t *testing.T) {
	sh := newSubagentHarness(t)

	result := callSubAgentTool(t, sh, map[string]any{
		"subagent_name": "does-not-exist",
		"prompt":        "x",
	})
	if !result.IsError {
		t.Fatalf("expected error result; got %+v", result)
	}
	tc, _ := result.Content[0].(*mcp.TextContent)
	if !strings.Contains(tc.Text, "subagent_not_found") {
		t.Errorf("error message = %q, want to contain subagent_not_found", tc.Text)
	}
	for _, fb := range []*fakebridge.Server{sh.parentBridge, sh.subABridge, sh.subBBridge} {
		c := fb.CapturedSnapshot()
		if len(c.UpsertAgents) != 0 || len(c.CreateConversations) != 0 || len(c.Messages) != 0 {
			t.Errorf("fakebridge unexpectedly contacted: %+v", c)
		}
	}
}

func mapKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
