package sandbox

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/config"
	"github.com/ziraloop/ziraloop/internal/crypto"
	"github.com/ziraloop/ziraloop/internal/model"
	"github.com/ziraloop/ziraloop/internal/turso"
)

const testDBURL = "postgres://ziraloop:localdev@localhost:5433/ziraloop_test?sslmode=disable"

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = testDBURL
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	sqlDB, _ := db.DB()
	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("ping: %v", err)
	}
	if err := model.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	return db
}

func testEncKey(t *testing.T) *crypto.SymmetricKey {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 42)
	}
	sk, err := crypto.NewSymmetricKey(base64.StdEncoding.EncodeToString(key))
	if err != nil {
		t.Fatal(err)
	}
	return sk
}

func mockTursoServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path != "" && r.URL.Path[len(r.URL.Path)-9:] == "databases":
			var body struct{ Name, Group string }
			json.NewDecoder(r.Body).Decode(&body)
			w.WriteHeader(200)
			json.NewEncoder(w).Encode(map[string]any{
				"database": map[string]any{"Name": body.Name, "DbId": "db-" + body.Name, "Hostname": body.Name + ".turso.io"},
			})
		case r.Method == http.MethodPost:
			w.WriteHeader(200)
			json.NewEncoder(w).Encode(map[string]string{"jwt": "mock-turso-jwt"})
		case r.Method == http.MethodDelete:
			w.WriteHeader(200)
		default:
			w.WriteHeader(404)
		}
	}))
}

func setupOrchestrator(t *testing.T) (*Orchestrator, *mockProvider, *gorm.DB) {
	t.Helper()
	db := setupTestDB(t)

	bridgeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(200)
			w.Write([]byte(`{"status":"ok"}`))
			return
		}
		w.WriteHeader(404)
	}))
	t.Cleanup(bridgeSrv.Close)

	provider := newMockProvider()
	provider.endpointOverride = bridgeSrv.URL
	tursoSrv := mockTursoServer(t)
	t.Cleanup(tursoSrv.Close)

	tursoClient := turso.NewClient("token", "org")
	tursoClient.SetBaseURL(tursoSrv.URL)
	tursoProvisioner := turso.NewProvisioner(tursoClient, "default", db)

	cfg := &config.Config{
		BridgeBaseImagePrefix:           "ziraloop-bridge-0-10-0",
		BridgeHost:                      "test.ziraloop.com",
		SharedSandboxIdleTimeoutMins:    30,
		DedicatedSandboxGracePeriodMins: 5,
		PoolSandboxResourceThreshold:    80.0,
		PoolSandboxIdleTimeoutMins:      30,
	}

	orch := NewOrchestrator(db, provider, tursoProvisioner, testEncKey(t), cfg)
	return orch, provider, db
}

func createTestOrg(t *testing.T, db *gorm.DB) model.Org {
	t.Helper()
	suffix := uuid.New().String()[:8]
	org := model.Org{Name: "orch-test-" + suffix}
	db.Create(&org)
	t.Cleanup(func() { db.Where("id = ?", org.ID).Delete(&model.Org{}) })
	return org
}

	t.Helper()
	suffix := uuid.New().String()[:8]
	db.Create(&identity)
	return identity
}

func createTestAgent(t *testing.T, db *gorm.DB, orgID, identityID, credID uuid.UUID, sandboxType string) model.Agent {
	t.Helper()
	suffix := uuid.New().String()[:8]
	agent := model.Agent{
		OrgID: &orgID, IdentityID: &identityID, Name: "agent-" + suffix,
		CredentialID: &credID, SandboxType: sandboxType,
		SystemPrompt: "test", Model: "gpt-4o",
	}
	db.Create(&agent)
	t.Cleanup(func() { db.Where("id = ?", agent.ID).Delete(&model.Agent{}) })
	return agent
}

func createTestCred(t *testing.T, db *gorm.DB, orgID uuid.UUID) model.Credential {
	t.Helper()
	cred := model.Credential{
		OrgID: orgID, BaseURL: "https://api.openai.com", AuthScheme: "bearer",
		ProviderID: "openai", EncryptedKey: []byte("enc"), WrappedDEK: []byte("dek"),
	}
	db.Create(&cred)
	t.Cleanup(func() { db.Where("id = ?", cred.ID).Delete(&model.Credential{}) })
	return cred
}

