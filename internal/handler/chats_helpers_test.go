package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/auth"
	"github.com/usehiveloop/hiveloop/internal/config"
	"github.com/usehiveloop/hiveloop/internal/credentials"
	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/sandbox"
)

type chatStub struct {
	mu       sync.Mutex
	chunks   [][]byte
	called   int
	lastBody []byte
}

func (s *chatStub) snapshot() (calls int, body []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.called, append([]byte(nil), s.lastBody...)
}

type chatHarness struct {
	db        *gorm.DB
	router    *chi.Mux
	encKey    *struct{ k []byte }
	signKey   []byte
	stub      *chatStub
	sidecarSrv *httptest.Server
	orgID     uuid.UUID
}

func newChatHarness(t *testing.T) (*chatHarness, *handler.ChatHandler) {
	t.Helper()
	db := connectTestDB(t)
	if err := credentials.SeedPlatformOrg(db); err != nil {
		t.Fatalf("seed platform org: %v", err)
	}

	stub := &chatStub{
		chunks: [][]byte{
			[]byte(`data: {"id":"resp_test_1","choices":[{"delta":{"content":"Hello"}}]}` + "\n\n"),
			[]byte(`data: {"id":"resp_test_1","choices":[{"delta":{"content":" world"}}]}` + "\n\n"),
			[]byte("data: [DONE]\n\n"),
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/chat/completions":
			stub.mu.Lock()
			stub.called++
			body, _ := readAllNoErr(r)
			stub.lastBody = body
			chunks := stub.chunks
			stub.mu.Unlock()
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			rc := http.NewResponseController(w)
			for _, c := range chunks {
				_, _ = w.Write(c)
				_ = rc.Flush()
				time.Sleep(5 * time.Millisecond)
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	provider := &stubEmployeeProvider{endpoint: srv.URL}
	encKey := newTestEncKey(t)
	signKey := []byte("chat-test-signing-key-32-bytes!!")
	cfg := &config.Config{
		BridgeHost: "cp.hiveloop.test",
		ProxyHost:  "proxy.hiveloop.test",
	}
	orch := sandbox.NewOrchestrator(db, provider, nil, encKey, cfg)
	h := handler.NewChatHandler(db, orch, encKey, signKey)

	r := chi.NewRouter()
	r.Get("/v1/chats/{id}/stream", h.Stream)
	r.Route("/v1", func(r chi.Router) {
		r.Use(middleware.ResolveOrgFromHeader(db))
		r.Use(middleware.ResolveUser(db))
		r.Post("/employees/{id}/chats", h.Create)
		r.Get("/chats", h.List)
		r.Get("/chats/{id}", h.Get)
		r.Post("/chats/{id}/messages", h.Send)
	})

	return &chatHarness{
		db: db, router: r, signKey: signKey,
		stub: stub, sidecarSrv: srv,
	}, h
}

func readAllNoErr(r *http.Request) ([]byte, error) {
	defer r.Body.Close()
	buf := new(bytes.Buffer)
	_, err := buf.ReadFrom(r.Body)
	return buf.Bytes(), err
}

type chatOrg struct {
	org   model.Org
	user  model.User
	agent model.Agent
	sb    model.Sandbox
}

func (h *chatHarness) seedOrgAgentSandbox(t *testing.T) chatOrg {
	t.Helper()
	user := model.User{Email: "chat-" + uuid.NewString()[:8] + "@test.com", Name: "Chat User"}
	if err := h.db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	org := model.Org{Name: "chat-org-" + uuid.NewString()[:8], Active: true}
	if err := h.db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	mem := model.OrgMembership{UserID: user.ID, OrgID: org.ID, Role: "admin"}
	if err := h.db.Create(&mem).Error; err != nil {
		t.Fatalf("create membership: %v", err)
	}

	cred := model.Credential{
		OrgID: credentials.PlatformOrgID, Label: "chat-test",
		BaseURL: "https://api.example.com", AuthScheme: "bearer",
		EncryptedKey: []byte("enc"), WrappedDEK: []byte("dek"),
		ProviderID: "crof", IsSystem: true,
	}
	if err := h.db.Create(&cred).Error; err != nil {
		t.Fatalf("create cred: %v", err)
	}

	credID := cred.ID
	agent := model.Agent{
		OrgID:        &org.ID,
		Name:         "Hakari-" + uuid.NewString()[:6],
		IsEmployee:   true,
		Harness:      "employee-sandbox",
		Model:        "deepseek-v4-pro-precision",
		SystemPrompt: "test",
		CredentialID: &credID,
		Status:       "active",
		Tools:        model.JSON{}, McpServers: model.JSON{},
		Skills: model.JSON{}, Integrations: model.JSON{},
		Resources: model.JSON{}, AgentConfig: model.JSON{},
		Permissions: model.JSON{},
	}
	if err := h.db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}

	sidecarKey := "sidecar-key-" + uuid.NewString()[:8]
	encryptedKey, err := newTestEncKey(t).EncryptString(sidecarKey)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	sb := model.Sandbox{
		OrgID:                 &org.ID,
		AgentID:               &agent.ID,
		ExternalID:            "stub-sb-" + uuid.NewString()[:6],
		BridgeURL:             h.sidecarSrv.URL,
		EncryptedBridgeAPIKey: encryptedKey,
		Status:                "running",
	}
	if err := h.db.Create(&sb).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}

	t.Cleanup(func() {
		h.db.Where("session_id IN (SELECT id FROM chat_sessions WHERE org_id = ?)", org.ID).Delete(&model.ChatMessage{})
		h.db.Where("org_id = ?", org.ID).Delete(&model.ChatSession{})
		h.db.Where("id = ?", sb.ID).Delete(&model.Sandbox{})
		h.db.Where("id = ?", agent.ID).Delete(&model.Agent{})
		h.db.Where("id = ?", cred.ID).Delete(&model.Credential{})
		h.db.Where("user_id = ?", user.ID).Delete(&model.OrgMembership{})
		h.db.Where("id = ?", org.ID).Delete(&model.Org{})
		h.db.Where("id = ?", user.ID).Delete(&model.User{})
	})
	h.orgID = org.ID
	return chatOrg{org: org, user: user, agent: agent, sb: sb}
}

func (h *chatHarness) authedReq(t *testing.T, m chatOrg, method, path string, body any) *http.Request {
	t.Helper()
	var b *bytes.Buffer
	if body != nil {
		b = new(bytes.Buffer)
		_ = json.NewEncoder(b).Encode(body)
	}
	var req *http.Request
	if b != nil {
		req = httptest.NewRequest(method, path, b)
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-ID", m.org.ID.String())
	req = middleware.WithAuthClaims(req, &auth.AuthClaims{
		UserID: m.user.ID.String(),
		OrgID:  m.org.ID.String(),
		Role:   "admin",
	})
	return req
}
