package tasks

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/employeeruntime"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/nango"
	"github.com/usehivy/hivy/internal/sandbox"
)

type employeeUpgradeFixture struct {
	db       *gorm.DB
	server   *httptest.Server
	provider *employeeUpgradeProvider
	enqueuer *enqueue.MockClient
	handler  *EmployeeSandboxUpgradeHandler
	org      model.Org
	agent    model.Employee
	old      model.Sandbox
	upgrade  model.EmployeeSandboxUpgrade
}

func newEmployeeUpgradeFixture(t *testing.T) *employeeUpgradeFixture {
	t.Helper()
	db := openTasksMemoryTestDB(t)
	encKey := testTasksEncKey(t)
	kms := testTasksKMS(t)
	cfg := &config.Config{
		SandboxesRuntimeBaseImage: "ghcr.io/usehivy/hivy-sandboxes-runtime:test-v2",
		SpecialistSandboxHost:           "cp.hivy.test",
		ProxyHost:                       "proxy.hivy.test",
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusOK)
		case "/readyz":
			if r.Header.Get("Authorization") == "" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.WriteHeader(http.StatusOK)
		case "/config":
			if r.Header.Get("Authorization") == "" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"applied":1}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	nangoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/connection/") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"credentials":{"bot_token":"xoxb-test-token"}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(nangoServer.Close)

	provider := &employeeUpgradeProvider{endpoint: server.URL}
	enqueuer := &enqueue.MockClient{}
	orch := sandbox.NewOrchestrator(db, provider, encKey, cfg)
	org := model.Org{Name: "upgrade-org-" + uuid.NewString()[:8], Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	user := model.User{Email: "upgrade-" + uuid.NewString()[:8] + "@test.com", Name: "Upgrade User"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	cred := model.Credential{
		OrgID:        &org.ID,
		Label:        "employee-upgrade",
		BaseURL:      "https://proxy.test",
		AuthScheme:   "bearer",
		EncryptedKey: []byte("enc"),
		WrappedDEK:   []byte("dek"),
		ProviderID:   "openrouter",
	}
	if err := db.Create(&cred).Error; err != nil {
		t.Fatalf("create credential: %v", err)
	}
	agent := model.Employee{
		OrgID:         &org.ID,
		Name:          "employee-" + uuid.NewString()[:8],
		IsEmployee:    true,
		Harness:       "employee-sandbox",
		Model:         "deepseek-v4-flash",
		CredentialID:  &cred.ID,
		Status:        "active",
		SystemPrompt:  "test employee",
		Tools:         model.JSON{},
		McpServers:    model.JSON{},
		Skills:        model.JSON{},
		Integrations:  model.JSON{},
		Resources:     model.JSON{},
		RuntimeConfig: model.JSON{},
		Permissions:   model.JSON{},
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	bridgeKey := "runtime-secret-" + uuid.NewString()
	encryptedKey, err := encKey.EncryptString(bridgeKey)
	if err != nil {
		t.Fatalf("encrypt runtime secret: %v", err)
	}
	old := model.Sandbox{
		OrgID:                  &org.ID,
		EmployeeID:             &agent.ID,
		SnapshotID:             &cfg.SandboxesRuntimeBaseImage,
		ExternalID:             "old-external",
		RuntimeURL:             server.URL,
		EncryptedRuntimeSecret: encryptedKey,
		Status:                 "running",
	}
	if err := db.Create(&old).Error; err != nil {
		t.Fatalf("create old sandbox: %v", err)
	}
	upgrade := model.EmployeeSandboxUpgrade{
		OrgID:        org.ID,
		EmployeeID:   agent.ID,
		OldSandboxID: &old.ID,
		Status:       model.EmployeeSandboxUpgradeStatusQueued,
		Phase:        model.EmployeeSandboxUpgradePhaseQueued,
	}
	if err := db.Create(&upgrade).Error; err != nil {
		t.Fatalf("create upgrade: %v", err)
	}
	handler := NewEmployeeSandboxUpgradeHandler(db, orch, employeeruntime.CompileDeps{
		DB:         db,
		KMS:        kms,
		EncKey:     encKey,
		Nango:      nango.NewClient(nangoServer.URL, "test-secret-key"),
		SigningKey: []byte("test-signing-key-32-bytes-long!!"),
		Cfg:        cfg,
	}, enqueuer)
	t.Cleanup(func() {
		db.Where("org_id = ?", org.ID).Delete(&model.EmployeeSandboxUpgrade{})
		db.Where("org_id = ?", org.ID).Delete(&model.Sandbox{})
		db.Where("org_id = ?", org.ID).Delete(&model.Connection{})
		db.Where("org_id = ?", org.ID).Delete(&model.Employee{})
		db.Where("org_id = ?", org.ID).Delete(&model.Credential{})
		db.Where("id = ?", org.ID).Delete(&model.Org{})
		db.Where("id = ?", user.ID).Delete(&model.User{})
	})
	return &employeeUpgradeFixture{
		db: db, server: server, provider: provider, enqueuer: enqueuer, handler: handler,
		org: org, agent: agent, old: old, upgrade: upgrade,
	}
}

func testTasksKMS(t *testing.T) *crypto.KeyWrapper {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(200 - i)
	}
	kms, err := crypto.NewAEADWrapper(context.Background(), base64.StdEncoding.EncodeToString(key), "employee-upgrade-test")
	if err != nil {
		t.Fatalf("new kms: %v", err)
	}
	return kms
}

func employeeUpgradeTask(t *testing.T, upgradeID, agentID uuid.UUID) *asynq.Task {
	t.Helper()
	task, _, err := NewEmployeeSandboxUpgradeTask(upgradeID, agentID)
	if err != nil {
		t.Fatalf("new task: %v", err)
	}
	return task
}
