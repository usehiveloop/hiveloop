package handler_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	"github.com/usehiveloop/hiveloop/internal/employeeruntime"
	"github.com/usehiveloop/hiveloop/internal/enqueue"
	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/registry"
	"github.com/usehiveloop/hiveloop/internal/sandbox"
)

type stubEmployeeProvider struct {
	mu             sync.Mutex
	endpoint       string
	failOnCreate   bool
	createdCount   int
	deletedCount   int
	lastCreateOpts sandbox.CreateSandboxOpts
}

type stubHermesProvider = stubEmployeeProvider

func (s *stubEmployeeProvider) CreateSandbox(_ context.Context, opts sandbox.CreateSandboxOpts) (*sandbox.SandboxInfo, error) {
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

func (s *stubEmployeeProvider) GetEndpoint(_ context.Context, _ string, _ int) (string, error) {
	return s.endpoint, nil
}

func (s *stubEmployeeProvider) DeleteSandbox(_ context.Context, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deletedCount++
	return nil
}

func (s *stubEmployeeProvider) StartSandbox(context.Context, string) error   { return nil }
func (s *stubEmployeeProvider) StopSandbox(context.Context, string) error    { return nil }
func (s *stubEmployeeProvider) ArchiveSandbox(context.Context, string) error { return nil }
func (s *stubEmployeeProvider) GetStatus(context.Context, string) (sandbox.SandboxStatus, error) {
	return sandbox.StatusRunning, nil
}
func (s *stubEmployeeProvider) BuildSnapshot(context.Context, sandbox.BuildSnapshotOpts) (string, error) {
	return "", nil
}
func (s *stubEmployeeProvider) BuildSnapshotWithLogs(context.Context, sandbox.BuildSnapshotOpts, func(string)) (string, error) {
	return "", nil
}
func (s *stubEmployeeProvider) GetSnapshotStatus(context.Context, string) (*sandbox.SnapshotStatusResult, error) {
	return &sandbox.SnapshotStatusResult{State: "ready"}, nil
}
func (s *stubEmployeeProvider) GetSnapshotLogs(context.Context, string) (string, error) {
	return "", nil
}
func (s *stubEmployeeProvider) DeleteSnapshot(context.Context, string) error      { return nil }
func (s *stubEmployeeProvider) SetAutoStop(context.Context, string, int) error    { return nil }
func (s *stubEmployeeProvider) SetAutoArchive(context.Context, string, int) error { return nil }
func (s *stubEmployeeProvider) ExecuteCommand(context.Context, string, string) (string, error) {
	return "", nil
}
func (s *stubEmployeeProvider) ExecuteCommandWithTimeout(ctx context.Context, externalID string, command string, _ time.Duration) (string, error) {
	return s.ExecuteCommand(ctx, externalID, command)
}

type employeeHarness struct {
	db         *gorm.DB
	router     *chi.Mux
	provider   *stubEmployeeProvider
	enqueuer   *enqueue.MockClient
	encKey     *crypto.SymmetricKey
	kms        *crypto.KeyWrapper
	cfg        *config.Config
	sidecar    *sidecarStub
	sidecarSrv *httptest.Server
}

func newEmployeeHarness(t *testing.T) *employeeHarness {
	t.Helper()
	db := connectTestDB(t)
	if err := credentials.SeedPlatformOrg(db); err != nil {
		t.Fatalf("seed platform org: %v", err)
	}
	defaultSkillNames := []string{
		"git-github",
		"asset-uploads",
		"agent-browser",
	}
	db.Unscoped().
		Where("org_id IS NULL AND (name IN ? OR slug IN ?)", defaultSkillNames, defaultSkillNames).
		Delete(&model.Skill{})

	stub := &sidecarStub{}
	sidecarSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`ok`))
		case "/readyz":
			if r.Header.Get("Authorization") == "" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.WriteHeader(http.StatusOK)
		case "/config":
			body, _ := io.ReadAll(r.Body)
			stub.mu.Lock()
			stub.syncConfigCalls++
			stub.lastSyncBearer = r.Header.Get("Authorization")
			stub.lastConfigBody = body
			status := stub.syncConfigStatus
			errs := stub.syncConfigErrors
			stub.mu.Unlock()
			if status == 0 {
				status = http.StatusOK
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			respBody := map[string]any{
				"applied": 3, "deleted": 0, "repos_cloned": 1, "restart_triggered": true,
			}
			if len(errs) > 0 {
				respBody["errors"] = errs
			}
			_ = json.NewEncoder(w).Encode(respBody)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(sidecarSrv.Close)

	provider := &stubEmployeeProvider{endpoint: sidecarSrv.URL}
	encKey := newTestEncKey(t)
	kms := newTestKMS(t)

	cfg := &config.Config{
		EmployeeSandboxBaseImagePrefix: "hiveloop-employee-sandbox-test-small-v1",
		BridgeHost:                     "cp.hiveloop.test",
		ProxyHost:                      "proxy.hiveloop.test",
	}
	orch := sandbox.NewOrchestrator(db, provider, nil, encKey, cfg)

	compileDeps := employeeruntime.CompileDeps{
		DB:         db,
		Picker:     credentials.NewPickerWithRegistry(db, registry.Global()),
		KMS:        kms,
		EncKey:     encKey,
		SigningKey: []byte("test-signing-key-32-bytes-long!!"),
		Cfg:        cfg,
	}
	enq := &enqueue.MockClient{}
	agentH := handler.NewAgentHandler(db, registry.Global(), encKey, enq)
	h := handler.NewEmployeeHandler(db, orch, compileDeps, agentH)
	h.SetEnqueuer(enq)

	r := chi.NewRouter()
	r.Route("/v1/employees", func(r chi.Router) {
		r.Use(middleware.ResolveOrgFromHeader(db))
		r.Get("/", h.List)
		r.Get("/{id}", h.Get)
		r.Get("/{id}/agent-templates", h.ListAgentTemplates)
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireOrgAdmin(db))
			r.Post("/", h.Create)
			r.Put("/{id}", h.Update)
			r.Post("/{id}/sync", h.Sync)
			r.Post("/{id}/agent-templates/{slug}/install", h.InstallAgentTemplate)
			r.Post("/{id}/sandbox/upgrade", h.StartSandboxUpgrade)
			r.Get("/{id}/sandbox/upgrades/{upgradeID}", h.GetSandboxUpgrade)
		})
	})
	r.Route("/v1/orgs/current/onboarding", func(r chi.Router) {
		r.Use(middleware.ResolveOrgFromHeader(db))
		r.Use(middleware.RequireOrgAdmin(db))
		r.Post("/complete", h.CompleteOnboarding)
	})

	return &employeeHarness{
		db: db, router: r, provider: provider, enqueuer: enq,
		encKey: encKey, kms: kms, cfg: cfg,
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

func (h *employeeHarness) seedGlobalSkill(t *testing.T, name, status string) model.Skill {
	t.Helper()
	skill := model.Skill{
		Slug:       name + "-" + uuid.NewString()[:8],
		Name:       name,
		SourceType: model.SkillSourceInline,
		Status:     status,
	}
	if err := h.db.Create(&skill).Error; err != nil {
		t.Fatalf("seed global skill %s: %v", name, err)
	}
	t.Cleanup(func() { h.db.Unscoped().Delete(&skill) })
	return skill
}

func validEmployeeBody() map[string]any {
	return map[string]any{
		"category":    "engineering",
		"name":        "agent-" + uuid.NewString()[:8],
		"avatar_url":  "https://cdn.example/a.png",
		"description": "a software engineer",
	}
}
