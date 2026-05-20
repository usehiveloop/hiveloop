package handler_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/nango"
)

const railwayTestDBURL = "postgres://hivy:localdev@localhost:5433/hivy_test?sslmode=disable" // #nosec G101 -- test fixture, not a real secret

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

	handler.SetRailwayUpstreamURL(railwayProxyHandler, railwayMock.URL)

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

	agentID := uuid.New()
	agent := model.Agent{
		ID:     agentID,
		OrgID:  &orgID,
		Name:   "test-railway-agent",
		Status: "active",
	}
	if err := database.Create(&agent).Error; err != nil {
		t.Fatalf("create test agent: %v", err)
	}

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
		EncryptedBridgeAPIKey: encryptedKey,
		Status:                "running",
		ExternalID:            "mock-external-id",
		BridgeURL:             "http://localhost:25434",
	}
	if err := database.Create(&sandbox).Error; err != nil {
		t.Fatalf("create test sandbox: %v", err)
	}

	userID := uuid.New()
	user := model.User{
		ID:    userID,
		Email: fmt.Sprintf("railway-test-%s@example.com", uuid.New().String()[:8]),
		Name:  "Test User",
	}
	if err := database.Create(&user).Error; err != nil {
		t.Fatalf("create test user: %v", err)
	}

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
	router.Post("/internal/railway-proxy/{employeeID}", railwayProxyHandler.Handle)

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
