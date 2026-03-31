package sandbox

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/llmvault/llmvault/internal/config"
	"github.com/llmvault/llmvault/internal/crypto"
	"github.com/llmvault/llmvault/internal/model"
	"github.com/llmvault/llmvault/internal/turso"
)

const testDBURL = "postgres://llmvault:localdev@localhost:5433/llmvault_test?sslmode=disable"

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
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/databases"):
			var body struct{ Name, Group string }
			json.NewDecoder(r.Body).Decode(&body)
			w.WriteHeader(200)
			json.NewEncoder(w).Encode(map[string]any{
				"database": map[string]any{"Name": body.Name, "DbId": "db-" + body.Name, "Hostname": body.Name + ".turso.io"},
			})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/auth/tokens"):
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

	// Mock Bridge health endpoint — all mock sandbox URLs point here
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
	provider.endpointOverride = bridgeSrv.URL // all sandboxes return this URL
	tursoSrv := mockTursoServer(t)
	t.Cleanup(tursoSrv.Close)

	tursoClient := turso.NewClient("token", "org")
	tursoClient.SetBaseURL(tursoSrv.URL) // we'll need to add this method
	tursoProvisioner := turso.NewProvisioner(tursoClient, "default", db)

	cfg := &config.Config{
		BridgeBaseImagePrefix: "llmvault-bridge-0-10-0",
		BridgeHost:            "test.llmvault.dev",
		SharedSandboxIdleTimeoutMins:    30,
		DedicatedSandboxGracePeriodMins: 5,
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

func createTestIdentity(t *testing.T, db *gorm.DB, orgID uuid.UUID) model.Identity {
	t.Helper()
	suffix := uuid.New().String()[:8]
	identity := model.Identity{OrgID: orgID, ExternalID: "user-" + suffix}
	db.Create(&identity)
	t.Cleanup(func() { db.Where("id = ?", identity.ID).Delete(&model.Identity{}) })
	return identity
}

func createTestAgent(t *testing.T, db *gorm.DB, orgID, identityID, credID uuid.UUID, sandboxType string) model.Agent {
	t.Helper()
	suffix := uuid.New().String()[:8]
	agent := model.Agent{
		OrgID: orgID, IdentityID: identityID, Name: "agent-" + suffix,
		CredentialID: credID, SandboxType: sandboxType,
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

func TestEnsureSharedSandbox_CreateNew(t *testing.T) {
	orch, provider, db := setupOrchestrator(t)
	org := createTestOrg(t, db)
	identity := createTestIdentity(t, db, org.ID)

	ctx := context.Background()
	sb, err := orch.EnsureSharedSandbox(ctx, &org, &identity)
	if err != nil {
		t.Fatalf("EnsureSharedSandbox: %v", err)
	}
	t.Cleanup(func() {
		db.Where("id = ?", sb.ID).Delete(&model.Sandbox{})
		db.Where("org_id = ?", org.ID).Delete(&model.WorkspaceStorage{})
	})

	if sb.Status != "running" {
		t.Errorf("status: got %q, want running", sb.Status)
	}
	if sb.SandboxType != "shared" {
		t.Errorf("type: got %q", sb.SandboxType)
	}
	if sb.IdentityID != identity.ID {
		t.Errorf("identity_id mismatch")
	}
	if sb.ExternalID == "" {
		t.Error("external_id should be set")
	}
	if sb.BridgeURL == "" {
		t.Error("bridge_url should be set")
	}
	if sb.BridgeURLExpiresAt == nil {
		t.Error("bridge_url_expires_at should be set")
	}
	if len(sb.EncryptedBridgeAPIKey) == 0 {
		t.Error("encrypted_bridge_api_key should be set")
	}
	if provider.count() != 1 {
		t.Errorf("provider should have 1 sandbox, got %d", provider.count())
	}

	// Verify WorkspaceStorage was created
	var ws model.WorkspaceStorage
	if err := db.Where("org_id = ?", org.ID).First(&ws).Error; err != nil {
		t.Errorf("workspace storage should have been created: %v", err)
	}
}

func TestEnsureSharedSandbox_ReturnExisting(t *testing.T) {
	orch, provider, db := setupOrchestrator(t)
	org := createTestOrg(t, db)
	identity := createTestIdentity(t, db, org.ID)

	ctx := context.Background()
	sb1, err := orch.EnsureSharedSandbox(ctx, &org, &identity)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	t.Cleanup(func() {
		db.Where("id = ?", sb1.ID).Delete(&model.Sandbox{})
		db.Where("org_id = ?", org.ID).Delete(&model.WorkspaceStorage{})
	})

	sb2, err := orch.EnsureSharedSandbox(ctx, &org, &identity)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}

	if sb1.ID != sb2.ID {
		t.Errorf("should return same sandbox: got %s and %s", sb1.ID, sb2.ID)
	}
	if provider.count() != 1 {
		t.Errorf("provider should still have 1 sandbox, got %d", provider.count())
	}
}

func TestEnsureSharedSandbox_WakeStopped(t *testing.T) {
	orch, provider, db := setupOrchestrator(t)
	org := createTestOrg(t, db)
	identity := createTestIdentity(t, db, org.ID)

	ctx := context.Background()
	sb, err := orch.EnsureSharedSandbox(ctx, &org, &identity)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() {
		db.Where("id = ?", sb.ID).Delete(&model.Sandbox{})
		db.Where("org_id = ?", org.ID).Delete(&model.WorkspaceStorage{})
	})

	// Stop it
	if err := orch.StopSandbox(ctx, sb); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if provider.getStatus(sb.ExternalID) != StatusStopped {
		t.Fatal("provider should show stopped")
	}

	// EnsureSharedSandbox should wake it
	woken, err := orch.EnsureSharedSandbox(ctx, &org, &identity)
	if err != nil {
		t.Fatalf("wake: %v", err)
	}
	if woken.ID != sb.ID {
		t.Error("should return same sandbox")
	}
	if woken.Status != "running" {
		t.Errorf("status after wake: got %q", woken.Status)
	}
	if provider.getStatus(sb.ExternalID) != StatusRunning {
		t.Error("provider should show running after wake")
	}
}

func TestCreateDedicatedSandbox(t *testing.T) {
	orch, provider, db := setupOrchestrator(t)
	org := createTestOrg(t, db)
	identity := createTestIdentity(t, db, org.ID)
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
	if sb.IdentityID != identity.ID {
		t.Error("identity_id should match agent's identity")
	}
	if sb.Status != "running" {
		t.Errorf("status: got %q", sb.Status)
	}
	if provider.count() != 1 {
		t.Errorf("expected 1 sandbox, got %d", provider.count())
	}
}

func TestGetBridgeClient(t *testing.T) {
	orch, _, db := setupOrchestrator(t)
	org := createTestOrg(t, db)
	identity := createTestIdentity(t, db, org.ID)

	ctx := context.Background()
	sb, err := orch.EnsureSharedSandbox(ctx, &org, &identity)
	if err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	t.Cleanup(func() {
		db.Where("id = ?", sb.ID).Delete(&model.Sandbox{})
		db.Where("org_id = ?", org.ID).Delete(&model.WorkspaceStorage{})
	})

	client, err := orch.GetBridgeClient(ctx, sb)
	if err != nil {
		t.Fatalf("GetBridgeClient: %v", err)
	}
	if client == nil {
		t.Fatal("client should not be nil")
	}
}

func TestGetBridgeClient_RefreshesExpiredURL(t *testing.T) {
	orch, _, db := setupOrchestrator(t)
	org := createTestOrg(t, db)
	identity := createTestIdentity(t, db, org.ID)

	ctx := context.Background()
	sb, err := orch.EnsureSharedSandbox(ctx, &org, &identity)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() {
		db.Where("id = ?", sb.ID).Delete(&model.Sandbox{})
		db.Where("org_id = ?", org.ID).Delete(&model.WorkspaceStorage{})
	})

	// Expire the URL
	expired := time.Now().Add(-1 * time.Hour)
	db.Model(sb).Update("bridge_url_expires_at", expired)
	sb.BridgeURLExpiresAt = &expired

	oldURL := sb.BridgeURL

	_, err = orch.GetBridgeClient(ctx, sb)
	if err != nil {
		t.Fatalf("GetBridgeClient with expired URL: %v", err)
	}

	// URL should have been refreshed
	var refreshed model.Sandbox
	db.Where("id = ?", sb.ID).First(&refreshed)
	if refreshed.BridgeURLExpiresAt == nil || refreshed.BridgeURLExpiresAt.Before(time.Now()) {
		t.Error("bridge_url_expires_at should be refreshed to the future")
	}
	// URL stays the same in mock (same endpoint returned), but expiry changed
	_ = oldURL
}

