package handler_test

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/crypto"
	"github.com/ziraloop/ziraloop/internal/handler"
	"github.com/ziraloop/ziraloop/internal/model"
	"github.com/ziraloop/ziraloop/internal/nango"
)

const gitCredsTestDBURL = "postgres://ziraloop:localdev@localhost:5433/ziraloop_test?sslmode=disable"

func testSymmetricKey(t *testing.T) *crypto.SymmetricKey {
	t.Helper()
	key := make([]byte, 32)
	for idx := range key {
		key[idx] = byte(idx + 42)
	}
	encKey, err := crypto.NewSymmetricKey(base64.StdEncoding.EncodeToString(key))
	if err != nil {
		t.Fatal(err)
	}
	return encKey
}

type gitCredsHarness struct {
	db         *gorm.DB
	router     *chi.Mux
	encKey     *crypto.SymmetricKey
	orgID      uuid.UUID
	agentID    uuid.UUID
	sandboxID  uuid.UUID
	bridgeKey  string
	nangoMock  *httptest.Server
}

func newGitCredsHarness(t *testing.T, nangoHandler http.Handler) *gitCredsHarness {
	t.Helper()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = gitCredsTestDBURL
	}
	database, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Skipf("cannot connect to test database: %v", err)
	}
	if err := model.AutoMigrate(database); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	encKey := testSymmetricKey(t)

	nangoMock := httptest.NewServer(nangoHandler)
	t.Cleanup(nangoMock.Close)

	nangoClient := nango.NewClient(nangoMock.URL, "test-nango-secret")

	gitCredsHandler := handler.NewGitCredentialsHandler(database, encKey, nangoClient)

	// Create test org
	orgID := uuid.New()
	org := model.Org{
		ID:        orgID,
		Name:      fmt.Sprintf("gitcreds-test-%s", uuid.New().String()[:8]),
		RateLimit: 1000,
		Active:    true,
	}
	if err := database.Create(&org).Error; err != nil {
		t.Fatalf("create test org: %v", err)
	}

	// Create test agent
	agentID := uuid.New()
	agent := model.Agent{
		ID:          agentID,
		OrgID:       &orgID,
		Name:        "test-agent",
		Status:      "active",
		SandboxType: "dedicated",
	}
	if err := database.Create(&agent).Error; err != nil {
		t.Fatalf("create test agent: %v", err)
	}

	// Create sandbox with encrypted bridge API key
	bridgeKey := "test-bridge-api-key-for-git-creds"
	encryptedKey, err := encKey.EncryptString(bridgeKey)
	if err != nil {
		t.Fatalf("encrypt bridge key: %v", err)
	}

	sandboxID := uuid.New()
	sandbox := model.Sandbox{
		ID:                    sandboxID,
		OrgID:                 &orgID,
		AgentID:               &agentID,
		SandboxType:           "dedicated",
		EncryptedBridgeAPIKey: encryptedKey,
		Status:                "running",
		ExternalID:            "mock-external-id",
		BridgeURL:             "http://localhost:25434",
	}
	if err := database.Create(&sandbox).Error; err != nil {
		t.Fatalf("create test sandbox: %v", err)
	}

	// Create a user (required FK for InConnection)
	userID := uuid.New()
	user := model.User{
		ID:    userID,
		Email: fmt.Sprintf("gitcreds-test-%s@example.com", uuid.New().String()[:8]),
		Name:  "Test User",
	}
	if err := database.Create(&user).Error; err != nil {
		t.Fatalf("create test user: %v", err)
	}

	// Create in_integration + in_connection for github-app
	inIntegrationID := uuid.New()
	inIntegration := model.InIntegration{
		ID:          inIntegrationID,
		UniqueKey:   fmt.Sprintf("github-app-test-%s", uuid.New().String()[:8]),
		Provider:    "github-app",
		DisplayName: "Test GitHub App",
	}
	if err := database.Create(&inIntegration).Error; err != nil {
		t.Fatalf("create test in_integration: %v", err)
	}

	inConnectionID := uuid.New()
	inConnection := model.InConnection{
		ID:                inConnectionID,
		OrgID:             orgID,
		UserID:            userID,
		InIntegrationID:   inIntegrationID,
		NangoConnectionID: "nango-conn-123",
	}
	if err := database.Create(&inConnection).Error; err != nil {
		t.Fatalf("create test in_connection: %v", err)
	}

	t.Cleanup(func() {
		database.Where("org_id = ?", orgID).Delete(&model.InConnection{})
		database.Where("id = ?", inIntegrationID).Delete(&model.InIntegration{})
		database.Where("id = ?", sandboxID).Delete(&model.Sandbox{})
		database.Where("org_id = ?", orgID).Delete(&model.Agent{})
		database.Where("id = ?", userID).Delete(&model.User{})
		database.Where("id = ?", orgID).Delete(&model.Org{})
	})

	router := chi.NewRouter()
	router.Post("/internal/git-credentials/{agentID}", gitCredsHandler.Handle)

	return &gitCredsHarness{
		db:        database,
		router:    router,
		encKey:    encKey,
		orgID:     orgID,
		agentID:   agentID,
		sandboxID: sandboxID,
		bridgeKey: bridgeKey,
		nangoMock: nangoMock,
	}
}