// seedSharedSandbox inserts a shared sandbox directly into the DB with specified resource usage.
// Returns the sandbox for cleanup/assertion. Does NOT go through the provider.
func seedSharedSandbox(t *testing.T, db *gorm.DB, memUsed, memLimit int64) model.Sandbox {
	t.Helper()
	encKey := testEncKey(t)
	apiKey, _ := generateRandomHex(32)
	encrypted, _ := encKey.EncryptString(apiKey)

	sb := model.Sandbox{
		SandboxType:           "shared",
		ExternalID:            "seed-" + uuid.New().String()[:8],
		BridgeURL:             "https://mock:25434",
		EncryptedBridgeAPIKey: encrypted,
		Status:                "running",
		MemoryUsedBytes:       memUsed,
		MemoryLimitBytes:      memLimit,
	}
	if err := db.Create(&sb).Error; err != nil {
		t.Fatalf("seed sandbox: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", sb.ID).Delete(&model.Sandbox{}) })
	return sb
}

// --- Selection Logic Tests (DB-seeded, no mock provider) ---

func TestSelection_PicksLowestMemoryUsage(t *testing.T) {
	orch, _, db := setupOrchestrator(t)
	org := createTestOrg(t, db)
	cred := createTestCred(t, db, org.ID)
	agent := createTestAgent(t, db, org.ID, identity.ID, cred.ID, "shared")

	gb := int64(1024 * 1024 * 1024)

	// Seed 3 sandboxes with different memory usage
	sbHigh := seedSharedSandbox(t, db, 70*gb/100, gb)   // 70%
	sbLow := seedSharedSandbox(t, db, 20*gb/100, gb)    // 20% — should be picked
	sbMid := seedSharedSandbox(t, db, 50*gb/100, gb)    // 50%
	_ = sbHigh
	_ = sbMid

	ctx := context.Background()
	picked, err := orch.AssignPoolSandbox(ctx, &agent)
	if err != nil {
		t.Fatalf("AssignPoolSandbox: %v", err)
	}

	if picked.ID != sbLow.ID {
		t.Errorf("should pick lowest usage sandbox (20%%): got %s, want %s", picked.ID, sbLow.ID)
	}

	// Verify agent is assigned in DB
	var reloaded model.Agent
	db.Where("id = ?", agent.ID).First(&reloaded)
	if reloaded.SandboxID == nil || *reloaded.SandboxID != sbLow.ID {
		t.Error("agent.SandboxID should point to the lowest-usage sandbox")
	}
}

func TestSelection_SkipsOverThreshold(t *testing.T) {
	orch, _, db := setupOrchestrator(t)
	org := createTestOrg(t, db)
	cred := createTestCred(t, db, org.ID)
	agent := createTestAgent(t, db, org.ID, identity.ID, cred.ID, "shared")

	gb := int64(1024 * 1024 * 1024)

	// Threshold is 80%. Seed one at 90% and one at 50%.
	sbOver := seedSharedSandbox(t, db, 90*gb/100, gb)  // 90% — over threshold
	sbUnder := seedSharedSandbox(t, db, 50*gb/100, gb) // 50% — under threshold
	_ = sbOver

	ctx := context.Background()
	picked, err := orch.AssignPoolSandbox(ctx, &agent)
	if err != nil {
		t.Fatalf("AssignPoolSandbox: %v", err)
	}

	if picked.ID != sbUnder.ID {
		t.Errorf("should skip over-threshold sandbox: got %s, want %s", picked.ID, sbUnder.ID)
	}
}

func TestSelection_AllOverThreshold_CreatesNew(t *testing.T) {
	orch, provider, db := setupOrchestrator(t)
	org := createTestOrg(t, db)
	cred := createTestCred(t, db, org.ID)
	agent := createTestAgent(t, db, org.ID, identity.ID, cred.ID, "shared")

	gb := int64(1024 * 1024 * 1024)

	// Both over 80% threshold
	seedSharedSandbox(t, db, 85*gb/100, gb) // 85%
	seedSharedSandbox(t, db, 95*gb/100, gb) // 95%

	ctx := context.Background()
	picked, err := orch.AssignPoolSandbox(ctx, &agent)
	if err != nil {
		t.Fatalf("AssignPoolSandbox: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", picked.ID).Delete(&model.Sandbox{}) })

	// Should have created a new sandbox via provider
	if provider.count() != 1 {
		t.Errorf("provider should have created 1 new sandbox, got %d", provider.count())
	}
	if picked.SandboxType != "shared" {
		t.Errorf("new sandbox type: got %q, want shared", picked.SandboxType)
	}
}

