package tasks

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/config"
	"github.com/usehiveloop/hiveloop/internal/employeeruntime"
	"github.com/usehiveloop/hiveloop/internal/enqueue"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/sandbox"
)

type proxyRefreshRuntime struct {
	mu          sync.Mutex
	envCalls    int
	lastEnv     map[string]string
	envStatus   int
	readyStatus int
}

func TestEmployeeProxyTokenRefreshHandler_InjectsNewTokenRevokesOldAndSchedulesNext(t *testing.T) {
	f := newEmployeeProxyTokenRefreshFixture(t, 0)
	oldToken, err := employeeruntime.MintProxyToken(context.Background(), f.compileDeps, &f.agent, f.sandbox.ID)
	if err != nil {
		t.Fatalf("mint old proxy token: %v", err)
	}

	task, _, err := NewEmployeeProxyTokenRefreshTask(EmployeeProxyTokenRefreshPayload{
		AgentID:     f.agent.ID,
		SandboxID:   f.sandbox.ID,
		ScheduledAt: oldToken.ExpiresAt.Add(-employeeProxyTokenRefreshLead),
	})
	if err != nil {
		t.Fatalf("new refresh task: %v", err)
	}
	if err := f.handler.Handle(context.Background(), task); err != nil {
		t.Fatalf("handle refresh: %v", err)
	}

	f.runtime.mu.Lock()
	envCalls := f.runtime.envCalls
	injected := f.runtime.lastEnv[employeeruntime.ProxyAPIKeyEnv]
	f.runtime.mu.Unlock()
	if envCalls != 1 {
		t.Fatalf("runtime env calls = %d, want 1", envCalls)
	}
	if !strings.HasPrefix(injected, "ptok_") || injected == oldToken.Token {
		t.Fatalf("injected proxy token was not refreshed: %q", injected)
	}

	var old model.Token
	if err := f.db.First(&old, "jti = ?", oldToken.JTI).Error; err != nil {
		t.Fatalf("load old token: %v", err)
	}
	if old.RevokedAt == nil {
		t.Fatal("old token was not revoked")
	}
	var activeCount int64
	if err := f.db.Model(&model.Token{}).
		Where("org_id = ? AND meta->>'agent_id' = ? AND meta->>'harness' = ? AND revoked_at IS NULL",
			f.org.ID, f.agent.ID.String(), "employee-sandbox").
		Count(&activeCount).Error; err != nil {
		t.Fatalf("count active tokens: %v", err)
	}
	if activeCount != 1 {
		t.Fatalf("active employee proxy tokens = %d, want 1", activeCount)
	}
	var refreshedAgent model.Agent
	if err := f.db.First(&refreshedAgent, "id = ?", f.agent.ID).Error; err != nil {
		t.Fatalf("load refreshed agent: %v", err)
	}
	if refreshedAgent.LastProxyTokenRefreshedAt == nil {
		t.Fatal("last_proxy_token_refreshed_at was not set")
	}

	refreshTask := requireProxyRefreshTask(t, f.enqueuer)
	requireOption(t, refreshTask.Options, asynq.QueueOpt, QueueDefault)
	requireOption(t, refreshTask.Options, asynq.TimeoutOpt, employeeProxyTokenRefreshTimeout)
}

func TestEmployeeProxyTokenRefreshHandler_RevokesMintedTokenWhenRuntimeRejectsEnv(t *testing.T) {
	f := newEmployeeProxyTokenRefreshFixture(t, http.StatusInternalServerError)
	if _, err := employeeruntime.MintProxyToken(context.Background(), f.compileDeps, &f.agent, f.sandbox.ID); err != nil {
		t.Fatalf("mint old proxy token: %v", err)
	}
	task, _, err := NewEmployeeProxyTokenRefreshTask(EmployeeProxyTokenRefreshPayload{
		AgentID:   f.agent.ID,
		SandboxID: f.sandbox.ID,
	})
	if err != nil {
		t.Fatalf("new refresh task: %v", err)
	}
	if err := f.handler.Handle(context.Background(), task); err == nil {
		t.Fatal("expected runtime env failure")
	}

	var revokedCount int64
	if err := f.db.Model(&model.Token{}).
		Where("org_id = ? AND meta->>'agent_id' = ? AND meta->>'harness' = ? AND revoked_at IS NOT NULL",
			f.org.ID, f.agent.ID.String(), "employee-sandbox").
		Count(&revokedCount).Error; err != nil {
		t.Fatalf("count revoked tokens: %v", err)
	}
	if revokedCount == 0 {
		t.Fatal("failed refresh token was not revoked")
	}
	if len(f.enqueuer.Tasks()) != 0 {
		t.Fatalf("next refresh should not be scheduled after failure: %v", f.enqueuer.Tasks())
	}
}

