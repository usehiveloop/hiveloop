package handler_test

import (
	"encoding/json"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/specialists"
)

type sidecarStub struct {
	mu               sync.Mutex
	syncConfigCalls  int
	syncEnvCalls     int
	lastSyncBearer   string
	lastEnvBearer    string
	lastConfigBody   []byte
	lastEnvBody      []byte
	syncConfigStatus int // override; default 200
	syncConfigErrors []string
}

func (s *sidecarStub) snapshot() (calls int, bearer string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.syncConfigCalls, s.lastSyncBearer
}

func (s *sidecarStub) configBody() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]byte(nil), s.lastConfigBody...)
}

func (s *sidecarStub) envBody() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]byte(nil), s.lastEnvBody...)
}

func (s *sidecarStub) snapshotRuntime() (calls int, bearer string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.syncEnvCalls, s.lastEnvBearer
}

func (s *sidecarStub) setStatus(status int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.syncConfigStatus = status
}

func (h *employeeHarness) seedEmployeeAgent(t *testing.T, m orgWithMember) model.Employee {
	t.Helper()
	cred := h.seedSystemCred(t, "openrouter", false)

	encryptedKey, err := h.encKey.EncryptString("sk-openrouter-test")
	if err != nil {
		t.Fatalf("encrypt cred: %v", err)
	}
	// seedSystemCred writes a placeholder encrypted_key; replace it so
	// credentials.Resolve can decrypt with h.encKey during compile.
	if err := h.db.Model(&cred).Update("encrypted_key", encryptedKey).Error; err != nil {
		t.Fatalf("update cred key: %v", err)
	}

	credID := cred.ID
	catalog, err := specialists.Load("global/specialists")
	if err != nil {
		t.Fatalf("load specialists: %v", err)
	}
	agent := model.Employee{
		OrgID:               &m.org.ID,
		Name:                "agent-" + uuid.NewString()[:8],
		IsEmployee:          true,
		Harness:             "employee-sandbox",
		Model:               "deepseek-v4-flash",
		SystemPrompt:        "you are a test employee",
		CredentialID:        &credID,
		Status:              "active",
		AttachedSpecialists: pq.StringArray(catalog.AutoAttachSlugs()),
		Tools:               model.JSON{},
		McpServers:          model.JSON{},
		Skills:              model.JSON{},
		Integrations:        model.JSON{},
		Resources:           model.JSON{},
		RuntimeConfig:       model.JSON{},
		Permissions:         model.JSON{},
	}
	if err := h.db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	agentID := agent.ID
	t.Cleanup(func() {
		h.db.Where("employee_id = ?", agentID).Delete(&model.Sandbox{})
		h.db.Where("meta->>'employee_id' = ?", agentID.String()).Delete(&model.Token{})
		h.db.Where("id = ?", agentID).Delete(&model.Employee{})
	})
	return agent
}

func (h *employeeHarness) seedSandbox(t *testing.T, m orgWithMember, agentID uuid.UUID) model.Sandbox {
	t.Helper()
	apiKey := "sidecar-test-key-" + uuid.NewString()[:8]
	encryptedKey, err := h.encKey.EncryptString(apiKey)
	if err != nil {
		t.Fatalf("encrypt sidecar key: %v", err)
	}
	sb := model.Sandbox{
		OrgID:                 &m.org.ID,
		EmployeeID:            &agentID,
		SnapshotID:            &h.cfg.SandboxesRuntimeBaseImagePrefix,
		ExternalID:            "stub-sb-" + uuid.NewString()[:8],
		BridgeURL:             h.sidecarSrv.URL,
		EncryptedBridgeAPIKey: encryptedKey,
		Status:                "running",
	}
	if err := h.db.Create(&sb).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	return sb
}

func (h *employeeHarness) setSandboxSnapshot(t *testing.T, sandboxID uuid.UUID, snapshotID *string) {
	t.Helper()
	var value any
	if snapshotID != nil {
		value = *snapshotID
	}
	if err := h.db.Model(&model.Sandbox{}).
		Where("id = ?", sandboxID).
		Updates(map[string]any{"snapshot_id": value}).Error; err != nil {
		t.Fatalf("set sandbox snapshot: %v", err)
	}
}

func (h *employeeHarness) setRuntimeEnvVars(t *testing.T, agentID uuid.UUID, vars map[string]string) {
	t.Helper()
	payload, err := json.Marshal(vars)
	if err != nil {
		t.Fatalf("marshal env vars: %v", err)
	}
	encrypted, err := h.encKey.EncryptString(string(payload))
	if err != nil {
		t.Fatalf("encrypt env vars: %v", err)
	}
	if err := h.db.Model(&model.Employee{}).
		Where("id = ?", agentID).
		Update("encrypted_env_vars", []byte(encrypted)).
		Error; err != nil {
		t.Fatalf("store env vars: %v", err)
	}
}

func (h *employeeHarness) seedWhatsappProfile(t *testing.T, m orgWithMember, agentID uuid.UUID) model.Connection {
	t.Helper()
	integ := model.Integration{
		ID:          uuid.New(),
		UniqueKey:   "whatsapp-test-" + uuid.NewString()[:8],
		Provider:    "whatsapp",
		DisplayName: "WhatsApp",
	}
	if err := h.db.Create(&integ).Error; err != nil {
		t.Fatalf("create whatsapp integration: %v", err)
	}
	conn := model.Connection{
		ID:                uuid.New(),
		OrgID:             m.org.ID,
		UserID:            m.user.ID,
		IntegrationID:     integ.ID,
		NangoConnectionID: "whatsapp-conn-test",
		Meta:              model.JSON{},
	}
	if err := h.db.Create(&conn).Error; err != nil {
		t.Fatalf("create whatsapp connection: %v", err)
	}
	conn.Integration = integ
	return conn
}

func (h *employeeHarness) platformCredCleanup(t *testing.T) {
	t.Helper()
	h.db.Unscoped().Where("org_id IS NULL").Delete(&model.Credential{})
	t.Cleanup(func() {
		h.db.Unscoped().Where("org_id IS NULL").Delete(&model.Credential{})
	})
}
