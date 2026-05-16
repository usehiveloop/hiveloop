package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
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

func TestBugsinkProxy_EmployeeForwardsRawRequest(t *testing.T) {
	var captured struct {
		method       string
		path         string
		query        string
		auth         string
		providerKey  string
		connectionID string
		contentType  string
	}
	var mu sync.Mutex
	harness := newBugsinkProxyHarness(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		captured.method = r.Method
		captured.path = r.URL.Path
		captured.query = r.URL.RawQuery
		captured.auth = r.Header.Get("Authorization")
		captured.providerKey = r.Header.Get("Provider-Config-Key")
		captured.connectionID = r.Header.Get("Connection-Id")
		captured.contentType = r.Header.Get("Content-Type")
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-RateLimit-Remaining", "99")
		_, _ = w.Write([]byte(`{"projects":[{"id":"1","name":"api"}]}`))
	}))

	req := httptest.NewRequest(http.MethodGet, "/internal/bugsink-proxy/"+harness.employeeID.String()+"/api/canonical/0/projects/?cursor=abc", nil)
	req.Header.Set("Authorization", "Bearer "+harness.bridgeKey)
	rec := httptest.NewRecorder()
	harness.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("content-type = %q", got)
	}
	if got := rec.Header().Get("X-RateLimit-Remaining"); got != "99" {
		t.Fatalf("rate limit header = %q", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if captured.method != http.MethodGet || captured.path != "/proxy/api/canonical/0/projects/" || captured.query != "cursor=abc" {
		t.Fatalf("captured request = %+v", captured)
	}
	if captured.auth != "Bearer test-nango-secret" || captured.providerKey == "" || captured.connectionID != "bugsink-nango-1" {
		t.Fatalf("captured nango headers = %+v", captured)
	}
}

func TestBugsinkProxy_SubagentUsesOwningEmployeeConnection(t *testing.T) {
	var connectionID string
	harness := newBugsinkProxyHarness(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connectionID = r.Header.Get("Connection-Id")
		_, _ = w.Write([]byte(`ok`))
	}))

	req := httptest.NewRequest(http.MethodGet, "/internal/bugsink-proxy/"+harness.subagentID.String()+"/api/canonical/0/issues/", nil)
	req.Header.Set("Authorization", "Bearer "+harness.bridgeKey)
	rec := httptest.NewRecorder()
	harness.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if connectionID != "bugsink-nango-1" {
		t.Fatalf("connection id = %q", connectionID)
	}
}