func TestDeleteSandbox(t *testing.T) {
	orch, provider, db := setupOrchestrator(t)
	org := createTestOrg(t, db)
	identity := createTestIdentity(t, db, org.ID)

	ctx := context.Background()
	sb, err := orch.EnsureSharedSandbox(ctx, &org, &identity)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// No cleanup needed — we're testing delete
	db.Where("org_id = ?", org.ID).Delete(&model.WorkspaceStorage{})

	if err := orch.DeleteSandbox(ctx, sb); err != nil {
		t.Fatalf("DeleteSandbox: %v", err)
	}

	// Provider should have no sandboxes
	if provider.count() != 0 {
		t.Errorf("provider should have 0 sandboxes, got %d", provider.count())
	}

	// DB record should be gone
	var count int64
	db.Model(&model.Sandbox{}).Where("id = ?", sb.ID).Count(&count)
	if count != 0 {
		t.Error("sandbox record should be deleted from DB")
	}
}

func TestMultipleIdentities_SeparateSharedSandboxes(t *testing.T) {
	orch, provider, db := setupOrchestrator(t)
	org := createTestOrg(t, db)
	id1 := createTestIdentity(t, db, org.ID)
	id2 := createTestIdentity(t, db, org.ID)

	ctx := context.Background()
	sb1, err := orch.EnsureSharedSandbox(ctx, &org, &id1)
	if err != nil {
		t.Fatalf("identity1: %v", err)
	}
	sb2, err := orch.EnsureSharedSandbox(ctx, &org, &id2)
	if err != nil {
		t.Fatalf("identity2: %v", err)
	}
	t.Cleanup(func() {
		db.Where("id IN ?", []uuid.UUID{sb1.ID, sb2.ID}).Delete(&model.Sandbox{})
		db.Where("org_id = ?", org.ID).Delete(&model.WorkspaceStorage{})
	})

	if sb1.ID == sb2.ID {
		t.Error("different identities should get different shared sandboxes")
	}
	if provider.count() != 2 {
		t.Errorf("expected 2 sandboxes, got %d", provider.count())
	}
}
