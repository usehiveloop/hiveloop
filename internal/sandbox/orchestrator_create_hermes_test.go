package sandbox

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/config"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// setupHermesOrchestrator builds an Orchestrator pointed at a stand-in
// sidecar httptest server. lastAuth records the Authorization header from
// the most recent /v1/hermes/status call so tests can assert the orchestrator
// sent a Bearer token. hermesStatusFn lets a test return a non-200 to drive
// the failure branch.
func setupHermesOrchestrator(t *testing.T, hermesStatusFn http.HandlerFunc) (*Orchestrator, *mockProvider, *gorm.DB, string, *atomic.Value) {
	t.Helper()
	db := setupTestDB(t)

	var lastAuth atomic.Value
	lastAuth.Store("")

	hermesSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/hermes/status" {
			http.NotFound(w, r)
			return
		}
		lastAuth.Store(r.Header.Get("Authorization"))
		if hermesStatusFn != nil {
			hermesStatusFn(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"state":"awaiting_initial_config"}`))
	}))
	t.Cleanup(hermesSrv.Close)

	provider := newMockProvider()
	provider.endpointOverride = hermesSrv.URL

	cfg := &config.Config{
		HermesBaseImagePrefix: "hiveloop-hermes-0-0-2-small-v1",
		BridgeHost:            "cp.hiveloop.test",
	}

	orch := NewOrchestrator(db, provider, nil, testEncKey(t), cfg)

	t.Cleanup(func() {
		db.Exec("DELETE FROM sandboxes")
	})

	return orch, provider, db, hermesSrv.URL, &lastAuth
}

func TestCreateHermesSandbox_PersistsRowAndReturnsRunning(t *testing.T) {
	orch, provider, db, hermesURL, lastAuth := setupHermesOrchestrator(t, nil)
	org := createTestOrg(t, db)
	cred := createTestCred(t, db, org.ID)
	agent := createTestAgent(t, db, org.ID, cred.ID)

	sb, err := orch.CreateHermesSandbox(context.Background(), &agent)
	if err != nil {
		t.Fatalf("CreateHermesSandbox: %v", err)
	}

	if sb.AgentID == nil || *sb.AgentID != agent.ID {
		t.Errorf("sandbox.agent_id mismatch: got %v, want %v", sb.AgentID, agent.ID)
	}
	if sb.OrgID == nil || *sb.OrgID != org.ID {
		t.Errorf("sandbox.org_id mismatch")
	}
	if sb.Status != "running" {
		t.Errorf("sandbox.status = %q, want running", sb.Status)
	}
	if sb.BridgeURL != hermesURL {
		t.Errorf("sandbox.bridge_url = %q, want %q", sb.BridgeURL, hermesURL)
	}
	if sb.ExternalID == "" {
		t.Errorf("sandbox.external_id should be set")
	}
	if len(sb.EncryptedBridgeAPIKey) == 0 {
		t.Errorf("sandbox.encrypted_bridge_api_key should be set")
	}

	// Re-read from DB to confirm persistence.
	var fromDB model.Sandbox
	if err := db.Where("id = ?", sb.ID).First(&fromDB).Error; err != nil {
		t.Fatalf("re-read sandbox: %v", err)
	}
	if fromDB.Status != "running" {
		t.Errorf("DB sandbox.status = %q, want running", fromDB.Status)
	}

	// Decryption round-trip — proves the bearer is recoverable for SyncConfig.
	apiKey, err := orch.encKey.DecryptString(fromDB.EncryptedBridgeAPIKey)
	if err != nil {
		t.Fatalf("decrypt sidecar api key: %v", err)
	}
	if len(apiKey) != 64 {
		t.Errorf("sidecar api key length = %d, want 64 (32-byte hex)", len(apiKey))
	}

	// waitForSidecarReady must call the sidecar with the same bearer token
	// that was injected into CONTROL_PLANE_API_KEY — otherwise CP and the
	// sidecar are out of sync and SyncConfig will 401 later.
	if got, want := lastAuth.Load().(string), "Bearer "+apiKey; got != want {
		t.Errorf("Authorization header on /v1/hermes/status = %q, want %q", got, want)
	}

	if provider.count() != 1 {
		t.Errorf("provider sandbox count = %d, want 1", provider.count())
	}
}

func TestCreateHermesSandbox_InjectsRequiredEnvVarsAndUsesPort7777(t *testing.T) {
	orch, provider, db, _, _ := setupHermesOrchestrator(t, nil)
	org := createTestOrg(t, db)
	cred := createTestCred(t, db, org.ID)
	agent := createTestAgent(t, db, org.ID, cred.ID)

	sb, err := orch.CreateHermesSandbox(context.Background(), &agent)
	if err != nil {
		t.Fatalf("CreateHermesSandbox: %v", err)
	}

	if len(provider.createCalls) != 1 {
		t.Fatalf("provider.createCalls = %d, want 1", len(provider.createCalls))
	}
	call := provider.createCalls[0]

	if call.SnapshotID != "hiveloop-hermes-0-0-2-small-v1" {
		t.Errorf("snapshot = %q, want hermes prefix from cfg", call.SnapshotID)
	}

	apiKey, err := orch.encKey.DecryptString(sb.EncryptedBridgeAPIKey)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}

	cases := map[string]string{
		"AGENT_ID":              agent.ID.String(),
		"CONTROL_PLANE_URL":     "https://cp.hiveloop.test",
		"CONTROL_PLANE_API_KEY": apiKey,
		"HIVELOOP_GIT_USERNAME": sanitizeName(agent.Name),
		"HIVELOOP_GIT_EMAIL":    sanitizeName(agent.Name) + "@usehiveloop.com",
		"HIVELOOP_GIT_CREDENTIALS_URL": "https://cp.hiveloop.test/internal/git-credentials/" +
			agent.ID.String(),
		"BUGSINK_URL":         "https://cp.hiveloop.test/internal/bugsink-proxy/" + agent.ID.String(),
		"BUGSINK_TOKEN":       apiKey,
		"LINEAR_URL":          "https://cp.hiveloop.test/internal/linear-proxy/" + agent.ID.String(),
		"LINEAR_TOKEN":        apiKey,
		"HIVELOOP_SANDBOX_ID": sb.ID.String(),
		"HIVELOOP_ORG_ID":     org.ID.String(),
	}
	for key, want := range cases {
		if got := call.EnvVars[key]; got != want {
			t.Errorf("env[%s] = %q, want %q", key, got, want)
		}
	}

	// Sidecar runs on 7777, NOT bridge's 25434.
	if len(provider.endpointPorts) == 0 || provider.endpointPorts[0] != HermesSidecarPort {
		t.Errorf("GetEndpoint port = %v, want %d (HermesSidecarPort)",
			provider.endpointPorts, HermesSidecarPort)
	}

	// Bridge-only env must not leak in.
	if _, exists := call.EnvVars["BRIDGE_CONTROL_PLANE_API_KEY"]; exists {
		t.Errorf("bridge env BRIDGE_CONTROL_PLANE_API_KEY leaked into hermes sandbox")
	}
	if _, exists := call.EnvVars["BRIDGE_LISTEN_ADDR"]; exists {
		t.Errorf("bridge env BRIDGE_LISTEN_ADDR leaked into hermes sandbox")
	}
}

func TestCreateHermesSandbox_DisablesProviderLifecycle(t *testing.T) {
	orch, provider, db, _, _ := setupHermesOrchestrator(t, nil)
	org := createTestOrg(t, db)
	cred := createTestCred(t, db, org.ID)
	agent := createTestAgent(t, db, org.ID, cred.ID)

	if _, err := orch.CreateHermesSandbox(context.Background(), &agent); err != nil {
		t.Fatalf("CreateHermesSandbox: %v", err)
	}

	if len(provider.setAutoStopCalls) == 0 || provider.setAutoStopCalls[0].intervalMinutes != 0 {
		t.Errorf("SetAutoStop not called with intervalMinutes=0: %#v", provider.setAutoStopCalls)
	}
	if len(provider.setAutoArchiveCalls) == 0 || provider.setAutoArchiveCalls[0].intervalMinutes != 0 {
		t.Errorf("SetAutoArchive not called with intervalMinutes=0: %#v", provider.setAutoArchiveCalls)
	}
}

func TestCreateHermesSandbox_HealthFailureMarksSandboxError(t *testing.T) {
	// Sidecar always returns 503 — wait must time out via ctx cancellation
	// (without us shrinking the 90s package-level timeout).
	failing := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	orch, _, db, _, _ := setupHermesOrchestrator(t, failing)
	org := createTestOrg(t, db)
	cred := createTestCred(t, db, org.ID)
	agent := createTestAgent(t, db, org.ID, cred.ID)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := orch.CreateHermesSandbox(ctx, &agent)
	if err == nil {
		t.Fatalf("CreateHermesSandbox: expected error on 503 health, got nil")
	}

	// The sandbox row was created before the wait, so it must exist with status=error.
	var sandboxes []model.Sandbox
	if err := db.Where("agent_id = ?", agent.ID).Find(&sandboxes).Error; err != nil {
		t.Fatalf("query sandboxes: %v", err)
	}
	if len(sandboxes) != 1 {
		t.Fatalf("sandboxes for agent = %d, want 1 (with error status)", len(sandboxes))
	}
	got := sandboxes[0]
	if got.Status != "error" {
		t.Errorf("sandbox.status = %q, want error", got.Status)
	}
	if got.ErrorMessage == nil || *got.ErrorMessage == "" {
		t.Errorf("sandbox.error_message should be populated, got %v", got.ErrorMessage)
	}
}

func TestCreateHermesSandbox_RejectsAgentWithoutOrgID(t *testing.T) {
	orch, _, _, _, _ := setupHermesOrchestrator(t, nil)
	agent := model.Agent{Name: "no-org", SystemPrompt: "x", Model: "y"} // OrgID = nil

	_, err := orch.CreateHermesSandbox(context.Background(), &agent)
	if err == nil {
		t.Fatalf("expected error for nil OrgID, got nil")
	}
}
