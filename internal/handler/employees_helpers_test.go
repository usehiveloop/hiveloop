package handler_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
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
	"github.com/usehiveloop/hiveloop/internal/crypto"
	"github.com/usehiveloop/hiveloop/internal/enqueue"
	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/hermes"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/registry"
	"github.com/usehiveloop/hiveloop/internal/sandbox"
)

type stubHermesProvider struct {
	mu             sync.Mutex
	endpoint       string
	failOnCreate   bool
	createdCount   int
	deletedCount   int
	lastCreateOpts sandbox.CreateSandboxOpts
}

func (s *stubHermesProvider) CreateSandbox(_ context.Context, opts sandbox.CreateSandboxOpts) (*sandbox.SandboxInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failOnCreate {
		return nil, errors.New("stub: provider create failed")
	}
	s.createdCount++
	s.lastCreateOpts = opts
	return &sandbox.SandboxInfo{
		ExternalID: fmt.Sprintf("stub-sb-%d", s.createdCount),
		Status:     sandbox.StatusRunning,
	}, nil
}

func (s *stubHermesProvider) GetEndpoint(_ context.Context, _ string, _ int) (string, error) {
	return s.endpoint, nil
}

func (s *stubHermesProvider) DeleteSandbox(_ context.Context, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deletedCount++
	return nil
}

func (s *stubHermesProvider) StartSandbox(context.Context, string) error   { return nil }
func (s *stubHermesProvider) StopSandbox(context.Context, string) error    { return nil }
func (s *stubHermesProvider) ArchiveSandbox(context.Context, string) error { return nil }
func (s *stubHermesProvider) GetStatus(context.Context, string) (sandbox.SandboxStatus, error) {
	return sandbox.StatusRunning, nil
}
func (s *stubHermesProvider) BuildSnapshot(context.Context, sandbox.BuildSnapshotOpts) (string, error) {
	return "", nil
}
func (s *stubHermesProvider) BuildSnapshotWithLogs(context.Context, sandbox.BuildSnapshotOpts, func(string)) (string, error) {
	return "", nil
}
func (s *stubHermesProvider) GetSnapshotStatus(context.Context, string) (*sandbox.SnapshotStatusResult, error) {
	return &sandbox.SnapshotStatusResult{State: "ready"}, nil
}
func (s *stubHermesProvider) GetSnapshotLogs(context.Context, string) (string, error) {
	return "", nil
}
func (s *stubHermesProvider) DeleteSnapshot(context.Context, string) error      { return nil }
func (s *stubHermesProvider) SetAutoStop(context.Context, string, int) error    { return nil }
func (s *stubHermesProvider) SetAutoArchive(context.Context, string, int) error { return nil }
func (s *stubHermesProvider) ExecuteCommand(context.Context, string, string) (string, error) {
	return "", nil
}

type employeeHarness struct {
	db        *gorm.DB
	router    *chi.Mux
	provider  *stubHermesProvider
	encKey    *crypto.SymmetricKey
	kms       *crypto.KeyWrapper
	sidecar   *sidecarStub
	sidecarSrv *httptest.Server
}

