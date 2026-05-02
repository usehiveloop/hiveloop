// Wave 3 e2e: tool-approval roundtrip against the fakebridge.
//
// Required infra:
//   DATABASE_URL  → Postgres reachable
//   REDIS_ADDR    → Redis reachable
// Tests skip via the existing harness if either is unavailable.
package e2e

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/e2e/fakebridge"
	bridgepkg "github.com/usehiveloop/hiveloop/internal/bridge"
	"github.com/usehiveloop/hiveloop/internal/config"
	"github.com/usehiveloop/hiveloop/internal/crypto"
	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/sandbox"
	"github.com/usehiveloop/hiveloop/internal/streaming"
)

// approvalsHarness wires the conversation handler with a real orchestrator
// (so getBridgeClient works) pointed at the fakebridge.
type approvalsHarness struct {
	*testHarness
	encKey  *crypto.SymmetricKey
	bridge  *fakebridge.Server
	org     model.Org
	agent   model.Agent
	sandbox model.Sandbox
	conv    model.AgentConversation
	router  *chi.Mux
}

func newApprovalsHarness(t *testing.T) *approvalsHarness {
	t.Helper()
	h := newHarness(t)
	suffix := uuid.New().String()[:8]

	keyBytes := make([]byte, 32)
	for i := range keyBytes {
		keyBytes[i] = byte(i + 31)
	}
	encKey, err := crypto.NewSymmetricKey(base64.StdEncoding.EncodeToString(keyBytes))
	if err != nil {
		t.Fatalf("symmetric key: %v", err)
	}

	bridgeSecret := "ap-secret-" + suffix
	encryptedKey, err := encKey.EncryptString(bridgeSecret)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	fb := fakebridge.New(t)
	fb.SignSecret = []byte(bridgeSecret)

	org := model.Org{Name: "ap-org-" + suffix}
	h.db.Create(&org)
	t.Cleanup(func() { h.db.Where("id = ?", org.ID).Delete(&model.Org{}) })

	cred := model.Credential{
		OrgID: org.ID, BaseURL: "https://api.openai.com", AuthScheme: "bearer",
		ProviderID: "openai", EncryptedKey: []byte("enc"), WrappedDEK: []byte("dek"),
	}
	h.db.Create(&cred)
	t.Cleanup(func() { h.db.Where("id = ?", cred.ID).Delete(&model.Credential{}) })

	agent := model.Agent{
		OrgID: &org.ID, Name: "ap-agent-" + suffix,
		CredentialID: &cred.ID, SystemPrompt: "test", Model: "gpt-4o",
		Permissions: model.JSON{"bash": "require_approval"},
	}
	h.db.Create(&agent)
	t.Cleanup(func() { h.db.Where("id = ?", agent.ID).Delete(&model.Agent{}) })

	expiresAt := time.Now().Add(24 * time.Hour)
	sb := model.Sandbox{
		OrgID:                 &org.ID,
		AgentID:               &agent.ID,
		ExternalID:            "ap-ext-" + suffix,
		BridgeURL:             fb.URL,
		BridgeURLExpiresAt:    &expiresAt,
		EncryptedBridgeAPIKey: encryptedKey,
		Status:                "running",
	}
	h.db.Create(&sb)
	t.Cleanup(func() { h.db.Where("id = ?", sb.ID).Delete(&model.Sandbox{}) })

	conv := model.AgentConversation{
		OrgID: org.ID, AgentID: agent.ID, SandboxID: sb.ID,
		BridgeConversationID: "ap-conv-" + suffix, Status: "active",
	}
	h.db.Create(&conv)
	t.Cleanup(func() {
		h.db.Where("conversation_id = ?", conv.ID).Delete(&model.ConversationEvent{})
		h.db.Where("id = ?", conv.ID).Delete(&model.AgentConversation{})
	})

	cfg := &config.Config{
		ProxyHost:   "proxy.test",
		MCPBaseURL:  "https://mcp.test",
		BridgeHost:  "bridge.test",
	}
	orch := sandbox.NewOrchestrator(h.db, nil, nil, encKey, cfg)
	eventBus := streaming.NewEventBus(h.redisClient)

	convHandler := handler.NewConversationHandler(h.db, orch, nil, eventBus)
	webhookHandler := handler.NewBridgeWebhookHandler(h.db, encKey, eventBus, nil)

	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			req = middleware.WithOrg(req, &org)
			next.ServeHTTP(w, req)
		})
	})
	r.Route("/v1/conversations/{convID}", func(r chi.Router) {
		r.Post("/messages", convHandler.SendMessage)
		r.Get("/stream", convHandler.Stream)
		r.Get("/approvals", convHandler.ListApprovals)
		r.Post("/approvals/{requestID}", convHandler.ResolveApproval)
	})
	r.Post("/internal/webhooks/bridge/{sandboxID}", webhookHandler.Handle)

	whSrv := httptest.NewServer(r)
	t.Cleanup(whSrv.Close)
	fb.WebhookURL = whSrv.URL + "/internal/webhooks/bridge/" + sb.ID.String()

	return &approvalsHarness{
		testHarness: h,
		encKey:      encKey,
		bridge:      fb,
		org:         org,
		agent:       agent,
		sandbox:     sb,
		conv:        conv,
		router:      r,
	}
}

