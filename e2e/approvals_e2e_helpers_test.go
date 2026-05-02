package e2e

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/e2e/fakebridge"
	"github.com/usehiveloop/hiveloop/internal/config"
	"github.com/usehiveloop/hiveloop/internal/crypto"
	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/sandbox"
	"github.com/usehiveloop/hiveloop/internal/streaming"
)

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
		ProxyHost:  "proxy.test",
		MCPBaseURL: "https://mcp.test",
		BridgeHost: "bridge.test",
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