func TestSelection_UnmeasuredSandboxesPreferred(t *testing.T) {
	orch, _, db := setupOrchestrator(t)
	org := createTestOrg(t, db)
	cred := createTestCred(t, db, org.ID)
	agent := createTestAgent(t, db, org.ID, identity.ID, cred.ID, "shared")

	gb := int64(1024 * 1024 * 1024)

	// One measured at 50%, one unmeasured (memory_limit_bytes=0)
	sbMeasured := seedSharedSandbox(t, db, 50*gb/100, gb)
	sbUnmeasured := seedSharedSandbox(t, db, 0, 0) // no resource data yet
	_ = sbMeasured

	ctx := context.Background()
	picked, err := orch.AssignPoolSandbox(ctx, &agent)
	if err != nil {
		t.Fatalf("AssignPoolSandbox: %v", err)
	}

	// Unmeasured sandboxes sort as 0% usage, so they're picked first
	if picked.ID != sbUnmeasured.ID {
		t.Errorf("should prefer unmeasured sandbox: got %s, want %s", picked.ID, sbUnmeasured.ID)
	}
}

func TestSelection_SkipsNonRunningSandboxes(t *testing.T) {
	orch, _, db := setupOrchestrator(t)
	org := createTestOrg(t, db)
	cred := createTestCred(t, db, org.ID)
	agent := createTestAgent(t, db, org.ID, identity.ID, cred.ID, "shared")

	// Seed one stopped, one running
	sbStopped := seedSharedSandbox(t, db, 0, 0)
	db.Model(&sbStopped).Update("status", "stopped")

	sbRunning := seedSharedSandbox(t, db, 0, 0)

	ctx := context.Background()
	picked, err := orch.AssignPoolSandbox(ctx, &agent)
	if err != nil {
		t.Fatalf("AssignPoolSandbox: %v", err)
	}

	if picked.ID != sbRunning.ID {
		t.Errorf("should skip stopped sandbox: got %s, want %s", picked.ID, sbRunning.ID)
	}
}

func TestSelection_SkipsDedicatedSandboxes(t *testing.T) {
	orch, _, db := setupOrchestrator(t)
	org := createTestOrg(t, db)
	cred := createTestCred(t, db, org.ID)
	agent := createTestAgent(t, db, org.ID, identity.ID, cred.ID, "shared")

	// Seed a dedicated sandbox (should be ignored) and a shared one
	encKey := testEncKey(t)
	apiKey, _ := generateRandomHex(32)
	encrypted, _ := encKey.EncryptString(apiKey)
	dedicated := model.Sandbox{
		OrgID: &org.ID, IdentityID: &identity.ID, SandboxType: "dedicated",
		ExternalID: "ded-" + uuid.New().String()[:8], BridgeURL: "https://mock:25434",
		EncryptedBridgeAPIKey: encrypted, Status: "running",
	}
	db.Create(&dedicated)
	t.Cleanup(func() { db.Where("id = ?", dedicated.ID).Delete(&model.Sandbox{}) })

	sbShared := seedSharedSandbox(t, db, 0, 0)

	ctx := context.Background()
	picked, err := orch.AssignPoolSandbox(ctx, &agent)
	if err != nil {
		t.Fatalf("AssignPoolSandbox: %v", err)
	}

	if picked.ID != sbShared.ID {
		t.Errorf("should only pick shared sandboxes: got %s, want %s", picked.ID, sbShared.ID)
	}
}

