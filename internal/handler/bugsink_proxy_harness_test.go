package handler_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	sentrygo "github.com/getsentry/sentry-go"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/nango"
)

const bugsinkProxyTestDBURL = "postgres://hiveloop:localdev@localhost:5433/hiveloop_test?sslmode=disable" // #nosec G101 -- test fixture, not a real secret

type bugsinkProxyHarness struct {
	db           *gorm.DB
	router       *chi.Mux
	orgID        uuid.UUID
	employeeID   uuid.UUID
	subagentID   uuid.UUID
	standaloneID uuid.UUID
	connectionID uuid.UUID
	bridgeKey    string
}

func newBugsinkProxyHarness(t *testing.T, nangoHandler http.Handler) *bugsinkProxyHarness {
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
	unattachedConnectionID := uuid.New()
	revokedConnectionID := uuid.New()

	if err := database.Create(&model.Org{ID: orgID, Name: "bugsink-proxy-" + uuid.NewString()[:8], RateLimit: 1000, Active: true}).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	if err := database.Create(&model.User{ID: userID, Email: fmt.Sprintf("bugsink-proxy-%s@example.com", uuid.NewString()[:8]), Name: "Proxy Tester"}).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	integration := createTestInIntegration(t, database, "bugsink")
	if err := database.Create(&model.InConnection{ID: connectionID, OrgID: orgID, UserID: userID, InIntegrationID: integration.ID, NangoConnectionID: "bugsink-nango-1"}).Error; err != nil {
		t.Fatalf("create connection: %v", err)
	}
	if err := database.Create(&model.InConnection{ID: unattachedConnectionID, OrgID: orgID, UserID: userID, InIntegrationID: integration.ID, NangoConnectionID: "bugsink-nango-unattached"}).Error; err != nil {
		t.Fatalf("create unattached connection: %v", err)
	}
	revokedAt := mustParseTime(t, "2026-05-16T00:00:00Z")
	if err := database.Create(&model.InConnection{ID: revokedConnectionID, OrgID: orgID, UserID: userID, InIntegrationID: integration.ID, NangoConnectionID: "bugsink-nango-revoked", RevokedAt: &revokedAt}).Error; err != nil {
		t.Fatalf("create revoked connection: %v", err)
	}

	employee := model.Agent{
		ID:           employeeID,
		OrgID:        &orgID,
		Name:         "Bugsink Employee " + uuid.NewString()[:8],
		Status:       "active",
		IsEmployee:   true,
		Integrations: model.JSON{connectionID.String(): map[string]any{"actions": []string{}}},
	}
	if err := database.Create(&employee).Error; err != nil {
		t.Fatalf("create employee: %v", err)
	}
	subagent := model.Agent{ID: subagentID, OrgID: &orgID, Name: "Bugsink Subagent " + uuid.NewString()[:8], Status: "active"}
	if err := database.Create(&subagent).Error; err != nil {
		t.Fatalf("create subagent: %v", err)
	}
	if err := database.Create(&model.AgentSubagent{AgentID: employeeID, SubagentID: subagentID}).Error; err != nil {
		t.Fatalf("link subagent: %v", err)
	}
	standalone := model.Agent{ID: standaloneID, OrgID: &orgID, Name: "Bugsink Standalone " + uuid.NewString()[:8], Status: "active"}
	if err := database.Create(&standalone).Error; err != nil {
		t.Fatalf("create standalone: %v", err)
	}

	bridgeKey := "bugsink-proxy-bridge-key"
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
		database.Where("agent_id = ? OR subagent_id IN ?", employeeID, []uuid.UUID{subagentID}).Delete(&model.AgentSubagent{})
		database.Where("org_id = ?", orgID).Delete(&model.Agent{})
		database.Where("org_id = ?", orgID).Delete(&model.InConnection{})
		database.Where("id = ?", userID).Delete(&model.User{})
		database.Where("id = ?", orgID).Delete(&model.Org{})
	})

	bugsinkProxyHandler := handler.NewBugsinkProxyHandler(database, encKey, nango.NewClient(nangoMock.URL, "test-nango-secret"))
	router := chi.NewRouter()
	router.Handle("/internal/bugsink-proxy/{agentID}/*", http.HandlerFunc(bugsinkProxyHandler.Handle))

	return &bugsinkProxyHarness{
		db:           database,
		router:       router,
		orgID:        orgID,
		employeeID:   employeeID,
		subagentID:   subagentID,
		standaloneID: standaloneID,
		connectionID: connectionID,
		bridgeKey:    bridgeKey,
	}
}

type bugsinkCaptureTransport struct {
	event atomic.Pointer[sentrygo.Event]
}

func (*bugsinkCaptureTransport) Configure(sentrygo.ClientOptions)      {}
func (t *bugsinkCaptureTransport) SendEvent(event *sentrygo.Event)     { t.event.Store(event) }
func (*bugsinkCaptureTransport) SendEvents([]*sentrygo.Event)          {}
func (*bugsinkCaptureTransport) Flush(time.Duration) bool              { return true }
func (*bugsinkCaptureTransport) FlushWithContext(context.Context) bool { return true }
func (*bugsinkCaptureTransport) Close()                                {}

func mustParseTime(t *testing.T, s string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
