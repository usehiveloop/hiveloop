package handler_test

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/nango"
)

type notionProxyHarness struct {
	db           *gorm.DB
	router       *chi.Mux
	orgID        uuid.UUID
	userID       uuid.UUID
	employeeID   uuid.UUID
	subagentID   uuid.UUID
	standaloneID uuid.UUID
	profileID    uuid.UUID
	connectionID uuid.UUID
	bridgeKey    string
	providerKey  string
}

func newNotionProxyHarness(t *testing.T, nangoHandler http.Handler) *notionProxyHarness {
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
	profileID := uuid.New()
	connectionID := uuid.New()

	if err := database.Create(&model.Org{ID: orgID, Name: "notion-proxy-" + uuid.NewString()[:8], RateLimit: 1000, Active: true}).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	if err := database.Create(&model.User{ID: userID, Email: fmt.Sprintf("notion-proxy-%s@example.com", uuid.NewString()[:8]), Name: "Proxy Tester"}).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	integration := createTestInIntegration(t, database, "notion-profile")
	providerKey := "in_" + integration.UniqueKey
	if err := database.Create(&model.InConnection{ID: connectionID, OrgID: orgID, UserID: userID, InIntegrationID: integration.ID, NangoConnectionID: "notion-nango-1"}).Error; err != nil {
		t.Fatalf("create connection: %v", err)
	}

	employee := model.Agent{
		ID:         employeeID,
		OrgID:      &orgID,
		Name:       "Notion Employee " + uuid.NewString()[:8],
		Status:     "active",
		IsEmployee: true,
	}
	if err := database.Create(&employee).Error; err != nil {
		t.Fatalf("create employee: %v", err)
	}
	if err := database.Create(&model.AgentProfile{
		ID:         profileID,
		OrgID:      orgID,
		AgentID:    employeeID,
		Provider:   "notion-profile",
		ExternalID: "notion-nango-1",
		Label:      "Notion Profile",
		Status:     "active",
		Config: model.JSON{
			"in_connection_id":    connectionID.String(),
			"provider_config_key": providerKey,
		},
	}).Error; err != nil {
		t.Fatalf("create profile: %v", err)
	}
	subagent := model.Agent{ID: subagentID, OrgID: &orgID, Name: "Notion Subagent " + uuid.NewString()[:8], Status: "active"}
	if err := database.Create(&subagent).Error; err != nil {
		t.Fatalf("create subagent: %v", err)
	}
	if err := database.Create(&model.AgentSubagent{AgentID: employeeID, SubagentID: subagentID}).Error; err != nil {
		t.Fatalf("link subagent: %v", err)
	}
	standalone := model.Agent{ID: standaloneID, OrgID: &orgID, Name: "Notion Standalone " + uuid.NewString()[:8], Status: "active"}
	if err := database.Create(&standalone).Error; err != nil {
		t.Fatalf("create standalone: %v", err)
	}

	bridgeKey := "notion-proxy-bridge-key"
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

	notionProxyHandler := handler.NewNotionProxyHandler(database, encKey, nango.NewClient(nangoMock.URL, "test-nango-secret"))
	router := chi.NewRouter()
	router.Handle("/internal/notion-proxy/{agentID}/*", http.HandlerFunc(notionProxyHandler.Handle))

	return &notionProxyHarness{
		db:           database,
		router:       router,
		orgID:        orgID,
		userID:       userID,
		employeeID:   employeeID,
		subagentID:   subagentID,
		standaloneID: standaloneID,
		profileID:    profileID,
		connectionID: connectionID,
		bridgeKey:    bridgeKey,
		providerKey:  providerKey,
	}
}

