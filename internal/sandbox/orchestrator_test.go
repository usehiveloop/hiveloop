package sandbox

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/config"
	"github.com/usehiveloop/hiveloop/internal/crypto"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/turso"
)

const testDBURL = "postgres://hiveloop:localdev@localhost:5433/hiveloop_test?sslmode=disable"

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
	db.Exec("DELETE FROM sandboxes")
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
		BridgeBaseImagePrefix:           "hiveloop-bridge-0-10-0",
		BridgeHost:                      "test.hiveloop.com",
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

func createTestAgent(t *testing.T, db *gorm.DB, orgID, credID uuid.UUID, sandboxType string) model.Agent {
	t.Helper()
	suffix := uuid.New().String()[:8]
	agent := model.Agent{
		OrgID: &orgID, Name: "agent-" + suffix,
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
