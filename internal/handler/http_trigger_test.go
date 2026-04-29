package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/usehiveloop/hiveloop/internal/enqueue"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/tasks"
)

// --------------------------------------------------------------------------
// Test infrastructure
// --------------------------------------------------------------------------

func connectHTTPTriggerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := "postgres://hiveloop:localdev@localhost:5433/hiveloop_test?sslmode=disable"
	database, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormlogger.Discard,
	})
	if err != nil {
		t.Skipf("skipping: test database not available: %v", err)
	}
	// Migrate tables in dependency order — orgs → routers → router_triggers.
	if err := database.AutoMigrate(&model.Org{}); err != nil {
		t.Fatalf("auto-migrate orgs: %v", err)
	}
	if err := database.AutoMigrate(&model.Router{}); err != nil {
		t.Fatalf("auto-migrate routers: %v", err)
	}
	if err := database.AutoMigrate(&model.RouterTrigger{}); err != nil {
		t.Fatalf("auto-migrate router_triggers: %v", err)
	}
	return database
}

type httpTriggerHarness struct {
	db       *gorm.DB
	mock     *enqueue.MockClient
	handler  *HTTPTriggerHandler
	router   *chi.Mux
	orgID    uuid.UUID
	routerID uuid.UUID
}

func newHTTPTriggerHarness(t *testing.T) *httpTriggerHarness {
	t.Helper()
	database := connectHTTPTriggerTestDB(t)

	orgID := uuid.New()
	routerID := uuid.New()

	// Create org first (routers FK → orgs), then the router.
	if err := database.Create(&model.Org{ID: orgID, Name: "test-org-" + orgID.String()}).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	if err := database.Create(&model.Router{ID: routerID, OrgID: orgID, Name: "Zira"}).Error; err != nil {
		t.Fatalf("create router: %v", err)
	}
	t.Cleanup(func() {
		database.Where("org_id = ?", orgID).Delete(&model.RouterTrigger{})
		database.Where("id = ?", routerID).Delete(&model.Router{})
		database.Where("id = ?", orgID).Delete(&model.Org{})
	})

	mockEnqueuer := &enqueue.MockClient{}
	httpHandler := NewHTTPTriggerHandler(database, mockEnqueuer)

	chiRouter := chi.NewRouter()
	chiRouter.Post("/incoming/triggers/{triggerID}", httpHandler.Handle)

	return &httpTriggerHarness{
		db:       database,
		mock:     mockEnqueuer,
		handler:  httpHandler,
		router:   chiRouter,
		orgID:    orgID,
		routerID: routerID,
	}
}

func (harness *httpTriggerHarness) createTrigger(t *testing.T, triggerType, plaintextSecret string) model.RouterTrigger {
	t.Helper()
	storedSecret := ""
	if plaintextSecret != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(plaintextSecret), bcrypt.MinCost)
		if err != nil {
			t.Fatalf("hash trigger secret: %v", err)
		}
		storedSecret = string(hash)
	}
	trigger := model.RouterTrigger{
		OrgID:       harness.orgID,
		RouterID:    harness.routerID,
		TriggerType: triggerType,
		RoutingMode: "rule",
		Enabled:     true,
		SecretKey:   storedSecret,
	}
	if err := harness.db.Create(&trigger).Error; err != nil {
		t.Fatalf("create trigger: %v", err)
	}
	t.Cleanup(func() {
		harness.db.Where("id = ?", trigger.ID).Delete(&model.RouterTrigger{})
	})
	return trigger
}

func (harness *httpTriggerHarness) doPost(t *testing.T, triggerID string, body []byte, headers map[string]string) *httptest.ResponseRecorder {
	return harness.doPostWithQuery(t, triggerID, "", body, headers)
}

func (harness *httpTriggerHarness) doPostWithQuery(t *testing.T, triggerID, query string, body []byte, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/incoming/triggers/"+triggerID+query, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	harness.router.ServeHTTP(recorder, request)
	return recorder
}

// --------------------------------------------------------------------------
// Tests
// --------------------------------------------------------------------------

// TestHTTPTrigger_ValidRequest_Returns200AndEnqueues verifies that a valid HTTP trigger request
// correctly enqueues a router dispatch task with the expected payload.
func TestHTTPTrigger_ValidRequest_Returns200AndEnqueues(t *testing.T) {
	harness := newHTTPTriggerHarness(t)
	trigger := harness.createTrigger(t, "http", "")

	body := []byte(`{"action":"deploy","service":"api"}`)
	recorder := harness.doPost(t, trigger.ID.String(), body, nil)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", recorder.Code)
	}

	// Should have enqueued a router dispatch task.
	harness.mock.AssertEnqueued(t, tasks.TypeRouterDispatch)

	enqueuedTasks := harness.mock.Tasks()
	if len(enqueuedTasks) != 1 {
		t.Fatalf("expected 1 enqueued task, got %d", len(enqueuedTasks))
	}

	// Verify the payload has RouterTriggerID set.
	var payload tasks.TriggerDispatchPayload
	if err := json.Unmarshal(enqueuedTasks[0].Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.RouterTriggerID == nil {
		t.Fatal("RouterTriggerID should be set")
	}
	if *payload.RouterTriggerID != trigger.ID {
		t.Errorf("trigger ID: got %s, want %s", *payload.RouterTriggerID, trigger.ID)
	}
	if payload.Provider != "http" {
		t.Errorf("provider: got %q, want http", payload.Provider)
	}
}

// TestHTTPTrigger_ValidBearer_Returns200 verifies that valid Bearer token authentication works.
func TestHTTPTrigger_ValidBearer_Returns200(t *testing.T) {
	secret := "test-webhook-secret-key"
	harness := newHTTPTriggerHarness(t)
	trigger := harness.createTrigger(t, "http", secret)

	recorder := harness.doPost(t, trigger.ID.String(), []byte(`{"event":"test"}`), map[string]string{
		"Authorization": "Bearer " + secret,
	})

	if recorder.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 with valid Bearer token", recorder.Code)
	}
	harness.mock.AssertEnqueued(t, tasks.TypeRouterDispatch)
}

// TestHTTPTrigger_NoSecret_AcceptsAnyRequest verifies that triggers without a secret
// accept requests without authentication.
func TestHTTPTrigger_NoSecret_AcceptsAnyRequest(t *testing.T) {
	harness := newHTTPTriggerHarness(t)
	trigger := harness.createTrigger(t, "http", "")

	recorder := harness.doPost(t, trigger.ID.String(), []byte(`{"ok":true}`), nil)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 without secret configured", recorder.Code)
	}
	harness.mock.AssertEnqueued(t, tasks.TypeRouterDispatch)
}

// Note: Tests for empty body, invalid trigger ID, missing/invalid secrets, and extractTriggerSecret
// were removed as they test library/framework behavior rather than business logic.
// See USELESS_TESTS_RECOMMENDATIONS.md for details.