func runApprovalRoundtrip(t *testing.T, decision string) {
	t.Helper()
	ah := newApprovalsHarness(t)

	// Seed fakebridge with a pending approval request that ListApprovals
	// can return. The hiveloop ResolveApproval flow doesn't actually
	// require this — it forwards the decision regardless — but it makes
	// the test more end-to-end.
	approvalID := "appr-" + uuid.New().String()[:8]
	ah.bridge.SetPendingApprovals([]bridgepkg.ApprovalRequest{
		{
			Id:             approvalID,
			AgentId:        ah.agent.ID.String(),
			ConversationId: ah.conv.BridgeConversationID,
			ToolName:       "bash",
			ToolCallId:     "tc-1",
			Arguments:      map[string]any{"cmd": "ls"},
			Status:         "pending",
			CreatedAt:      time.Now(),
		},
	})

	srv := httptest.NewServer(ah.router)
	t.Cleanup(srv.Close)

	// Open SSE.
	sseReq, _ := http.NewRequest(http.MethodGet,
		srv.URL+"/v1/conversations/"+ah.conv.ID.String()+"/stream", nil)
	sseResp, err := (&http.Client{Timeout: 30 * time.Second}).Do(sseReq)
	if err != nil {
		t.Fatalf("open SSE: %v", err)
	}
	defer sseResp.Body.Close()

	gotTypes := make(chan string, 32)
	go func() {
		defer close(gotTypes)
		buf := make([]byte, 4096)
		acc := ""
		for {
			n, err := sseResp.Body.Read(buf)
			if n > 0 {
				acc += string(buf[:n])
				for {
					idx := strings.Index(acc, "\n\n")
					if idx == -1 {
						break
					}
					frame := acc[:idx]
					acc = acc[idx+2:]
					for _, line := range strings.Split(frame, "\n") {
						if strings.HasPrefix(line, "event:") {
							gotTypes <- strings.TrimSpace(strings.TrimPrefix(line, "event:"))
						}
					}
				}
			}
			if err != nil {
				return
			}
		}
	}()

	time.Sleep(150 * time.Millisecond)

	// Bridge fires the approval-required event.
	approvalReq := []fakebridge.BridgeEvent{
		{
			EventID: "ev-tar1", EventType: "tool_approval_required",
			AgentID: ah.agent.ID.String(), ConversationID: ah.conv.BridgeConversationID,
			Timestamp: time.Now(), SequenceNumber: 1,
			Data: json.RawMessage(`{"request_id":"` + approvalID + `","tool":"bash"}`),
		},
	}
	if status, body := ah.bridge.PostWebhook(t, approvalReq); status != http.StatusOK {
		t.Fatalf("approval webhook: status=%d body=%s", status, body)
	}

	// Verify SSE consumer saw it.
	if !waitForType(gotTypes, "tool_approval_required", 3*time.Second) {
		t.Fatal("SSE never delivered tool_approval_required")
	}

	// Client POSTs the decision via hiveloop's ResolveApproval handler.
	body := []byte(`{"decision":"` + decision + `"}`)
	req, _ := http.NewRequest(http.MethodPost,
		srv.URL+"/v1/conversations/"+ah.conv.ID.String()+"/approvals/"+approvalID,
		strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("resolve POST: %v", err)
	}
	respBody := readAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("resolve: status=%d body=%s", resp.StatusCode, respBody)
	}

	// Verify fakebridge captured the decision under the right path.
	cap := ah.bridge.CapturedSnapshot()
	if len(cap.Approvals) != 1 {
		t.Fatalf("fakebridge approvals captured: got %d, want 1", len(cap.Approvals))
	}
	gotCall := cap.Approvals[0]
	if gotCall.AgentID != ah.agent.ID.String() {
		t.Errorf("agent_id: got %q, want %q", gotCall.AgentID, ah.agent.ID.String())
	}
	if gotCall.ConversationID != ah.conv.BridgeConversationID {
		t.Errorf("conversation_id: got %q, want %q", gotCall.ConversationID, ah.conv.BridgeConversationID)
	}
	if gotCall.RequestID != approvalID {
		t.Errorf("request_id: got %q, want %q", gotCall.RequestID, approvalID)
	}
	if gotCall.Decision != decision {
		t.Errorf("decision: got %q, want %q", gotCall.Decision, decision)
	}

	// Bridge fires the resolution events.
	resolved := []fakebridge.BridgeEvent{
		{
			EventID: "ev-resolved", EventType: "tool_approval_resolved",
			AgentID: ah.agent.ID.String(), ConversationID: ah.conv.BridgeConversationID,
			Timestamp: time.Now(), SequenceNumber: 2,
			Data: json.RawMessage(`{"request_id":"` + approvalID + `","decision":"` + decision + `"}`),
		},
		{
			EventID: "ev-result", EventType: "tool_call_result",
			AgentID: ah.agent.ID.String(), ConversationID: ah.conv.BridgeConversationID,
			Timestamp: time.Now(), SequenceNumber: 3,
			Data: json.RawMessage(`{"output":"ok"}`),
		},
		{
			EventID: "ev-tc", EventType: "turn_completed",
			AgentID: ah.agent.ID.String(), ConversationID: ah.conv.BridgeConversationID,
			Timestamp: time.Now(), SequenceNumber: 4,
			Data: json.RawMessage(`{"stop_reason":"end_turn"}`),
		},
	}
	if status, body := ah.bridge.PostWebhook(t, resolved); status != http.StatusOK {
		t.Fatalf("resolved webhook: status=%d body=%s", status, body)
	}

	if !waitForType(gotTypes, "tool_approval_resolved", 3*time.Second) {
		t.Errorf("SSE never delivered tool_approval_resolved (decision=%s)", decision)
	}
}

// TestApprovalFlow_BridgeRequestRoundtrip drives an approve decision
// through the full chain.
func TestApprovalFlow_BridgeRequestRoundtrip(t *testing.T) {
	runApprovalRoundtrip(t, "approve")
}

// TestApprovalFlow_DenyRoundtrip locks the deny path.
func TestApprovalFlow_DenyRoundtrip(t *testing.T) {
	runApprovalRoundtrip(t, "deny")
}

// waitForType drains the gotTypes channel until it sees `target` or the
// timeout fires. Returns true on hit.
func waitForType(ch <-chan string, target string, timeout time.Duration) bool {
	deadline := time.After(timeout)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return false
			}
			if ev == target {
				return true
			}
		case <-deadline:
			return false
		}
	}
}

func readAll(r interface {
	Read(p []byte) (int, error)
}) []byte {
	b := make([]byte, 4096)
	out := []byte{}
	for {
		n, err := r.Read(b)
		if n > 0 {
			out = append(out, b[:n]...)
		}
		if err != nil {
			return out
		}
	}
}
