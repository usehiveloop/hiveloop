package handler_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/credentials"
	"github.com/usehiveloop/hiveloop/internal/crypto"
	"github.com/usehiveloop/hiveloop/internal/model"
	slackprov "github.com/usehiveloop/hiveloop/internal/profiles/slack"
)

// sidecarStub records calls made to the fake sidecar so tests can assert
// behaviour. Methods are goroutine-safe.
type sidecarStub struct {
	mu               sync.Mutex
	syncConfigCalls  int
	lastSyncBearer   string
	syncConfigStatus int // override; default 200
	syncConfigErrors []string
}

func (s *sidecarStub) snapshot() (calls int, bearer string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.syncConfigCalls, s.lastSyncBearer
}

func (s *sidecarStub) setStatus(status int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.syncConfigStatus = status
}

// seedEmployeeAgent creates an Agent that's ready for hermes.Compile to
// succeed: is_employee=true, harness=hermes, points at a system credential.
func (h *employeeHarness) seedEmployeeAgent(t *testing.T, m orgWithMember) model.Agent {
	t.Helper()
	cred := h.seedSystemCred(t, "crof", false)

	encryptedKey, err := h.encKey.EncryptString("sk-crof-test")
	if err != nil {
		t.Fatalf("encrypt cred: %v", err)
	}
	// Override the cred's encrypted key so credentials.Resolve can decrypt
	// (seedSystemCred uses a placeholder).
	if err := h.db.Model(&cred).Update("encrypted_key", encryptedKey).Error; err != nil {
		t.Fatalf("update cred key: %v", err)
	}

	credID := cred.ID
	agent := model.Agent{
		OrgID:        &m.org.ID,
		Name:         "agent-" + uuid.NewString()[:8],
		IsEmployee:   true,
		Harness:      "hermes",
		Model:        "deepseek-v4-pro-precision",
		SystemPrompt: "you are a test employee",
		CredentialID: &credID,
		Status:       "active",
		Tools:        model.JSON{},
		McpServers:   model.JSON{},
		Skills:       model.JSON{},
		Integrations: model.JSON{},
		Resources:    model.JSON{},
		AgentConfig:  model.JSON{},
		Permissions:  model.JSON{},
	}
	if err := h.db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	agentID := agent.ID
	t.Cleanup(func() {
		h.db.Where("agent_id = ?", agentID).Delete(&model.AgentProfile{})
		h.db.Where("agent_id = ?", agentID).Delete(&model.Sandbox{})
		h.db.Where("meta->>'agent_id' = ?", agentID.String()).Delete(&model.Token{})
		h.db.Where("id = ?", agentID).Delete(&model.Agent{})
	})
	return agent
}

// seedSandbox creates a Sandbox row pointing at the harness's sidecar
// httptest server. Returns the row.
func (h *employeeHarness) seedSandbox(t *testing.T, m orgWithMember, agentID uuid.UUID) model.Sandbox {
	t.Helper()
	apiKey := "sidecar-test-key-" + uuid.NewString()[:8]
	encryptedKey, err := h.encKey.EncryptString(apiKey)
	if err != nil {
		t.Fatalf("encrypt sidecar key: %v", err)
	}
	sb := model.Sandbox{
		OrgID:                 &m.org.ID,
		AgentID:               &agentID,
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

// seedSlackProfile creates an active slack profile with KMS-wrapped secrets
// so mergeProfileSecrets can decrypt them during compile.
func (h *employeeHarness) seedSlackProfile(t *testing.T, m orgWithMember, agentID uuid.UUID) model.AgentProfile {
	t.Helper()
	dek, err := crypto.GenerateDEK()
	if err != nil {
		t.Fatalf("dek: %v", err)
	}
	wrappedDEK, err := h.kms.Wrap(context.Background(), dek)
	if err != nil {
		t.Fatalf("wrap: %v", err)
	}
	plain, _ := json.Marshal(slackprov.Secrets{
		BotToken: "xoxb-test", AppToken: "xapp-test",
	})
	enc, err := crypto.EncryptCredential(plain, dek)
	if err != nil {
		t.Fatalf("encrypt secrets: %v", err)
	}
	p := model.AgentProfile{
		OrgID: m.org.ID, AgentID: agentID,
		Provider: slackprov.Provider, Status: "active",
		EncryptedSecrets: enc, WrappedDEK: wrappedDEK,
	}
	if err := h.db.Create(&p).Error; err != nil {
		t.Fatalf("create slack profile: %v", err)
	}
	return p
}

// seedWhatsappProfile creates an active whatsapp profile. compile.go does not
// emit env for whatsapp today, so encrypted secrets are empty — the row
// satisfies the "must have an active slack/whatsapp profile" gate alone.
func (h *employeeHarness) seedWhatsappProfile(t *testing.T, m orgWithMember, agentID uuid.UUID) model.AgentProfile {
	t.Helper()
	p := model.AgentProfile{
		OrgID: m.org.ID, AgentID: agentID,
		Provider: "whatsapp", Status: "active",
	}
	if err := h.db.Create(&p).Error; err != nil {
		t.Fatalf("create whatsapp profile: %v", err)
	}
	return p
}

// platformCredCleanup deletes any platform-org-scoped credentials the test
// inserted; called by happy-path tests so re-runs don't accumulate state.
func (h *employeeHarness) platformCredCleanup(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		h.db.Unscoped().Where("org_id = ?", credentials.PlatformOrgID).Delete(&model.Credential{})
	})
}