func TestSelection_CrossOrg(t *testing.T) {
	orch, _, db := setupOrchestrator(t)

	// Two different orgs, each with an agent
	org1 := createTestOrg(t, db)
	cred1 := createTestCred(t, db, org1.ID)
	agent1 := createTestAgent(t, db, org1.ID, identity1.ID, cred1.ID, "shared")

	org2 := createTestOrg(t, db)
	cred2 := createTestCred(t, db, org2.ID)
	agent2 := createTestAgent(t, db, org2.ID, identity2.ID, cred2.ID, "shared")

	// One shared sandbox in the pool (no org ownership)
	sbPool := seedSharedSandbox(t, db, 0, 0)

	ctx := context.Background()

	// Agent from org1 gets the pool sandbox
	picked1, err := orch.AssignPoolSandbox(ctx, &agent1)
	if err != nil {
		t.Fatalf("org1 assign: %v", err)
	}
	if picked1.ID != sbPool.ID {
		t.Errorf("org1 agent should get pool sandbox: got %s, want %s", picked1.ID, sbPool.ID)
	}

	// Agent from org2 also gets the same pool sandbox
	picked2, err := orch.AssignPoolSandbox(ctx, &agent2)
	if err != nil {
		t.Fatalf("org2 assign: %v", err)
	}
	if picked2.ID != sbPool.ID {
		t.Errorf("org2 agent should get same pool sandbox: got %s, want %s", picked2.ID, sbPool.ID)
	}
}

// --- Assignment Lifecycle Tests ---

func TestAssign_AgentWithExistingSandbox_ReturnsIt(t *testing.T) {
	orch, _, db := setupOrchestrator(t)
	org := createTestOrg(t, db)
	cred := createTestCred(t, db, org.ID)
	agent := createTestAgent(t, db, org.ID, identity.ID, cred.ID, "shared")

	sbExisting := seedSharedSandbox(t, db, 0, 0)
	sbOther := seedSharedSandbox(t, db, 0, 0)
	_ = sbOther

	// Pre-assign agent to sbExisting
	db.Model(&agent).Update("sandbox_id", sbExisting.ID)
	agent.SandboxID = &sbExisting.ID

	ctx := context.Background()
	picked, err := orch.AssignPoolSandbox(ctx, &agent)
	if err != nil {
		t.Fatalf("AssignPoolSandbox: %v", err)
	}

	// Should return the already-assigned sandbox, not pick a new one
	if picked.ID != sbExisting.ID {
		t.Errorf("should return existing assignment: got %s, want %s", picked.ID, sbExisting.ID)
	}
}

func TestRelease_ClearsAgentSandboxID(t *testing.T) {
	orch, _, db := setupOrchestrator(t)
	org := createTestOrg(t, db)
	cred := createTestCred(t, db, org.ID)
	agent := createTestAgent(t, db, org.ID, identity.ID, cred.ID, "shared")

	sb := seedSharedSandbox(t, db, 0, 0)
	db.Model(&agent).Update("sandbox_id", sb.ID)
	agent.SandboxID = &sb.ID

	ctx := context.Background()
	if err := orch.ReleasePoolSandbox(ctx, &agent); err != nil {
		t.Fatalf("release: %v", err)
	}

	var reloaded model.Agent
	db.Where("id = ?", agent.ID).First(&reloaded)
	if reloaded.SandboxID != nil {
		t.Error("agent.SandboxID should be nil after release")
	}
}

func TestRelease_NilSandboxID_Noop(t *testing.T) {
	orch, _, db := setupOrchestrator(t)
	org := createTestOrg(t, db)
	cred := createTestCred(t, db, org.ID)
	agent := createTestAgent(t, db, org.ID, identity.ID, cred.ID, "shared")

	ctx := context.Background()
	if err := orch.ReleasePoolSandbox(ctx, &agent); err != nil {
		t.Fatalf("release with nil SandboxID should be noop: %v", err)
	}
}

// --- On-Demand Creation Tests ---

