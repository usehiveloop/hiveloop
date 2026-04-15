package handler_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/handler"
	"github.com/ziraloop/ziraloop/internal/model"
	"github.com/ziraloop/ziraloop/internal/nango"
)

const railwayTestDBURL = "postgres://ziraloop:localdev@localhost:5433/ziraloop_test?sslmode=disable"

type railwayTestHarness struct {
	db          *gorm.DB
	router      *chi.Mux
	orgID       uuid.UUID
	agentID     uuid.UUID
	bridgeKey   string
	nangoMock   *httptest.Server
	railwayMock *httptest.Server
}

func newRailwayHarness(t *testing.T, nangoHandler http.Handler, railwayHandler http.Handler) *railwayTestHarness {
	t.Helper()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = railwayTestDBURL
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

	railwayMock := httptest.NewServer(railwayHandler)
	t.Cleanup(railwayMock.Close)

	railwayProxyHandler := handler.NewRailwayProxyHandler(database, encKey, nangoClient)
	// Override the upstream URL for testing
	handler.SetRailwayUpstreamURL(railwayProxyHandler, railwayMock.URL)

	// Create test org
	orgID := uuid.New()
	org := model.Org{
		ID:        orgID,
		Name:      fmt.Sprintf("railway-test-%s", uuid.New().String()[:8]),
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
		Name:        "test-railway-agent",
		Status:      "active",
		SandboxType: "dedicated",
	}
	if err := database.Create(&agent).Error; err != nil {
		t.Fatalf("create test agent: %v", err)
	}

	// Create sandbox
	bridgeKey := "test-bridge-api-key-for-railway"
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
		Email: fmt.Sprintf("railway-test-%s@example.com", uuid.New().String()[:8]),
		Name:  "Test User",
	}
	if err := database.Create(&user).Error; err != nil {
		t.Fatalf("create test user: %v", err)
	}

	// Create in_integration + in_connection for railway
	inIntegrationID := uuid.New()
	inIntegration := model.InIntegration{
		ID:          inIntegrationID,
		UniqueKey:   fmt.Sprintf("railway-test-%s", uuid.New().String()[:8]),
		Provider:    "railway",
		DisplayName: "Test Railway",
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
		NangoConnectionID: "nango-railway-conn-123",
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
	router.Post("/internal/railway-proxy/{agentID}", railwayProxyHandler.Handle)

	return &railwayTestHarness{
		db:          database,
		router:      router,
		orgID:       orgID,
		agentID:     agentID,
		bridgeKey:   bridgeKey,
		nangoMock:   nangoMock,
		railwayMock: railwayMock,
	}
}

func TestRailwayProxy_ForwardsRequestAndToken(t *testing.T) {
	var capturedAuth string
	var capturedBody string
	var mu sync.Mutex

	nangoHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"provider": "railway",
			"credentials": map[string]any{
				"access_token": "railway_test_token_abc",
			},
		})
	})

	railwayHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		capturedAuth = r.Header.Get("Authorization")
		bodyBytes, _ := io.ReadAll(r.Body)
		capturedBody = string(bodyBytes)
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"me": map[string]any{"name": "Test User"},
			},
		})
	})

	harness := newRailwayHarness(t, nangoHandler, railwayHandler)

	graphqlBody := `{"query": "query { me { name } }"}`
	req := httptest.NewRequest(http.MethodPost,
		"/internal/railway-proxy/"+harness.agentID.String(),
		bytes.NewReader([]byte(graphqlBody)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+harness.bridgeKey)
	recorder := httptest.NewRecorder()
	harness.router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	mu.Lock()
	defer mu.Unlock()

	if capturedAuth != "Bearer railway_test_token_abc" {
		t.Fatalf("expected railway to receive Bearer token, got %q", capturedAuth)
	}

	if capturedBody != graphqlBody {
		t.Fatalf("expected request body forwarded unchanged, got %q", capturedBody)
	}

	var resp map[string]any
	if err := json.NewDecoder(recorder.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	data, ok := resp["data"].(map[string]any)
	if !ok {
		t.Fatal("expected data in response")
	}
	me, ok := data["me"].(map[string]any)
	if !ok {
		t.Fatal("expected me in data")
	}
	if me["name"] != "Test User" {
		t.Fatalf("expected name=Test User, got %v", me["name"])
	}
}

func TestRailwayProxy_CachesTokenByOrg(t *testing.T) {
	nangoCallCount := 0
	var mu sync.Mutex

	nangoHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		nangoCallCount++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"provider":    "railway",
			"credentials": map[string]any{"access_token": "cached_railway_token"},
		})
	})

	railwayHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":{}}`))
	})

	harness := newRailwayHarness(t, nangoHandler, railwayHandler)

	for range 5 {
		req := httptest.NewRequest(http.MethodPost,
			"/internal/railway-proxy/"+harness.agentID.String(),
			bytes.NewReader([]byte(`{"query":"query{me{name}}"}`)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+harness.bridgeKey)
		recorder := httptest.NewRecorder()
		harness.router.ServeHTTP(recorder, req)

		if recorder.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if nangoCallCount != 1 {
		t.Fatalf("expected nango called once (cached by org), got %d", nangoCallCount)
	}
}

func TestRailwayProxy_InvalidAuth(t *testing.T) {
	nangoHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("nango should not be called with invalid auth")
	})
	railwayHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("railway should not be called with invalid auth")
	})

	harness := newRailwayHarness(t, nangoHandler, railwayHandler)

	req := httptest.NewRequest(http.MethodPost,
		"/internal/railway-proxy/"+harness.agentID.String(),
		bytes.NewReader([]byte(`{"query":"query{me{name}}"}`)))
	req.Header.Set("Authorization", "Bearer wrong-key")
	recorder := httptest.NewRecorder()
	harness.router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestRailwayProxy_NoRailwayConnection(t *testing.T) {
	nangoHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("nango should not be called when no connection exists")
	})
	railwayHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("railway should not be called when no connection exists")
	})

	harness := newRailwayHarness(t, nangoHandler, railwayHandler)

	// Delete railway connection
	harness.db.Where("org_id = ?", harness.orgID).Delete(&model.InConnection{})

	req := httptest.NewRequest(http.MethodPost,
		"/internal/railway-proxy/"+harness.agentID.String(),
		bytes.NewReader([]byte(`{"query":"query{me{name}}"}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+harness.bridgeKey)
	recorder := httptest.NewRecorder()
	harness.router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", recorder.Code, recorder.Body.String())
	}
}