func newEmployeeHarness(t *testing.T) *employeeHarness {
	t.Helper()
	db := connectTestDB(t)
	if err := credentials.SeedPlatformOrg(db); err != nil {
		t.Fatalf("seed platform org: %v", err)
	}

	stub := &sidecarStub{}
	sidecarSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/hermes/status":
			if r.Header.Get("Authorization") == "" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"state":"awaiting_initial_config"}`))
		case "/v1/config/sync":
			stub.mu.Lock()
			stub.syncConfigCalls++
			stub.lastSyncBearer = r.Header.Get("Authorization")
			status := stub.syncConfigStatus
			errs := stub.syncConfigErrors
			stub.mu.Unlock()
			if status == 0 {
				status = http.StatusOK
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			body := map[string]any{
				"applied": 3, "deleted": 0, "repos_cloned": 1, "restart_triggered": true,
			}
			if len(errs) > 0 {
				body["errors"] = errs
			}
			_ = json.NewEncoder(w).Encode(body)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(sidecarSrv.Close)

	provider := &stubHermesProvider{endpoint: sidecarSrv.URL}
	encKey := newTestEncKey(t)
	kms := newTestKMS(t)

	cfg := &config.Config{
		HermesBaseImagePrefix: "hiveloop-hermes-test-small-v1",
		BridgeHost:            "cp.hiveloop.test",
		ProxyHost:             "proxy.hiveloop.test",
	}
	orch := sandbox.NewOrchestrator(db, provider, nil, encKey, cfg)

	compileDeps := hermes.CompileDeps{
		DB:         db,
		Picker:     credentials.NewPickerWithRegistry(db, registry.Global()),
		KMS:        kms,
		EncKey:     encKey,
		SigningKey: []byte("test-signing-key-32-bytes-long!!"),
		Cfg:        cfg,
	}
	agentH := handler.NewAgentHandler(db, registry.Global(), encKey, &enqueue.MockClient{})
	h := handler.NewEmployeeHandler(db, orch, compileDeps, agentH)

	r := chi.NewRouter()
	r.Route("/v1/employees", func(r chi.Router) {
		r.Use(middleware.ResolveOrgFromHeader(db))
		r.Get("/", h.List)
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireOrgAdmin(db))
			r.Post("/", h.Create)
			r.Post("/{id}/sync", h.Sync)
		})
	})
	r.Route("/v1/orgs/current/onboarding", func(r chi.Router) {
		r.Use(middleware.ResolveOrgFromHeader(db))
		r.Use(middleware.RequireOrgAdmin(db))
		r.Post("/complete", h.CompleteOnboarding)
	})

	return &employeeHarness{
		db: db, router: r, provider: provider,
		encKey: encKey, kms: kms,
		sidecar: stub, sidecarSrv: sidecarSrv,
	}
}


func newTestEncKey(t *testing.T) *crypto.SymmetricKey {
	t.Helper()
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = byte(i + 7)
	}
	sk, err := crypto.NewSymmetricKey(base64.StdEncoding.EncodeToString(raw))
	if err != nil {
		t.Fatalf("enckey: %v", err)
	}
	return sk
}

type orgWithMember struct {
	org  model.Org
	user model.User
}

func (h *employeeHarness) createOrg(t *testing.T) orgWithMember {
	return h.createOrgWithRole(t, "admin")
}

func (h *employeeHarness) createOrgWithRole(t *testing.T, role string) orgWithMember {
	t.Helper()
	user := model.User{Email: "emp-" + uuid.NewString()[:8] + "@test.com", Name: "T"}
	if err := h.db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	org := model.Org{Name: "emp-org-" + uuid.NewString()[:8], Active: true}
	if err := h.db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	mem := model.OrgMembership{UserID: user.ID, OrgID: org.ID, Role: role}
	if err := h.db.Create(&mem).Error; err != nil {
		t.Fatalf("create membership: %v", err)
	}
	orgID := org.ID
	userID := user.ID
	t.Cleanup(func() {
		h.db.Where("org_id = ?", orgID).Delete(&model.Sandbox{})
		h.db.Where("org_id = ?", orgID).Delete(&model.Agent{})
		h.db.Where("user_id = ?", userID).Delete(&model.OrgMembership{})
		h.db.Where("id = ?", orgID).Delete(&model.Org{})
		h.db.Where("id = ?", userID).Delete(&model.User{})
	})
	return orgWithMember{org: org, user: user}
}

func (h *employeeHarness) seedSystemCred(t *testing.T, providerID string, revoked bool) model.Credential {
	t.Helper()
	cred := model.Credential{
		OrgID:        credentials.PlatformOrgID,
		Label:        "sys-" + providerID,
		BaseURL:      "https://api.example.com",
		AuthScheme:   "bearer",
		EncryptedKey: []byte("enc"),
		WrappedDEK:   []byte("dek"),
		ProviderID:   providerID,
		IsSystem:     true,
	}
	if revoked {
		now := time.Now()
		cred.RevokedAt = &now
	}
	if err := h.db.Create(&cred).Error; err != nil {
		t.Fatalf("seed system cred %s: %v", providerID, err)
	}
	t.Cleanup(func() { h.db.Unscoped().Delete(&cred) })
	return cred
}

func (h *employeeHarness) post(t *testing.T, m orgWithMember, body any) *httptest.ResponseRecorder {
	t.Helper()
	buf := new(bytes.Buffer)
	_ = json.NewEncoder(buf).Encode(body)
	req := httptest.NewRequest("POST", "/v1/employees/", buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-ID", m.org.ID.String())
	req = middleware.WithAuthClaims(req, &auth.AuthClaims{
		UserID: m.user.ID.String(),
		OrgID:  m.org.ID.String(),
		Role:   "admin",
	})
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

func decodeEmployeeResp(t *testing.T, rr *httptest.ResponseRecorder) map[string]string {
	t.Helper()
	var out map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode body: %v (body=%s)", err, rr.Body.String())
	}
	return out
}

func validEmployeeBody() map[string]any {
	return map[string]any{
		"category":    "engineering",
		"name":        "agent-" + uuid.NewString()[:8],
		"avatar_url":  "https://cdn.example/a.png",
		"description": "a software engineer",
	}
}