func TestAssign_EmptyPool_CreatesAndPersistsSandbox(t *testing.T) {
	orch, provider, db := setupOrchestrator(t)
	org := createTestOrg(t, db)
	cred := createTestCred(t, db, org.ID)
	agent1 := createTestAgent(t, db, org.ID, identity.ID, cred.ID, "shared")
	agent2 := createTestAgent(t, db, org.ID, identity.ID, cred.ID, "shared")

	// No seeded sandboxes — pool is empty

	ctx := context.Background()
	sb, err := orch.AssignPoolSandbox(ctx, &agent1)
	if err != nil {
		t.Fatalf("AssignPoolSandbox: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", sb.ID).Delete(&model.Sandbox{}) })

	// Verify provider was called
	if provider.count() != 1 {
		t.Fatalf("provider should have created 1 sandbox, got %d", provider.count())
	}

	// Verify the sandbox was persisted to DB with correct fields
	var persisted model.Sandbox
	if err := db.Where("id = ?", sb.ID).First(&persisted).Error; err != nil {
		t.Fatalf("sandbox should be persisted in DB: %v", err)
	}
	if persisted.SandboxType != "shared" {
		t.Errorf("persisted type: got %q, want shared", persisted.SandboxType)
	}
	if persisted.OrgID != nil {
		t.Error("persisted sandbox should have nil OrgID (pool sandbox)")
	}
	if persisted.IdentityID != nil {
		t.Error("persisted sandbox should have nil IdentityID (pool sandbox)")
	}
	if persisted.Status != "running" {
		t.Errorf("persisted status: got %q, want running", persisted.Status)
	}
	if persisted.ExternalID == "" {
		t.Error("persisted sandbox should have an external_id from the provider")
	}
	if persisted.BridgeURL == "" {
		t.Error("persisted sandbox should have a bridge_url")
	}
	if persisted.BridgeURLExpiresAt == nil {
		t.Error("persisted sandbox should have bridge_url_expires_at set")
	}
	if len(persisted.EncryptedBridgeAPIKey) == 0 {
		t.Error("persisted sandbox should have encrypted bridge API key")
	}
	if persisted.LastActiveAt == nil {
		t.Error("persisted sandbox should have last_active_at set")
	}

	// Verify agent1 is assigned
	var a1 model.Agent
	db.Where("id = ?", agent1.ID).First(&a1)
	if a1.SandboxID == nil || *a1.SandboxID != sb.ID {
		t.Error("agent1 should be assigned to the new sandbox")
	}

	// Verify a second agent assignment reuses the auto-provisioned sandbox
	// (proves the sandbox is visible to the selection query)
	sb2, err := orch.AssignPoolSandbox(ctx, &agent2)
	if err != nil {
		t.Fatalf("second AssignPoolSandbox: %v", err)
	}
	if sb2.ID != sb.ID {
		t.Errorf("second agent should reuse the auto-provisioned sandbox: got %s, want %s", sb2.ID, sb.ID)
	}
	if provider.count() != 1 {
		t.Errorf("provider should still have 1 sandbox (reused), got %d", provider.count())
	}
}

// --- Health Checker Tests ---

func TestHealthCheck_SharedSandboxWithAgents_NotStopped(t *testing.T) {
	orch, _, db := setupOrchestrator(t)
	org := createTestOrg(t, db)
	cred := createTestCred(t, db, org.ID)
	agent := createTestAgent(t, db, org.ID, identity.ID, cred.ID, "shared")

	sb := seedSharedSandbox(t, db, 0, 0)
	// Assign the agent to the sandbox
	db.Model(&agent).Update("sandbox_id", sb.ID)

	// Set last_active_at to long ago
	old := time.Now().Add(-2 * time.Hour)
	db.Model(&sb).Update("last_active_at", old)
	sb.LastActiveAt = &old

	ctx := context.Background()
	orch.checkSandboxHealth(ctx, &sb)

	var reloaded model.Sandbox
	db.Where("id = ?", sb.ID).First(&reloaded)
	if reloaded.Status != "running" {
		t.Errorf("shared sandbox with agents should stay running, got %q", reloaded.Status)
	}
}

func TestHealthCheck_SharedSandboxEmpty_Stopped(t *testing.T) {
	orch, provider, db := setupOrchestrator(t)
	orch.cfg.PoolSandboxIdleTimeoutMins = 1

	sb := seedSharedSandbox(t, db, 0, 0)
	provider.registerSandbox(sb.ExternalID, StatusRunning)
	// No agents assigned

	old := time.Now().Add(-2 * time.Hour)
	db.Model(&sb).Update("last_active_at", old)
	sb.LastActiveAt = &old

	ctx := context.Background()
	orch.checkSandboxHealth(ctx, &sb)

	var reloaded model.Sandbox
	db.Where("id = ?", sb.ID).First(&reloaded)
	if reloaded.Status != "stopped" {
		t.Errorf("empty shared sandbox should be stopped, got %q", reloaded.Status)
	}
}

