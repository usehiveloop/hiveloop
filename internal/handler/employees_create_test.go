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
	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/sandbox"
)

// stubHermesProvider is a sandbox.Provider that records CreateSandbox calls
// and can be flipped to fail on demand. Only the methods CreateHermesSandbox
// actually exercises return useful values; the rest satisfy the interface.
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

func (s *stubHermesProvider) StartSandbox(context.Context, string) error      { return nil }
func (s *stubHermesProvider) StopSandbox(context.Context, string) error       { return nil }
func (s *stubHermesProvider) ArchiveSandbox(context.Context, string) error    { return nil }
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
func (s *stubHermesProvider) DeleteSnapshot(context.Context, string) error            { return nil }
func (s *stubHermesProvider) SetAutoStop(context.Context, string, int) error          { return nil }
func (s *stubHermesProvider) SetAutoArchive(context.Context, string, int) error       { return nil }
func (s *stubHermesProvider) ExecuteCommand(context.Context, string, string) (string, error) {
	return "", nil
}

type employeeHarness struct {
	db       *gorm.DB
	router   *chi.Mux
	provider *stubHermesProvider
}

func newEmployeeHarness(t *testing.T) *employeeHarness {
	t.Helper()
	db := connectTestDB(t)
	if err := credentials.SeedPlatformOrg(db); err != nil {
		t.Fatalf("seed platform org: %v", err)
	}

	hermesSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/hermes/status" && r.Header.Get("Authorization") != "" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"state":"awaiting_initial_config"}`))
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(hermesSrv.Close)

	provider := &stubHermesProvider{endpoint: hermesSrv.URL}

	encKey := newTestEncKey(t)
	cfg := &config.Config{
		HermesBaseImagePrefix: "hiveloop-hermes-test-small-v1",
		BridgeHost:            "cp.hiveloop.test",
	}
	orch := sandbox.NewOrchestrator(db, provider, nil, encKey, cfg)
	h := handler.NewEmployeeHandler(db, orch)

	r := chi.NewRouter()
	r.Route("/v1/employees", func(r chi.Router) {
		r.Use(middleware.ResolveOrgFromHeader(db))
		r.Post("/", h.Create)
	})

	return &employeeHarness{db: db, router: r, provider: provider}
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
	t.Helper()
	user := model.User{Email: "emp-" + uuid.NewString()[:8] + "@test.com", Name: "T"}
	if err := h.db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	org := model.Org{Name: "emp-org-" + uuid.NewString()[:8], Active: true}
	if err := h.db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	mem := model.OrgMembership{UserID: user.ID, OrgID: org.ID, Role: "admin"}
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

func TestIntegration_EmployeesCreate_Engineering_Crof_HappyPath(t *testing.T) {
	h := newEmployeeHarness(t)
	org := h.createOrg(t)
	crof := h.seedSystemCred(t, "crof", false)

	body := validEmployeeBody()
	rr := h.post(t, org, body)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rr.Code, rr.Body.String())
	}
	resp := decodeEmployeeResp(t, rr)
	if resp["agent_id"] == "" || resp["sandbox_id"] == "" {
		t.Fatalf("response missing ids: %v", resp)
	}
	if resp["status"] != "running" {
		t.Errorf("status = %q, want running", resp["status"])
	}

	var agent model.Agent
	if err := h.db.Where("id = ?", resp["agent_id"]).First(&agent).Error; err != nil {
		t.Fatalf("load agent: %v", err)
	}
	if !agent.IsEmployee {
		t.Errorf("agent.is_employee = false, want true")
	}
	if agent.Harness != "hermes" {
		t.Errorf("agent.harness = %q, want hermes", agent.Harness)
	}
	if agent.Model != "deepseek-v4-pro-precision" {
		t.Errorf("agent.model = %q, want deepseek-v4-pro-precision (crof)", agent.Model)
	}
	if agent.CredentialID == nil || *agent.CredentialID != crof.ID {
		t.Errorf("agent.credential_id = %v, want %v (crof)", agent.CredentialID, crof.ID)
	}
	if agent.SystemPrompt == "" {
		t.Errorf("agent.system_prompt should be set to engineering placeholder")
	}
	if agent.Status != "active" {
		t.Errorf("agent.status = %q, want active", agent.Status)
	}
	if agent.Category == nil || *agent.Category != "engineering" {
		t.Errorf("agent.category = %v, want engineering", agent.Category)
	}

	var sb model.Sandbox
	if err := h.db.Where("id = ?", resp["sandbox_id"]).First(&sb).Error; err != nil {
		t.Fatalf("load sandbox: %v", err)
	}
	if sb.AgentID == nil || *sb.AgentID != agent.ID {
		t.Errorf("sandbox.agent_id mismatch")
	}
}

func TestIntegration_EmployeesCreate_FallsBackToOpenrouter(t *testing.T) {
	h := newEmployeeHarness(t)
	org := h.createOrg(t)
	or := h.seedSystemCred(t, "openrouter", false)

	rr := h.post(t, org, validEmployeeBody())
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rr.Code, rr.Body.String())
	}
	resp := decodeEmployeeResp(t, rr)

	var agent model.Agent
	h.db.Where("id = ?", resp["agent_id"]).First(&agent)
	if agent.Model != "deepseek/deepseek-v4-pro" {
		t.Errorf("agent.model = %q, want deepseek/deepseek-v4-pro (openrouter)", agent.Model)
	}
	if agent.CredentialID == nil || *agent.CredentialID != or.ID {
		t.Errorf("agent.credential_id mismatch: got %v want %v", agent.CredentialID, or.ID)
	}
}

