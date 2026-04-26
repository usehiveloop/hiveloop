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

func TestHTTPTrigger_EmptyBody_UsesEmptyJSON(t *testing.T) {
	harness := newHTTPTriggerHarness(t)
	trigger := harness.createTrigger(t, "http", "")

	recorder := harness.doPost(t, trigger.ID.String(), nil, nil)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", recorder.Code)
	}

	enqueuedTasks := harness.mock.Tasks()
	if len(enqueuedTasks) != 1 {
		t.Fatalf("expected 1 enqueued task, got %d", len(enqueuedTasks))
	}

	var payload tasks.TriggerDispatchPayload
	json.Unmarshal(enqueuedTasks[0].Payload, &payload)
	if string(payload.PayloadJSON) != "{}" {
		t.Errorf("payload: got %q, want {}", string(payload.PayloadJSON))
	}
}

func TestHTTPTrigger_InvalidTriggerID_Returns400(t *testing.T) {
	harness := newHTTPTriggerHarness(t)

	recorder := harness.doPost(t, "not-a-uuid", []byte(`{}`), nil)

	if recorder.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want 400", recorder.Code)
	}
}

func TestHTTPTrigger_TriggerNotFound_Returns404(t *testing.T) {
	harness := newHTTPTriggerHarness(t)

	recorder := harness.doPost(t, uuid.New().String(), []byte(`{}`), nil)

	if recorder.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want 404", recorder.Code)
	}
}

func TestHTTPTrigger_WrongTriggerType_Returns404(t *testing.T) {
	harness := newHTTPTriggerHarness(t)
	// Create a webhook trigger, not http.
	trigger := harness.createTrigger(t, "webhook", "")

	recorder := harness.doPost(t, trigger.ID.String(), []byte(`{}`), nil)

	if recorder.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want 404 for wrong trigger type", recorder.Code)
	}
}

func TestHTTPTrigger_DisabledTrigger_Returns404(t *testing.T) {
	harness := newHTTPTriggerHarness(t)
	trigger := harness.createTrigger(t, "http", "")
	// Disable the trigger.
	harness.db.Model(&trigger).Update("enabled", false)

	recorder := harness.doPost(t, trigger.ID.String(), []byte(`{}`), nil)

	if recorder.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want 404 for disabled trigger", recorder.Code)
	}
}

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

func TestHTTPTrigger_ValidApiKeyHeader_Returns200(t *testing.T) {
	secret := "test-webhook-secret-key"
	harness := newHTTPTriggerHarness(t)
	trigger := harness.createTrigger(t, "http", secret)

	recorder := harness.doPost(t, trigger.ID.String(), []byte(`{}`), map[string]string{
		"X-Api-Key": secret,
	})

	if recorder.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 with X-Api-Key", recorder.Code)
	}
}

func TestHTTPTrigger_ValidWebhookSecretHeader_Returns200(t *testing.T) {
	secret := "test-webhook-secret-key"
	harness := newHTTPTriggerHarness(t)
	trigger := harness.createTrigger(t, "http", secret)

	recorder := harness.doPost(t, trigger.ID.String(), []byte(`{}`), map[string]string{
		"X-Webhook-Secret": secret,
	})

	if recorder.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 with X-Webhook-Secret", recorder.Code)
	}
}

func TestHTTPTrigger_ValidQuerySecret_Returns200(t *testing.T) {
	secret := "test-webhook-secret-key"
	harness := newHTTPTriggerHarness(t)
	trigger := harness.createTrigger(t, "http", secret)

	recorder := harness.doPostWithQuery(t, trigger.ID.String(), "?secret="+secret, []byte(`{}`), nil)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 with ?secret query", recorder.Code)
	}
}

func TestHTTPTrigger_InvalidSecret_Returns401(t *testing.T) {
	secret := "test-webhook-secret-key"
	harness := newHTTPTriggerHarness(t)
	trigger := harness.createTrigger(t, "http", secret)

	recorder := harness.doPost(t, trigger.ID.String(), []byte(`{"event":"test"}`), map[string]string{
		"Authorization": "Bearer wrong-secret",
	})

	if recorder.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d, want 401 with invalid secret", recorder.Code)
	}
	if len(harness.mock.Tasks()) != 0 {
		t.Error("should not enqueue any tasks with invalid secret")
	}
}

func TestHTTPTrigger_MissingSecret_Returns401(t *testing.T) {
	secret := "test-webhook-secret-key"
	harness := newHTTPTriggerHarness(t)
	trigger := harness.createTrigger(t, "http", secret)

	recorder := harness.doPost(t, trigger.ID.String(), []byte(`{}`), nil)

	if recorder.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d, want 401 with missing secret", recorder.Code)
	}
}

func TestHTTPTrigger_NoSecret_AcceptsAnyRequest(t *testing.T) {
	harness := newHTTPTriggerHarness(t)
	trigger := harness.createTrigger(t, "http", "")

	recorder := harness.doPost(t, trigger.ID.String(), []byte(`{"ok":true}`), nil)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 without secret configured", recorder.Code)
	}
	harness.mock.AssertEnqueued(t, tasks.TypeRouterDispatch)
}

// --------------------------------------------------------------------------
// extractTriggerSecret unit tests
// --------------------------------------------------------------------------

func TestExtractTriggerSecret_BearerHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/incoming/triggers/x", nil)
	req.Header.Set("Authorization", "Bearer the-secret")
	if got := extractTriggerSecret(req); got != "the-secret" {
		t.Errorf("got %q, want %q", got, "the-secret")
	}
}

func TestExtractTriggerSecret_ApiKeyHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/incoming/triggers/x", nil)
	req.Header.Set("X-Api-Key", "the-secret")
	if got := extractTriggerSecret(req); got != "the-secret" {
		t.Errorf("got %q, want %q", got, "the-secret")
	}
}

func TestExtractTriggerSecret_WebhookSecretHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/incoming/triggers/x", nil)
	req.Header.Set("X-Webhook-Secret", "the-secret")
	if got := extractTriggerSecret(req); got != "the-secret" {
		t.Errorf("got %q, want %q", got, "the-secret")
	}
}

func TestExtractTriggerSecret_QueryParam(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/incoming/triggers/x?secret=the-secret", nil)
	if got := extractTriggerSecret(req); got != "the-secret" {
		t.Errorf("got %q, want %q", got, "the-secret")
	}
}

func TestExtractTriggerSecret_None(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/incoming/triggers/x", nil)
	if got := extractTriggerSecret(req); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestExtractTriggerSecret_BearerWins(t *testing.T) {
	// Bearer takes precedence over other transports when multiple are set.
	req := httptest.NewRequest(http.MethodPost, "/incoming/triggers/x?secret=from-query", nil)
	req.Header.Set("Authorization", "Bearer from-bearer")
	req.Header.Set("X-Api-Key", "from-api-key")
	if got := extractTriggerSecret(req); got != "from-bearer" {
		t.Errorf("got %q, want %q", got, "from-bearer")
	}
}