func TestHealthCheck_SharedSandboxError_UnassignsAgents(t *testing.T) {
	orch, provider, db := setupOrchestrator(t)
	org := createTestOrg(t, db)
	cred := createTestCred(t, db, org.ID)
	agent1 := createTestAgent(t, db, org.ID, identity.ID, cred.ID, "shared")
	agent2 := createTestAgent(t, db, org.ID, identity.ID, cred.ID, "shared")

	sb := seedSharedSandbox(t, db, 0, 0)
	provider.registerSandbox(sb.ExternalID, StatusError)
	db.Model(&agent1).Update("sandbox_id", sb.ID)
	db.Model(&agent2).Update("sandbox_id", sb.ID)

	// Simulate error state
	db.Model(&sb).Update("status", "error")
	sb.Status = "error"

	ctx := context.Background()
	orch.checkSandboxHealth(ctx, &sb)

	// Both agents should be unassigned
	var a1, a2 model.Agent
	db.Where("id = ?", agent1.ID).First(&a1)
	db.Where("id = ?", agent2.ID).First(&a2)
	if a1.SandboxID != nil {
		t.Error("agent1 should be unassigned after sandbox error")
	}
	if a2.SandboxID != nil {
		t.Error("agent2 should be unassigned after sandbox error")
	}
}

// --- Dedicated Sandbox Tests ---

func TestCreateDedicatedSandbox(t *testing.T) {
	orch, provider, db := setupOrchestrator(t)
	org := createTestOrg(t, db)
	cred := createTestCred(t, db, org.ID)
	agent := createTestAgent(t, db, org.ID, identity.ID, cred.ID, "dedicated")

	ctx := context.Background()
	sb, err := orch.CreateDedicatedSandbox(ctx, &agent)
	if err != nil {
		t.Fatalf("CreateDedicatedSandbox: %v", err)
	}
	t.Cleanup(func() {
		db.Where("id = ?", sb.ID).Delete(&model.Sandbox{})
		db.Where("org_id = ?", org.ID).Delete(&model.WorkspaceStorage{})
	})

	if sb.SandboxType != "dedicated" {
		t.Errorf("type: got %q", sb.SandboxType)
	}
	if sb.AgentID == nil || *sb.AgentID != agent.ID {
		t.Error("agent_id should be set")
	}
	if sb.IdentityID == nil || *sb.IdentityID != identity.ID {
		t.Error("identity_id should match agent's identity")
	}
	if sb.OrgID == nil || *sb.OrgID != org.ID {
		t.Error("org_id should be set for dedicated sandboxes")
	}
	if sb.Status != "running" {
		t.Errorf("status: got %q", sb.Status)
	}
	if provider.count() != 1 {
		t.Errorf("expected 1 sandbox, got %d", provider.count())
	}
}

// --- System Sandbox Tests ---

// seedSystemAgent inserts an is_system=true agent row directly. Used by the
// EnsureSystemSandbox tests to verify the bulk-bind step.
func seedSystemAgent(t *testing.T, db *gorm.DB, name, providerGroup string) model.Agent {
	t.Helper()
	agent := model.Agent{
		Name:          name,
		IsSystem:      true,
		ProviderGroup: providerGroup,
		SandboxType:   "shared",
		SystemPrompt:  "test",
		Model:         "test-model",
		Status:        "active",
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("seed system agent: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", agent.ID).Delete(&model.Agent{}) })
	return agent
}

// seedSystemSandboxRow inserts a system sandbox row directly without going
// through the provider. Useful for the "returns existing" / "wakes stopped"
// branches of EnsureSystemSandbox.
func seedSystemSandboxRow(t *testing.T, db *gorm.DB, externalID string, status string) model.Sandbox {
	t.Helper()
	encKey := testEncKey(t)
	apiKey, _ := generateRandomHex(32)
	encrypted, _ := encKey.EncryptString(apiKey)

	sb := model.Sandbox{
		SandboxType:           "system",
		ExternalID:            externalID,
		BridgeURL:             "https://mock:25434",
		EncryptedBridgeAPIKey: encrypted,
		Status:                status,
	}
	if err := db.Create(&sb).Error; err != nil {
		t.Fatalf("seed system sandbox: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", sb.ID).Delete(&model.Sandbox{}) })
	return sb
}

