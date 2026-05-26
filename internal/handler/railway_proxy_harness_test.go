package handler_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/nango"
	"github.com/usehivy/hivy/internal/testdb"
)

const railwayTestDBURL = testdb.DefaultDatabaseURL

type railwayTestHarness struct {
	db            *gorm.DB
	router        *chi.Mux
	orgID         uuid.UUID
	agentID       uuid.UUID
	runtimeSecret string
	nangoMock     *httptest.Server
	railwayMock   *httptest.Server
}

func newRailwayHarness(t *testing.T, nangoHandler http.Handler, railwayHandler http.Handler) *railwayTestHarness {
	t.Helper()

	dsn := testdb.DatabaseURL("DATABASE_URL", "HIVY_DATABASE_URL", "TEST_DATABASE_URL")
	database, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("cannot connect to test database: %v", err)
	}
	testdb.ApplyMigrations(t, database)

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
	agent := model.Employee{
		ID:     agentID,
		OrgID:  &orgID,
		Name:   "test-railway-agent",
		Status: "active",
	}
	if err := database.Create(&agent).Error; err != nil {
		t.Fatalf("create test agent: %v", err)
	}

	runtimeSecret := "test-runtime-api-key-for-railway"
	encryptedKey, err := encKey.EncryptString(runtimeSecret)
	if err != nil {
		t.Fatalf("encrypt runtime secret: %v", err)
	}

	sandboxID := uuid.New()
	sandbox := model.Sandbox{
		ID:                     sandboxID,
		OrgID:                  &orgID,
		EmployeeID:             &agentID,
		EncryptedRuntimeSecret: encryptedKey,
		Status:                 "running",
		ExternalID:             "mock-external-id",
		RuntimeURL:             "http://localhost:25434",
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

	integrationID := uuid.New()
	integration := model.Integration{
		ID:          integrationID,
		UniqueKey:   fmt.Sprintf("railway-test-%s", uuid.New().String()[:8]),
		Provider:    "railway",
		DisplayName: "Test Railway",
	}
	if err := database.Create(&integration).Error; err != nil {
		t.Fatalf("create test in_integration: %v", err)
	}

	connectionID := uuid.New()
	connection := model.Connection{
		ID:                connectionID,
		OrgID:             orgID,
		UserID:            userID,
		IntegrationID:     integrationID,
		NangoConnectionID: "nango-railway-conn-123",
	}
	if err := database.Create(&connection).Error; err != nil {
		t.Fatalf("create test in_connection: %v", err)
	}

	t.Cleanup(func() {
		database.Where("org_id = ?", orgID).Delete(&model.Connection{})
		database.Where("id = ?", integrationID).Delete(&model.Integration{})
		database.Where("id = ?", sandboxID).Delete(&model.Sandbox{})
		database.Where("org_id = ?", orgID).Delete(&model.Employee{})
		database.Where("id = ?", userID).Delete(&model.User{})
		database.Where("id = ?", orgID).Delete(&model.Org{})
	})

	router := chi.NewRouter()
	router.Post("/internal/railway-proxy/{employeeID}", railwayProxyHandler.Handle)

	return &railwayTestHarness{
		db:            database,
		router:        router,
		orgID:         orgID,
		agentID:       agentID,
		runtimeSecret: runtimeSecret,
		nangoMock:     nangoMock,
		railwayMock:   railwayMock,
	}
}