func TestGitCredentials_Success(t *testing.T) {
	nangoHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"provider": "github-app",
			"credentials": map[string]any{
				"access_token": "ghs_test_installation_token",
				"token_type":   "bearer",
			},
		})
	})

	harness := newGitCredsHarness(t, nangoHandler)

	req := httptest.NewRequest(http.MethodPost,
		"/internal/git-credentials/"+harness.agentID.String(), nil)
	req.Header.Set("Authorization", "Bearer "+harness.bridgeKey)
	recorder := httptest.NewRecorder()
	harness.router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	body := recorder.Body.String()
	if body != "username=x-access-token\npassword=ghs_test_installation_token\n" {
		t.Fatalf("unexpected response body: %q", body)
	}

	if ct := recorder.Header().Get("Content-Type"); ct != "text/plain" {
		t.Fatalf("expected Content-Type text/plain, got %q", ct)
	}
}

func TestGitCredentials_CachesToken(t *testing.T) {
	callCount := 0
	nangoHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"provider": "github-app",
			"credentials": map[string]any{
				"access_token": "ghs_cached_token",
			},
		})
	})

	harness := newGitCredsHarness(t, nangoHandler)

	for range 3 {
		req := httptest.NewRequest(http.MethodPost,
			"/internal/git-credentials/"+harness.agentID.String(), nil)
		req.Header.Set("Authorization", "Bearer "+harness.bridgeKey)
		recorder := httptest.NewRecorder()
		harness.router.ServeHTTP(recorder, req)

		if recorder.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
		}
	}

	if callCount != 1 {
		t.Fatalf("expected nango to be called once (cached), got %d calls", callCount)
	}
}

func TestGitCredentials_InvalidBearerToken(t *testing.T) {
	nangoHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("nango should not be called with invalid auth")
	})

	harness := newGitCredsHarness(t, nangoHandler)

	req := httptest.NewRequest(http.MethodPost,
		"/internal/git-credentials/"+harness.agentID.String(), nil)
	req.Header.Set("Authorization", "Bearer wrong-key")
	recorder := httptest.NewRecorder()
	harness.router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestGitCredentials_MissingAuth(t *testing.T) {
	nangoHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("nango should not be called without auth")
	})

	harness := newGitCredsHarness(t, nangoHandler)

	req := httptest.NewRequest(http.MethodPost,
		"/internal/git-credentials/"+harness.agentID.String(), nil)
	recorder := httptest.NewRecorder()
	harness.router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestGitCredentials_NoGitHubConnection(t *testing.T) {
	nangoHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("nango should not be called when no connection exists")
	})

	harness := newGitCredsHarness(t, nangoHandler)

	// Delete the github-app connection
	harness.db.Where("org_id = ?", harness.orgID).Delete(&model.InConnection{})

	req := httptest.NewRequest(http.MethodPost,
		"/internal/git-credentials/"+harness.agentID.String(), nil)
	req.Header.Set("Authorization", "Bearer "+harness.bridgeKey)
	recorder := httptest.NewRecorder()
	harness.router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestGitCredentials_UnknownAgent(t *testing.T) {
	nangoHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("nango should not be called for unknown agent")
	})

	harness := newGitCredsHarness(t, nangoHandler)

	unknownID := uuid.New()
	req := httptest.NewRequest(http.MethodPost,
		"/internal/git-credentials/"+unknownID.String(), nil)
	req.Header.Set("Authorization", "Bearer "+harness.bridgeKey)
	recorder := httptest.NewRecorder()
	harness.router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", recorder.Code, recorder.Body.String())
	}
}