func TestBugsinkProxy_RejectsInvalidAndUnattachedRequests(t *testing.T) {
	var nangoCalls atomic.Int64
	harness := newBugsinkProxyHarness(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nangoCalls.Add(1)
		_, _ = w.Write([]byte(`ok`))
	}))

	tests := []struct {
		name    string
		agentID uuid.UUID
		path    string
		token   string
		want    int
	}{
		{name: "invalid bearer", agentID: harness.employeeID, path: "/api/canonical/0/projects/", token: "bad", want: http.StatusUnauthorized},
		{name: "standalone agent", agentID: harness.standaloneID, path: "/api/canonical/0/projects/", token: harness.bridgeKey, want: http.StatusNotFound},
		{name: "invalid path", agentID: harness.employeeID, path: "/api/other/", token: harness.bridgeKey, want: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/internal/bugsink-proxy/"+tt.agentID.String()+tt.path, nil)
			req.Header.Set("Authorization", "Bearer "+tt.token)
			rec := httptest.NewRecorder()
			harness.router.ServeHTTP(rec, req)
			if rec.Code != tt.want {
				t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
	if got := nangoCalls.Load(); got != 0 {
		t.Fatalf("nango calls = %d", got)
	}
}

func TestBugsinkProxy_RequiresAttachedConnection(t *testing.T) {
	var nangoCalls atomic.Int64
	harness := newBugsinkProxyHarness(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nangoCalls.Add(1)
	}))
	if err := harness.db.Model(&model.Agent{}).Where("id = ?", harness.employeeID).Update("integrations", model.JSON{}).Error; err != nil {
		t.Fatalf("clear integrations: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/internal/bugsink-proxy/"+harness.employeeID.String()+"/api/canonical/0/projects/", nil)
	req.Header.Set("Authorization", "Bearer "+harness.bridgeKey)
	rec := httptest.NewRecorder()
	harness.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if got := nangoCalls.Load(); got != 0 {
		t.Fatalf("nango calls = %d", got)
	}
}

func TestBugsinkProxy_ForwardsPostAndUpstreamErrors(t *testing.T) {
	var capturedBody string
	harness := newBugsinkProxyHarness(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		capturedBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"forbidden"}`))
	}))

	req := httptest.NewRequest(http.MethodPost, "/internal/bugsink-proxy/"+harness.employeeID.String()+"/api/canonical/0/issues/", bytes.NewReader([]byte(`{"query":"x"}`)))
	req.Header.Set("Authorization", "Bearer "+harness.bridgeKey)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	harness.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if capturedBody != `{"query":"x"}` {
		t.Fatalf("body = %q", capturedBody)
	}
	if rec.Body.String() != `{"error":"forbidden"}` {
		t.Fatalf("response body = %q", rec.Body.String())
	}
}

func TestBugsinkProxy_NangoTransportFailureReturnsBadGateway(t *testing.T) {
	harness := newBugsinkProxyHarness(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	badHandler := handler.NewBugsinkProxyHandler(harness.db, testSymmetricKey(t), nango.NewClient("http://127.0.0.1:1", "test-nango-secret"))
	router := chi.NewRouter()
	router.Handle("/internal/bugsink-proxy/{agentID}/*", http.HandlerFunc(badHandler.Handle))

	req := httptest.NewRequest(http.MethodGet, "/internal/bugsink-proxy/"+harness.employeeID.String()+"/api/canonical/0/projects/", nil)
	req.Header.Set("Authorization", "Bearer "+harness.bridgeKey)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestBugsinkProxy_SentryCapturesSafeContext(t *testing.T) {
	transport := &bugsinkCaptureTransport{}
	if err := sentrygo.Init(sentrygo.ClientOptions{Dsn: "https://public@example.com/1", Transport: transport, EnableTracing: false}); err != nil {
		t.Fatalf("sentry init: %v", err)
	}
	t.Cleanup(func() { sentrygo.Flush(0) })

	harness := newBugsinkProxyHarness(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"token":"must-not-leak"}`))
	}))
	req := httptest.NewRequest(http.MethodGet, "/internal/bugsink-proxy/"+harness.employeeID.String()+"/api/canonical/0/projects/?secret=must-not-leak", nil)
	req.Header.Set("Authorization", "Bearer "+harness.bridgeKey)
	rec := httptest.NewRecorder()
	harness.router.ServeHTTP(rec, req)
	sentrygo.Flush(0)

	event := transport.event.Load()
	if event == nil {
		t.Fatal("expected sentry event")
	}
	if got := event.Tags["bugsink_proxy"]; got != "true" {
		t.Fatalf("bugsink_proxy tag = %q", got)
	}
	if got := event.Tags["http.status_code"]; got != "403" {
		t.Fatalf("status tag = %q", got)
	}
	exceptionValues := make([]string, 0, len(event.Exception))
	for _, ex := range event.Exception {
		exceptionValues = append(exceptionValues, ex.Value)
	}
	blob, _ := json.Marshal(map[string]any{
		"message":    event.Message,
		"tags":       event.Tags,
		"exceptions": exceptionValues,
	})
	if strings.Contains(string(blob), "secret=must-not-leak") || strings.Contains(string(blob), `{"token":"must-not-leak"}`) || strings.Contains(string(blob), harness.bridgeKey) {
		t.Fatalf("sentry event leaked sensitive request context: %s", string(blob))
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
