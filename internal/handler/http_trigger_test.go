package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/tasks"
)

func connectHTTPTriggerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := "postgres://hivy:localdev@localhost:15432/hivy_test?sslmode=disable" // #nosec G101 -- test fixture, not a real secret
	database, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormlogger.Discard,
	})
	if err != nil {
		t.Skipf("skipping: test database not available: %v", err)
	}

	if err := database.AutoMigrate(&model.Org{}); err != nil {
		t.Fatalf("auto-migrate orgs: %v", err)
	}
	if err := database.AutoMigrate(&model.Employee{}, &model.EmployeeTrigger{}); err != nil {
		t.Fatalf("auto-migrate agent triggers: %v", err)
	}
	return database
}

type httpTriggerHarness struct {
	db      *gorm.DB
	mock    *enqueue.MockClient
	handler *HTTPTriggerHandler
	router  *chi.Mux
	orgID   uuid.UUID
	agentID uuid.UUID
}

func newHTTPTriggerHarness(t *testing.T) *httpTriggerHarness {
	t.Helper()
	database := connectHTTPTriggerTestDB(t)

	orgID := uuid.New()
	agentID := uuid.New()

	if err := database.Create(&model.Org{ID: orgID, Name: "test-org-" + orgID.String()}).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	if err := database.Create(&model.Employee{
		ID:           agentID,
		OrgID:        &orgID,
		Name:         "Employee",
		SystemPrompt: "You are an employee.",
		Model:        "test-model",
		IsEmployee:   true,
		Status:       "active",
	}).Error; err != nil {
		t.Fatalf("create employee: %v", err)
	}
	t.Cleanup(func() {
		database.Where("org_id = ?", orgID).Delete(&model.EmployeeTrigger{})
		database.Where("id = ?", agentID).Delete(&model.Employee{})
		database.Where("id = ?", orgID).Delete(&model.Org{})
	})

	mockEnqueuer := &enqueue.MockClient{}
	httpHandler := NewHTTPTriggerHandler(database, mockEnqueuer)

	chiRouter := chi.NewRouter()
	chiRouter.Post("/incoming/triggers/{triggerID}", httpHandler.Handle)

	return &httpTriggerHarness{
		db:      database,
		mock:    mockEnqueuer,
		handler: httpHandler,
		router:  chiRouter,
		orgID:   orgID,
		agentID: agentID,
	}
}

func (harness *httpTriggerHarness) createTrigger(t *testing.T, triggerType, plaintextSecret string) model.EmployeeTrigger {
	t.Helper()
	storedSecret := ""
	if plaintextSecret != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(plaintextSecret), bcrypt.MinCost)
		if err != nil {
			t.Fatalf("hash trigger secret: %v", err)
		}
		storedSecret = string(hash)
	}
	trigger := model.EmployeeTrigger{
		OrgID:       harness.orgID,
		EmployeeID:  harness.agentID,
		TriggerType: triggerType,
		Enabled:     true,
		SecretKey:   storedSecret,
	}
	if err := harness.db.Create(&trigger).Error; err != nil {
		t.Fatalf("create trigger: %v", err)
	}
	t.Cleanup(func() {
		harness.db.Where("id = ?", trigger.ID).Delete(&model.EmployeeTrigger{})
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

// TestHTTPTrigger_ValidRequest_Returns200AndEnqueues verifies that a valid HTTP trigger request
// correctly enqueues an employee trigger dispatch task with the expected payload.
func TestHTTPTrigger_ValidRequest_Returns200AndEnqueues(t *testing.T) {
	harness := newHTTPTriggerHarness(t)
	trigger := harness.createTrigger(t, "http", "")

	body := []byte(`{"action":"deploy","service":"api"}`)
	recorder := harness.doPost(t, trigger.ID.String(), body, nil)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", recorder.Code)
	}

	harness.mock.AssertEnqueued(t, tasks.TypeEmployeeTriggerDispatch)

	enqueuedTasks := harness.mock.Tasks()
	if len(enqueuedTasks) != 1 {
		t.Fatalf("expected 1 enqueued task, got %d", len(enqueuedTasks))
	}

	var payload tasks.EmployeeTriggerDispatchPayload
	if err := json.Unmarshal(enqueuedTasks[0].Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.TriggerID == nil {
		t.Fatal("TriggerID should be set")
	}
	if *payload.TriggerID != trigger.ID {
		t.Errorf("trigger ID: got %s, want %s", *payload.TriggerID, trigger.ID)
	}
	if payload.Provider != "http" {
		t.Errorf("provider: got %q, want http", payload.Provider)
	}
}

// TestHTTPTrigger_ValidBearer_Returns200 verifies that valid Bearer token authentication works.
func TestHTTPTrigger_ValidBearer_Returns200(t *testing.T) {
	secret := "test-webhook-secret-key" // #nosec G101 -- test fixture, not a real secret
	harness := newHTTPTriggerHarness(t)
	trigger := harness.createTrigger(t, "http", secret)

	recorder := harness.doPost(t, trigger.ID.String(), []byte(`{"event":"test"}`), map[string]string{
		"Authorization": "Bearer " + secret,
	})

	if recorder.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 with valid Bearer token", recorder.Code)
	}
	harness.mock.AssertEnqueued(t, tasks.TypeEmployeeTriggerDispatch)
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
	harness.mock.AssertEnqueued(t, tasks.TypeEmployeeTriggerDispatch)
}

func TestHTTPTrigger_RedactsSensitiveJSONKeysBeforeEnqueue(t *testing.T) {
	harness := newHTTPTriggerHarness(t)
	trigger := harness.createTrigger(t, "http", "")

	body := []byte(`{
		"event":"deploy",
		"api_key":"sk-test",
		"nested":{"Authorization":"Bearer test-token","safe":"kept"},
		"items":[{"password":"p4ss","value":1}]
	}`)
	recorder := harness.doPost(t, trigger.ID.String(), body, nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", recorder.Code)
	}

	enqueuedTasks := harness.mock.Tasks()
	if len(enqueuedTasks) != 1 {
		t.Fatalf("expected 1 enqueued task, got %d", len(enqueuedTasks))
	}
	var payload tasks.EmployeeTriggerDispatchPayload
	if err := json.Unmarshal(enqueuedTasks[0].Payload, &payload); err != nil {
		t.Fatalf("unmarshal task payload: %v", err)
	}
	payloadText := string(payload.PayloadJSON)
	for _, forbidden := range []string{"sk-test", "test-token", "p4ss"} {
		if strings.Contains(payloadText, forbidden) {
			t.Fatalf("payload leaked %q: %s", forbidden, payloadText)
		}
	}
	for _, want := range []string{`"api_key":"[redacted]"`, `"Authorization":"[redacted]"`, `"password":"[redacted]"`, `"safe":"kept"`} {
		if !strings.Contains(payloadText, want) {
			t.Fatalf("payload missing %s: %s", want, payloadText)
		}
	}
}

func TestHTTPTrigger_RejectsOversizedBody(t *testing.T) {
	harness := newHTTPTriggerHarness(t)
	trigger := harness.createTrigger(t, "http", "")

	body := bytes.Repeat([]byte("x"), int(maxHTTPTriggerBodyBytes)+1)
	recorder := harness.doPost(t, trigger.ID.String(), body, nil)

	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status: got %d, want %d: %s", recorder.Code, http.StatusRequestEntityTooLarge, recorder.Body.String())
	}
	if tasks := harness.mock.Tasks(); len(tasks) != 0 {
		t.Fatalf("expected no enqueued task, got %d", len(tasks))
	}
}
