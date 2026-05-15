package tasks

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/config"
	"github.com/usehiveloop/hiveloop/internal/crypto"
	"github.com/usehiveloop/hiveloop/internal/employeeruntime"
	"github.com/usehiveloop/hiveloop/internal/enqueue"
	"github.com/usehiveloop/hiveloop/internal/model"
	slackprov "github.com/usehiveloop/hiveloop/internal/profiles/slack"
	"github.com/usehiveloop/hiveloop/internal/sandbox"
)

type employeeUpgradeFixture struct {
	db       *gorm.DB
	server   *httptest.Server
	provider *employeeUpgradeProvider
	enqueuer *enqueue.MockClient
	handler  *EmployeeSandboxUpgradeHandler
	org      model.Org
	agent    model.Agent
	old      model.Sandbox
	upgrade  model.EmployeeSandboxUpgrade
}

func newEmployeeUpgradeFixture(t *testing.T) *employeeUpgradeFixture {
	t.Helper()
	db := openTasksMemoryTestDB(t)
	encKey := testTasksEncKey(t)
	kms := testTasksKMS(t)
	cfg := &config.Config{
		EmployeeSandboxBaseImagePrefix: "employee-runtime-test-v2",
		BridgeHost:                     "cp.hiveloop.test",
		ProxyHost:                      "proxy.hiveloop.test",
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

	provider := &employeeUpgradeProvider{endpoint: server.URL}
	enqueuer := &enqueue.MockClient{}
	orch := sandbox.NewOrchestrator(db, provider, nil, encKey, cfg)
	org := model.Org{Name: "upgrade-org-" + uuid.NewString()[:8], Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	cred := model.Credential{
		OrgID:        org.ID,
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
	agent := model.Agent{
		OrgID:        &org.ID,
		Name:         "employee-" + uuid.NewString()[:8],
		IsEmployee:   true,
		Harness:      "employee-sandbox",
		Model:        "deepseek/deepseek-v4-flash",
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
	seedEmployeeUpgradeSlackProfile(t, db, kms, org.ID, agent.ID)
	bridgeKey := "runtime-secret-" + uuid.NewString()
	encryptedKey, err := encKey.EncryptString(bridgeKey)
	if err != nil {
		t.Fatalf("encrypt runtime secret: %v", err)
	}
	old := model.Sandbox{
		OrgID:                 &org.ID,
		AgentID:               &agent.ID,
		SnapshotID:            &cfg.EmployeeSandboxBaseImagePrefix,
		ExternalID:            "old-external",
		BridgeURL:             server.URL,
		EncryptedBridgeAPIKey: encryptedKey,
		Status:                "running",
	}
	if err := db.Create(&old).Error; err != nil {
		t.Fatalf("create old sandbox: %v", err)
	}
	upgrade := model.EmployeeSandboxUpgrade{
		OrgID:        org.ID,
		AgentID:      agent.ID,
		OldSandboxID: &old.ID,
		Status:       model.EmployeeSandboxUpgradeStatusQueued,
		Phase:        model.EmployeeSandboxUpgradePhaseQueued,
	}
	if err := db.Create(&upgrade).Error; err != nil {
		t.Fatalf("create upgrade: %v", err)
	}
	handler := NewEmployeeSandboxUpgradeHandler(db, orch, fakeEmployeeUpgradeStore{size: 12}, employeeruntime.CompileDeps{
		DB:         db,
		KMS:        kms,
		EncKey:     encKey,
		SigningKey: []byte("test-signing-key-32-bytes-long!!"),
		Cfg:        cfg,
	}, enqueuer)
	t.Cleanup(func() {
		db.Where("org_id = ?", org.ID).Delete(&model.EmployeeSandboxUpgrade{})
		db.Where("org_id = ?", org.ID).Delete(&model.Sandbox{})
		db.Where("org_id = ?", org.ID).Delete(&model.AgentProfile{})
		db.Where("org_id = ?", org.ID).Delete(&model.Agent{})
		db.Where("org_id = ?", org.ID).Delete(&model.Credential{})
		db.Where("id = ?", org.ID).Delete(&model.Org{})
	})
	return &employeeUpgradeFixture{
		db: db, server: server, provider: provider, enqueuer: enqueuer, handler: handler,
		org: org, agent: agent, old: old, upgrade: upgrade,
	}
}

func seedEmployeeUpgradeSlackProfile(t *testing.T, db *gorm.DB, kms *crypto.KeyWrapper, orgID, agentID uuid.UUID) {
	t.Helper()
	dek, err := crypto.GenerateDEK()
	if err != nil {
		t.Fatalf("generate dek: %v", err)
	}
	wrapped, err := kms.Wrap(context.Background(), dek)
	if err != nil {
		t.Fatalf("wrap dek: %v", err)
	}
	plain, _ := json.Marshal(slackprov.Secrets{BotToken: "xoxb-test", AppToken: "xapp-test"})
	encrypted, err := crypto.EncryptCredential(plain, dek)
	if err != nil {
		t.Fatalf("encrypt slack secrets: %v", err)
	}
	profile := model.AgentProfile{
		OrgID:            orgID,
		AgentID:          agentID,
		Provider:         slackprov.Provider,
		Status:           "active",
		EncryptedSecrets: encrypted,
		WrappedDEK:       wrapped,
	}
	if err := db.Create(&profile).Error; err != nil {
		t.Fatalf("create slack profile: %v", err)
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
	task, _, err := NewEmployeeSandboxUpgradeTask(upgradeID, agentID, false)
	if err != nil {
		t.Fatalf("new task: %v", err)
	}
	return task
}
