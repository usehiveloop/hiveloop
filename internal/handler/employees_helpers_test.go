package handler_test

import (
	"context"
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
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/credentials"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/employeeruntime"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/nango"
	"github.com/usehivy/hivy/internal/registry"
	"github.com/usehivy/hivy/internal/sandbox"
)

type stubEmployeeProvider struct {
	mu             sync.Mutex
	endpoint       string
	failOnCreate   bool
	createdCount   int
	deletedCount   int
	lastCreateOpts sandbox.CreateSandboxOpts
}

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
			if r.Method == http.MethodGet {
				stub.mu.Lock()
				body := append([]byte(nil), stub.lastConfigBody...)
				stub.mu.Unlock()
				if len(body) == 0 {
					body = []byte(`{}`)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(body)
				return
			}
			if r.Method == http.MethodPut {
				body, _ := io.ReadAll(r.Body)
				stub.mu.Lock()
				stub.syncConfigCalls++
				stub.lastSyncBearer = r.Header.Get("Authorization")
				stub.lastConfigBody = body
				status := stub.syncConfigStatus
				errs := append([]string(nil), stub.syncConfigErrors...)
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
				return
			}
			w.WriteHeader(http.StatusMethodNotAllowed)
		case "/config/env":
			if r.Method != http.MethodPut {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			body, _ := io.ReadAll(r.Body)
			stub.mu.Lock()
			stub.syncEnvCalls++
			stub.lastEnvBearer = r.Header.Get("Authorization")
			stub.lastEnvBody = body
			status := stub.syncConfigStatus
			stub.mu.Unlock()
			if status == 0 {
				status = http.StatusOK
			}
			var env map[string]string
			if err := json.Unmarshal(body, &env); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"applied_at": "2026-01-01T00:00:00Z",
				"key_count":  len(env),
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(sidecarSrv.Close)

	provider := &stubEmployeeProvider{endpoint: sidecarSrv.URL}
	encKey := newTestEncKey(t)
	kms := newTestKMS(t)

	cfg := &config.Config{
		EmployeeSandboxBaseImagePrefix: "hivy-employee-sandbox-test-small-v1",
		BridgeHost:                     "cp.hivy.test",
		ProxyHost:                      "proxy.hivy.test",
	}
	orch := sandbox.NewOrchestrator(db, provider, nil, encKey, cfg)
	nangoSrv := httptest.NewServer(newNangoConnMock(&nangoConnMockConfig{}))
	t.Cleanup(nangoSrv.Close)

	compileDeps := employeeruntime.CompileDeps{
		DB:         db,
		Picker:     credentials.NewPickerWithRegistry(db, registry.Global()),
		KMS:        kms,
		EncKey:     encKey,
		Nango:      nango.NewClient(nangoSrv.URL, "test-secret-key"),
		SigningKey: []byte("test-signing-key-32-bytes-long!!"),
		Cfg:        cfg,
	}
	enq := &enqueue.MockClient{}
	h := handler.NewEmployeeHandler(db, orch, compileDeps, registry.Global())
	h.SetEnqueuer(enq)

	r := chi.NewRouter()
	r.Route("/v1/employees", func(r chi.Router) {
		r.Use(middleware.ResolveOrgFromHeader(db))
		r.Get("/", h.List)
		r.Get("/{id}", h.Get)
		r.Get("/{id}/specialists", h.ListSpecialists)
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireOrgAdmin(db))
			r.Post("/{id}/sync", h.Sync)
			r.Post("/{id}/sandbox/upgrade", h.StartSandboxUpgrade)
			r.Get("/{id}/sandbox/upgrades/{upgradeID}", h.GetSandboxUpgrade)
		})
	})

	return &employeeHarness{
		db: db, router: r, provider: provider, enqueuer: enq,
		encKey: encKey, kms: kms, cfg: cfg,
		sidecar: stub, sidecarSrv: sidecarSrv,
	}
}