func TestNotionProxy_EmployeeForwardsVersionedRequest(t *testing.T) {
	var captured struct {
		method        string
		path          string
		query         string
		auth          string
		providerKey   string
		connectionID  string
		contentType   string
		notionVersion string
		body          string
	}
	var mu sync.Mutex
	harness := newNotionProxyHarness(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		captured.method = r.Method
		captured.path = r.URL.Path
		captured.query = r.URL.RawQuery
		captured.auth = r.Header.Get("Authorization")
		captured.providerKey = r.Header.Get("Provider-Config-Key")
		captured.connectionID = r.Header.Get("Connection-Id")
		captured.contentType = r.Header.Get("Content-Type")
		captured.notionVersion = r.Header.Get("Notion-Version")
		captured.body = string(body)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-RateLimit-Remaining", "42")
		_, _ = w.Write([]byte(`{"object":"page","id":"page-1","url":"https://www.notion.so/page-1"}`))
	}))

	req := httptest.NewRequest(http.MethodPatch, "/internal/notion-proxy/"+harness.employeeID.String()+"/v1/pages/page-1?filter=value", bytes.NewReader([]byte(`{"archived":false}`)))
	req.Header.Set("Authorization", "Bearer "+harness.bridgeKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Notion-Version", "2022-06-28")
	rec := httptest.NewRecorder()
	harness.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("X-RateLimit-Remaining"); got != "42" {
		t.Fatalf("rate limit header = %q", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if captured.method != http.MethodPatch || captured.path != "/proxy/v1/pages/page-1" || captured.query != "filter=value" {
		t.Fatalf("captured request = %+v", captured)
	}
	if captured.auth != "Bearer test-nango-secret" || captured.providerKey != harness.providerKey || captured.connectionID != "notion-nango-1" {
		t.Fatalf("captured nango headers = %+v", captured)
	}
	if captured.contentType != "application/json" || captured.notionVersion != "2022-06-28" {
		t.Fatalf("captured notion headers = %+v", captured)
	}
	if captured.body != `{"archived":false}` {
		t.Fatalf("body = %q", captured.body)
	}
}

func TestNotionProxy_SubagentUsesOwningEmployeeConnection(t *testing.T) {
	var connectionID string
	harness := newNotionProxyHarness(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connectionID = r.Header.Get("Connection-Id")
		_, _ = w.Write([]byte(`ok`))
	}))

	req := httptest.NewRequest(http.MethodGet, "/internal/notion-proxy/"+harness.subagentID.String()+"/v1/users/me", nil)
	req.Header.Set("Authorization", "Bearer "+harness.bridgeKey)
	req.Header.Set("Notion-Version", "2022-06-28")
	rec := httptest.NewRecorder()
	harness.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if connectionID != "notion-nango-1" {
		t.Fatalf("connection id = %q", connectionID)
	}
}

func TestNotionProxy_RejectsInvalidAndUnattachedRequests(t *testing.T) {
	var nangoCalls atomic.Int64
	harness := newNotionProxyHarness(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		{name: "invalid bearer", agentID: harness.employeeID, path: "/v1/users/me", token: "bad", want: http.StatusUnauthorized},
		{name: "standalone agent", agentID: harness.standaloneID, path: "/v1/users/me", token: harness.bridgeKey, want: http.StatusNotFound},
		{name: "invalid path", agentID: harness.employeeID, path: "/users/me", token: harness.bridgeKey, want: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/internal/notion-proxy/"+tt.agentID.String()+tt.path, nil)
			req.Header.Set("Authorization", "Bearer "+tt.token)
			req.Header.Set("Notion-Version", "2022-06-28")
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

func TestNotionProxy_RequiresActiveProfile(t *testing.T) {
	var nangoCalls atomic.Int64
	harness := newNotionProxyHarness(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nangoCalls.Add(1)
	}))
	if err := harness.db.Model(&model.AgentProfile{}).Where("id = ?", harness.profileID).Update("status", "revoked").Error; err != nil {
		t.Fatalf("revoke profile: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/internal/notion-proxy/"+harness.employeeID.String()+"/v1/users/me", nil)
	req.Header.Set("Authorization", "Bearer "+harness.bridgeKey)
	req.Header.Set("Notion-Version", "2022-06-28")
	rec := httptest.NewRecorder()
	harness.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "no notion profile") {
		t.Fatalf("body = %s", rec.Body.String())
	}
	if got := nangoCalls.Load(); got != 0 {
		t.Fatalf("nango calls = %d", got)
	}
}

func TestNotionProxy_RegularNotionConnectionDoesNotSatisfyProfile(t *testing.T) {
	var nangoCalls atomic.Int64
	harness := newNotionProxyHarness(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nangoCalls.Add(1)
	}))
	if err := harness.db.Delete(&model.AgentProfile{}, "id = ?", harness.profileID).Error; err != nil {
		t.Fatalf("delete profile: %v", err)
	}
	regularIntegration := createTestInIntegration(t, harness.db, "notion")
	regularConnectionID := uuid.New()
	if err := harness.db.Create(&model.InConnection{ID: regularConnectionID, OrgID: harness.orgID, UserID: harness.userID, InIntegrationID: regularIntegration.ID, NangoConnectionID: "regular-notion-nango"}).Error; err != nil {
		t.Fatalf("create regular notion connection: %v", err)
	}
	if err := harness.db.Model(&model.Agent{}).Where("id = ?", harness.employeeID).Update("integrations", model.JSON{regularConnectionID.String(): map[string]any{"actions": []string{}}}).Error; err != nil {
		t.Fatalf("attach regular notion connection: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/internal/notion-proxy/"+harness.employeeID.String()+"/v1/users/me", nil)
	req.Header.Set("Authorization", "Bearer "+harness.bridgeKey)
	req.Header.Set("Notion-Version", "2022-06-28")
	rec := httptest.NewRecorder()
	harness.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if got := nangoCalls.Load(); got != 0 {
		t.Fatalf("nango calls = %d", got)
	}
}