func TestEnsureSystemSandbox_CreatesWhenMissing(t *testing.T) {
	orch, provider, db := setupOrchestrator(t)
	// Wipe any pre-existing system sandbox row from prior runs.
	db.Where("sandbox_type = ?", "system").Delete(&model.Sandbox{})

	ctx := context.Background()
	sb, err := orch.EnsureSystemSandbox(ctx)
	if err != nil {
		t.Fatalf("EnsureSystemSandbox: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", sb.ID).Delete(&model.Sandbox{}) })

	if sb.SandboxType != "system" {
		t.Errorf("sandbox type: got %q, want system", sb.SandboxType)
	}
	if sb.OrgID != nil {
		t.Error("system sandbox should have nil OrgID")
	}
	if sb.IdentityID != nil {
		t.Error("system sandbox should have nil IdentityID")
	}
	if sb.Status != "running" {
		t.Errorf("status: got %q, want running", sb.Status)
	}
	if sb.ExternalID == "" {
		t.Error("external_id should be populated from provider")
	}
	if provider.count() != 1 {
		t.Fatalf("provider should have 1 sandbox, got %d", provider.count())
	}

	// Persisted in DB.
	var persisted model.Sandbox
	if err := db.Where("sandbox_type = ?", "system").First(&persisted).Error; err != nil {
		t.Fatalf("system sandbox should be persisted: %v", err)
	}
	if persisted.ID != sb.ID {
		t.Errorf("persisted ID mismatch: got %s, want %s", persisted.ID, sb.ID)
	}
}

func TestEnsureSystemSandbox_DisablesAutoStop(t *testing.T) {
	orch, provider, db := setupOrchestrator(t)
	db.Where("sandbox_type = ?", "system").Delete(&model.Sandbox{})

	ctx := context.Background()
	sb, err := orch.EnsureSystemSandbox(ctx)
	if err != nil {
		t.Fatalf("EnsureSystemSandbox: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", sb.ID).Delete(&model.Sandbox{}) })

	provider.mu.Lock()
	calls := append([]setAutoStopCall(nil), provider.setAutoStopCalls...)
	provider.mu.Unlock()

	if len(calls) != 1 {
		t.Fatalf("SetAutoStop calls: got %d, want 1", len(calls))
	}
	if calls[0].externalID != sb.ExternalID {
		t.Errorf("SetAutoStop externalID: got %q, want %q", calls[0].externalID, sb.ExternalID)
	}
	if calls[0].intervalMinutes != 0 {
		t.Errorf("SetAutoStop intervalMinutes: got %d, want 0 (disabled)", calls[0].intervalMinutes)
	}
}

func TestEnsureSystemSandbox_ReturnsExistingRunning(t *testing.T) {
	orch, provider, db := setupOrchestrator(t)
	db.Where("sandbox_type = ?", "system").Delete(&model.Sandbox{})

	// Pre-seed a system row and register it in the mock provider so
	// verifySandboxExists succeeds.
	existing := seedSystemSandboxRow(t, db, "mock-existing-system", "running")
	provider.registerSandbox(existing.ExternalID, StatusRunning)

	ctx := context.Background()
	sb, err := orch.EnsureSystemSandbox(ctx)
	if err != nil {
		t.Fatalf("EnsureSystemSandbox: %v", err)
	}

	if sb.ID != existing.ID {
		t.Errorf("returned ID: got %s, want %s (existing)", sb.ID, existing.ID)
	}
	if provider.count() != 1 {
		t.Errorf("provider should still have 1 sandbox (no new create), got %d", provider.count())
	}
}

func TestEnsureSystemSandbox_RecreatesIfProviderLost(t *testing.T) {
	orch, provider, db := setupOrchestrator(t)
	db.Where("sandbox_type = ?", "system").Delete(&model.Sandbox{})

	// Pre-seed a system row but DON'T register in mock provider, so
	// verifySandboxExists fails.
	stale := seedSystemSandboxRow(t, db, "mock-stale-system", "running")

	ctx := context.Background()
	sb, err := orch.EnsureSystemSandbox(ctx)
	if err != nil {
		t.Fatalf("EnsureSystemSandbox: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", sb.ID).Delete(&model.Sandbox{}) })

	if sb.ID == stale.ID {
		t.Errorf("expected new sandbox ID after recreate, got the stale one (%s)", sb.ID)
	}
	// Stale row should be deleted.
	var count int64
	db.Model(&model.Sandbox{}).Where("id = ?", stale.ID).Count(&count)
	if count != 0 {
		t.Errorf("stale sandbox row should be deleted, found %d rows", count)
	}
	if provider.count() != 1 {
		t.Errorf("provider should have 1 sandbox (the new one), got %d", provider.count())
	}
}

