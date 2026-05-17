package handler_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/nango"
)

type linearProxyHarness struct {
	db           *gorm.DB
	router       *chi.Mux
	orgID        uuid.UUID
	employeeID   uuid.UUID
	subagentID   uuid.UUID
	standaloneID uuid.UUID
	profileID    uuid.UUID
	connectionID uuid.UUID
	bridgeKey    string
	providerKey  string
}

func newLinearProxyHarness(t *testing.T, nangoHandler http.Handler) *linearProxyHarness {
	t.Helper()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = bugsinkProxyTestDBURL
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

	orgID := uuid.New()
	userID := uuid.New()
	employeeID := uuid.New()
	subagentID := uuid.New()
	standaloneID := uuid.New()
	connectionID := uuid.New()
	profileID := uuid.New()

	if err := database.Create(&model.Org{ID: orgID, Name: "linear-proxy-" + uuid.NewString()[:8], RateLimit: 1000, Active: true}).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	if err := database.Create(&model.User{ID: userID, Email: "linear-proxy-" + uuid.NewString()[:8] + "@example.com", Name: "Proxy Tester"}).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	integration := createTestInIntegration(t, database, "linear-profile")
	providerKey := "in_" + integration.UniqueKey
	if err := database.Create(&model.InConnection{ID: connectionID, OrgID: orgID, UserID: userID, InIntegrationID: integration.ID, NangoConnectionID: "linear-nango-1"}).Error; err != nil {
		t.Fatalf("create connection: %v", err)
	}

	employee := model.Agent{ID: employeeID, OrgID: &orgID, Name: "Linear Employee " + uuid.NewString()[:8], Status: "active", IsEmployee: true}
	if err := database.Create(&employee).Error; err != nil {
		t.Fatalf("create employee: %v", err)
	}
	if err := database.Create(&model.AgentProfile{
		ID:         profileID,
		OrgID:      orgID,
		AgentID:    employeeID,
		Provider:   "linear-profile",
		ExternalID: "linear-nango-1",
		Label:      "Linear Profile",
		Status:     "active",
		Config: model.JSON{
			"in_connection_id":    connectionID.String(),
			"provider_config_key": providerKey,
		},
	}).Error; err != nil {
		t.Fatalf("create profile: %v", err)
	}

	subagent := model.Agent{ID: subagentID, OrgID: &orgID, Name: "Linear Subagent " + uuid.NewString()[:8], Status: "active"}
	if err := database.Create(&subagent).Error; err != nil {
		t.Fatalf("create subagent: %v", err)
	}
	if err := database.Create(&model.AgentSubagent{AgentID: employeeID, SubagentID: subagentID}).Error; err != nil {
		t.Fatalf("link subagent: %v", err)
	}
	standalone := model.Agent{ID: standaloneID, OrgID: &orgID, Name: "Linear Standalone " + uuid.NewString()[:8], Status: "active"}
	if err := database.Create(&standalone).Error; err != nil {
		t.Fatalf("create standalone: %v", err)
	}

	bridgeKey := "linear-proxy-bridge-key"
	encryptedKey, err := encKey.EncryptString(bridgeKey)
	if err != nil {
		t.Fatalf("encrypt bridge key: %v", err)
	}
	for _, agentID := range []uuid.UUID{employeeID, subagentID, standaloneID} {
		id := uuid.New()
		if err := database.Create(&model.Sandbox{ID: id, OrgID: &orgID, AgentID: &agentID, EncryptedBridgeAPIKey: encryptedKey, Status: "running", ExternalID: "mock-" + id.String(), BridgeURL: "http://localhost:25434"}).Error; err != nil {
			t.Fatalf("create sandbox: %v", err)
		}
	}

	t.Cleanup(func() {
		database.Where("org_id = ?", orgID).Delete(&model.Sandbox{})
		database.Where("org_id = ?", orgID).Delete(&model.AgentProfile{})
		database.Where("agent_id = ? OR subagent_id = ?", employeeID, subagentID).Delete(&model.AgentSubagent{})
		database.Where("org_id = ?", orgID).Delete(&model.Agent{})
		database.Where("org_id = ?", orgID).Delete(&model.InConnection{})
		database.Where("id = ?", userID).Delete(&model.User{})
		database.Where("id = ?", orgID).Delete(&model.Org{})
	})

	linearProxyHandler := handler.NewLinearProxyHandler(database, encKey, nango.NewClient(nangoMock.URL, "test-nango-secret"))
	router := chi.NewRouter()
	router.Post("/internal/linear-proxy/{agentID}", linearProxyHandler.Handle)

	return &linearProxyHarness{
		db:           database,
		router:       router,
		orgID:        orgID,
		employeeID:   employeeID,
		subagentID:   subagentID,
		standaloneID: standaloneID,
		profileID:    profileID,
		connectionID: connectionID,
		bridgeKey:    bridgeKey,
		providerKey:  providerKey,
	}
}
