package sandbox

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/testdb"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := testdb.DatabaseURL("DATABASE_URL", "HIVY_DATABASE_URL", "TEST_DATABASE_URL")
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	sqlDB, _ := db.DB()
	testdb.ApplyMigrations(t, db)
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

func setupOrchestrator(t *testing.T) (*Orchestrator, *mockProvider, *gorm.DB) {
	t.Helper()
	db := setupTestDB(t)

	bridgeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(200)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		w.WriteHeader(404)
	}))
	t.Cleanup(bridgeSrv.Close)

	provider := newMockProvider()
	provider.endpointOverride = bridgeSrv.URL

	cfg := &config.Config{
		SandboxesRuntimeSpecialistImagePrefix: "hivy-sandboxes-runtime-specialist-test-small-v1",
		SpecialistSandboxHost:                 "test.usehivy.com",
		SpecialistSandboxGracePeriodMins:      5,
	}

	orch := NewOrchestrator(db, provider, testEncKey(t), cfg)
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

func createTestAgent(t *testing.T, db *gorm.DB, orgID, credID uuid.UUID) model.Employee {
	t.Helper()
	suffix := uuid.New().String()[:8]
	agent := model.Employee{
		OrgID: &orgID, Name: "agent-" + suffix,
		CredentialID: &credID,
		SystemPrompt: "test", Model: "gpt-4o",
	}
	db.Create(&agent)
	t.Cleanup(func() { db.Where("id = ?", agent.ID).Delete(&model.Employee{}) })
	return agent
}

func createTestCred(t *testing.T, db *gorm.DB, orgID uuid.UUID) model.Credential {
	t.Helper()
	cred := model.Credential{
		OrgID: &orgID, BaseURL: "https://api.openai.com", AuthScheme: "bearer",
		ProviderID: "openai", EncryptedKey: []byte("enc"), WrappedDEK: []byte("dek"),
	}
	db.Create(&cred)
	t.Cleanup(func() { db.Where("id = ?", cred.ID).Delete(&model.Credential{}) })
	return cred
}