type employeeProxyTokenRefreshFixture struct {
	db          *gorm.DB
	server      *httptest.Server
	runtime     *proxyRefreshRuntime
	enqueuer    *enqueue.MockClient
	handler     *EmployeeProxyTokenRefreshHandler
	compileDeps employeeruntime.CompileDeps
	org         model.Org
	agent       model.Agent
	sandbox     model.Sandbox
}

func newEmployeeProxyTokenRefreshFixture(t *testing.T, envStatus int) *employeeProxyTokenRefreshFixture {
	t.Helper()
	db := openTasksMemoryTestDB(t)
	encKey := testTasksEncKey(t)
	cfg := &config.Config{ProxyHost: "proxy.hiveloop.test"}
	runtime := &proxyRefreshRuntime{envStatus: envStatus}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusOK)
		case "/readyz":
			w.WriteHeader(http.StatusOK)
		case "/config/env":
			var env map[string]string
			if err := json.NewDecoder(r.Body).Decode(&env); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			runtime.mu.Lock()
			runtime.envCalls++
			runtime.lastEnv = env
			status := runtime.envStatus
			runtime.mu.Unlock()
			if status == 0 {
				status = http.StatusOK
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]any{"key_count": len(env)})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	org := model.Org{Name: "proxy-refresh-org-" + uuid.NewString()[:8], Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	cred := model.Credential{
		OrgID:        org.ID,
		Label:        "proxy-refresh",
		BaseURL:      "https://proxy.test",
		AuthScheme:   "bearer",
		EncryptedKey: []byte("enc"),
		WrappedDEK:   []byte("dek"),
		ProviderID:   "openrouter",
	}
	if err := db.Create(&cred).Error; err != nil {
		t.Fatalf("create credential: %v", err)
	}
	agent := model.Agent{
		OrgID:        &org.ID,
		Name:         "employee-" + uuid.NewString()[:8],
		IsEmployee:   true,
		Harness:      "employee-sandbox",
		Model:        employeeruntime.DefaultEmployeeModel,
		CredentialID: &cred.ID,
		Status:       "active",
		SystemPrompt: "test employee",
		Tools:        model.JSON{},
		McpServers:   model.JSON{},
		Skills:       model.JSON{},
		Integrations: model.JSON{},
		Resources:    model.JSON{},
		AgentConfig:  model.JSON{},
		Permissions:  model.JSON{},
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	bridgeKey := "runtime-secret-" + uuid.NewString()
	encryptedKey, err := encKey.EncryptString(bridgeKey)
	if err != nil {
		t.Fatalf("encrypt runtime secret: %v", err)
	}
	sb := model.Sandbox{
		OrgID:                 &org.ID,
		AgentID:               &agent.ID,
		ExternalID:            "proxy-refresh-external",
		BridgeURL:             server.URL,
		EncryptedBridgeAPIKey: encryptedKey,
		Status:                string(sandbox.StatusRunning),
	}
	if err := db.Create(&sb).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	enqueuer := &enqueue.MockClient{}
	provider := &employeeUpgradeProvider{endpoint: server.URL}
	orch := sandbox.NewOrchestrator(db, provider, nil, encKey, cfg)
	compileDeps := employeeruntime.CompileDeps{
		DB:         db,
		EncKey:     encKey,
		SigningKey: []byte("test-signing-key-32-bytes-long!!"),
		Cfg:        cfg,
	}
	handler := NewEmployeeProxyTokenRefreshHandler(db, orch, compileDeps, enqueuer)
	t.Cleanup(func() {
		db.Where("org_id = ?", org.ID).Delete(&model.Token{})
		db.Where("org_id = ?", org.ID).Delete(&model.Sandbox{})
		db.Where("org_id = ?", org.ID).Delete(&model.Agent{})
		db.Where("org_id = ?", org.ID).Delete(&model.Credential{})
		db.Where("id = ?", org.ID).Delete(&model.Org{})
	})
	return &employeeProxyTokenRefreshFixture{
		db: db, server: server, runtime: runtime, enqueuer: enqueuer,
		handler: handler, compileDeps: compileDeps, org: org, agent: agent, sandbox: sb,
	}
}

func requireProxyRefreshTask(t *testing.T, enqueuer *enqueue.MockClient) enqueue.EnqueuedTask {
	t.Helper()
	for _, task := range enqueuer.Tasks() {
		if task.TypeName == TypeEmployeeProxyTokenRefresh {
			return task
		}
	}
	t.Fatalf("expected %s task to be enqueued", TypeEmployeeProxyTokenRefresh)
	return enqueue.EnqueuedTask{}
}