func TestIntegration_EmployeesCreate_PrefersCrofWhenBothPresent(t *testing.T) {
	h := newEmployeeHarness(t)
	org := h.createOrg(t)
	crof := h.seedSystemCred(t, "crof", false)
	h.seedSystemCred(t, "openrouter", false)

	rr := h.post(t, org, validEmployeeBody())
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d: %s", rr.Code, rr.Body.String())
	}
	resp := decodeEmployeeResp(t, rr)

	var agent model.Agent
	h.db.Where("id = ?", resp["agent_id"]).First(&agent)
	if agent.Model != "deepseek-v4-pro-precision" {
		t.Errorf("crof should win: agent.model = %q", agent.Model)
	}
	if agent.CredentialID == nil || *agent.CredentialID != crof.ID {
		t.Errorf("crof should win: agent.credential_id = %v", agent.CredentialID)
	}
}

func TestIntegration_EmployeesCreate_SkipsRevokedCrof(t *testing.T) {
	h := newEmployeeHarness(t)
	org := h.createOrg(t)
	h.seedSystemCred(t, "crof", true) // revoked
	or := h.seedSystemCred(t, "openrouter", false)

	rr := h.post(t, org, validEmployeeBody())
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d: %s", rr.Code, rr.Body.String())
	}
	resp := decodeEmployeeResp(t, rr)

	var agent model.Agent
	h.db.Where("id = ?", resp["agent_id"]).First(&agent)
	if agent.Model != "deepseek/deepseek-v4-pro" {
		t.Errorf("revoked crof must be skipped: agent.model = %q", agent.Model)
	}
	if agent.CredentialID == nil || *agent.CredentialID != or.ID {
		t.Errorf("revoked crof must be skipped: cred = %v", agent.CredentialID)
	}
}

func TestIntegration_EmployeesCreate_NoSystemCredential_503(t *testing.T) {
	h := newEmployeeHarness(t)
	org := h.createOrg(t)

	rr := h.post(t, org, validEmployeeBody())
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503: %s", rr.Code, rr.Body.String())
	}

	var count int64
	h.db.Model(&model.Agent{}).Where("org_id = ?", org.org.ID).Count(&count)
	if count != 0 {
		t.Errorf("agent rows after 503 = %d, want 0", count)
	}
}

func TestIntegration_EmployeesCreate_NonEngineeringCategory_400(t *testing.T) {
	h := newEmployeeHarness(t)
	org := h.createOrg(t)
	h.seedSystemCred(t, "crof", false)

	body := validEmployeeBody()
	body["category"] = "design" // valid category, but not engineering
	rr := h.post(t, org, body)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rr.Code, rr.Body.String())
	}

	var count int64
	h.db.Model(&model.Agent{}).Where("org_id = ?", org.org.ID).Count(&count)
	if count != 0 {
		t.Errorf("agent rows after 400 = %d, want 0", count)
	}
}

func TestIntegration_EmployeesCreate_InvalidCategory_400(t *testing.T) {
	h := newEmployeeHarness(t)
	org := h.createOrg(t)
	h.seedSystemCred(t, "crof", false)

	body := validEmployeeBody()
	body["category"] = "not-a-real-category"
	rr := h.post(t, org, body)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rr.Code, rr.Body.String())
	}
}

func TestIntegration_EmployeesCreate_MissingFields_400(t *testing.T) {
	h := newEmployeeHarness(t)
	org := h.createOrg(t)
	h.seedSystemCred(t, "crof", false)

	cases := map[string]map[string]any{
		"missing name":        {"category": "engineering", "description": "desc"},
		"missing description": {"category": "engineering", "name": "n-" + uuid.NewString()[:8]},
		"missing category":    {"name": "n-" + uuid.NewString()[:8], "description": "desc"},
		"empty name":          {"category": "engineering", "name": "", "description": "desc"},
	}
	for label, body := range cases {
		t.Run(label, func(t *testing.T) {
			rr := h.post(t, org, body)
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", rr.Code, rr.Body.String())
			}
		})
	}
}

func TestIntegration_EmployeesCreate_RollbackOnSandboxFailure(t *testing.T) {
	h := newEmployeeHarness(t)
	org := h.createOrg(t)
	h.seedSystemCred(t, "crof", false)
	h.provider.failOnCreate = true

	rr := h.post(t, org, validEmployeeBody())
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500: %s", rr.Code, rr.Body.String())
	}

	var agentCount, sbCount int64
	h.db.Model(&model.Agent{}).Where("org_id = ?", org.org.ID).Count(&agentCount)
	h.db.Model(&model.Sandbox{}).Where("org_id = ?", org.org.ID).Count(&sbCount)
	if agentCount != 0 {
		t.Errorf("agent rows after rollback = %d, want 0", agentCount)
	}
	if sbCount != 0 {
		t.Errorf("sandbox rows after rollback = %d, want 0", sbCount)
	}
}