func TestEnsureSystemSandbox_WakesStopped(t *testing.T) {
	orch, provider, db := setupOrchestrator(t)
	db.Where("sandbox_type = ?", "system").Delete(&model.Sandbox{})

	// Pre-seed a stopped system row and register in mock as stopped.
	existing := seedSystemSandboxRow(t, db, "mock-stopped-system", "stopped")
	provider.registerSandbox(existing.ExternalID, StatusStopped)

	ctx := context.Background()
	sb, err := orch.EnsureSystemSandbox(ctx)
	if err != nil {
		t.Fatalf("EnsureSystemSandbox: %v", err)
	}

	if sb.ID != existing.ID {
		t.Errorf("returned ID: got %s, want %s", sb.ID, existing.ID)
	}
	if sb.Status != "running" {
		t.Errorf("status after wake: got %q, want running", sb.Status)
	}
	if provider.getStatus(existing.ExternalID) != StatusRunning {
		t.Error("mock provider should report sandbox as running after wake")
	}
}

func TestEnsureSystemSandbox_BindsAllSystemAgents(t *testing.T) {
	orch, _, db := setupOrchestrator(t)
	db.Where("sandbox_type = ?", "system").Delete(&model.Sandbox{})

	// Seed three system agent rows.
	a1 := seedSystemAgent(t, db, "test-system-agent-1-"+uuid.New().String()[:8], "anthropic")
	a2 := seedSystemAgent(t, db, "test-system-agent-2-"+uuid.New().String()[:8], "openai")
	a3 := seedSystemAgent(t, db, "test-system-agent-3-"+uuid.New().String()[:8], "gemini")

	ctx := context.Background()
	sb, err := orch.EnsureSystemSandbox(ctx)
	if err != nil {
		t.Fatalf("EnsureSystemSandbox: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", sb.ID).Delete(&model.Sandbox{}) })

	for _, want := range []model.Agent{a1, a2, a3} {
		var reloaded model.Agent
		if err := db.Where("id = ?", want.ID).First(&reloaded).Error; err != nil {
			t.Fatalf("reload agent %s: %v", want.Name, err)
		}
		if reloaded.SandboxID == nil {
			t.Errorf("agent %s: sandbox_id should be set", want.Name)
			continue
		}
		if *reloaded.SandboxID != sb.ID {
			t.Errorf("agent %s: sandbox_id %s, want %s", want.Name, *reloaded.SandboxID, sb.ID)
		}
	}
}

func TestHealthCheck_SystemSandboxNeverStopped(t *testing.T) {
	orch, provider, db := setupOrchestrator(t)
	orch.cfg.PoolSandboxIdleTimeoutMins = 1 // would stop a shared sandbox

	sb := seedSystemSandboxRow(t, db, "mock-system-idle", "running")
	provider.registerSandbox(sb.ExternalID, StatusRunning)

	// Set last_active_at well past the threshold so a shared sandbox
	// would be auto-stopped here.
	old := time.Now().Add(-2 * time.Hour)
	db.Model(&sb).Update("last_active_at", old)
	sb.LastActiveAt = &old

	ctx := context.Background()
	orch.checkSandboxHealth(ctx, &sb)

	var reloaded model.Sandbox
	db.Where("id = ?", sb.ID).First(&reloaded)
	if reloaded.Status != "running" {
		t.Errorf("system sandbox should never auto-stop, got status %q", reloaded.Status)
	}
	if provider.getStatus(sb.ExternalID) != StatusRunning {
		t.Error("provider should still report system sandbox as running")
	}
}

func TestHealthCheck_SystemSandboxStoppedWakes(t *testing.T) {
	orch, provider, db := setupOrchestrator(t)

	sb := seedSystemSandboxRow(t, db, "mock-system-stopped", "stopped")
	provider.registerSandbox(sb.ExternalID, StatusStopped)

	ctx := context.Background()
	orch.checkSandboxHealth(ctx, &sb)

	if provider.getStatus(sb.ExternalID) != StatusRunning {
		t.Errorf("system sandbox should be woken by health check, provider status %v",
			provider.getStatus(sb.ExternalID))
	}
}
