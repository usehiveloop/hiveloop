package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/ziraloop/ziraloop/internal/handler"
	"github.com/ziraloop/ziraloop/internal/mcp/catalog"
	"github.com/ziraloop/ziraloop/internal/middleware"
	"github.com/ziraloop/ziraloop/internal/model"
	"github.com/ziraloop/ziraloop/internal/nango"
)

// nangoConnMockConfig configures the Nango mock for connection tests.
type nangoConnMockConfig struct {
	mu               sync.Mutex
	capturedPaths    []string
	capturedMethods  []string
	capturedBodies   [][]byte
	connectStatus    int
	getConnStatus    int
	deleteConnStatus int
}

func newNangoConnMock(cfg *nangoConnMockConfig) http.Handler {
	if cfg.connectStatus == 0 {
		cfg.connectStatus = http.StatusOK
	}
	if cfg.getConnStatus == 0 {
		cfg.getConnStatus = http.StatusOK
	}
	if cfg.deleteConnStatus == 0 {
		cfg.deleteConnStatus = http.StatusOK
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		cfg.mu.Lock()
		cfg.capturedPaths = append(cfg.capturedPaths, r.URL.Path)
		cfg.capturedMethods = append(cfg.capturedMethods, r.Method)
		cfg.capturedBodies = append(cfg.capturedBodies, body)
		cfg.mu.Unlock()

		// Provider catalog
		if r.URL.Path == "/providers" && r.Method == http.MethodGet {
			json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{"name": "github", "display_name": "GitHub", "auth_mode": "OAUTH2"},
					{"name": "slack", "display_name": "Slack", "auth_mode": "OAUTH2"},
					{"name": "notion", "display_name": "Notion", "auth_mode": "OAUTH2"},
				},
			})
			return
		}

		// Connect sessions
		if r.URL.Path == "/connect/sessions" && r.Method == http.MethodPost {
			w.WriteHeader(cfg.connectStatus)
			if cfg.connectStatus >= 400 {
				json.NewEncoder(w).Encode(map[string]any{"error": "nango error"})
				return
			}
			json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"token":      "test-connect-token",
					"expires_at": time.Now().Add(15 * time.Minute).Format(time.RFC3339),
				},
			})
			return
		}

		// Get connection
		if strings.HasPrefix(r.URL.Path, "/connection/") && r.Method == http.MethodGet {
			w.WriteHeader(cfg.getConnStatus)
			if cfg.getConnStatus >= 400 {
				json.NewEncoder(w).Encode(map[string]any{"error": "nango error"})
				return
			}
			json.NewEncoder(w).Encode(map[string]any{
				"provider":          "github",
				"connection_config": map[string]any{"org": "ziraloop"},
				"credentials":      map[string]any{"access_token": "gho_xxxx"},
			})
			return
		}

		// Delete connection
		if strings.HasPrefix(r.URL.Path, "/connection/") && r.Method == http.MethodDelete {
			w.WriteHeader(cfg.deleteConnStatus)
			if cfg.deleteConnStatus >= 400 {
				json.NewEncoder(w).Encode(map[string]any{"error": "nango error"})
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
			return
		}

		w.WriteHeader(http.StatusNotFound)
	})
}

// ---------------------------------------------------------------------------
// Harness
// ---------------------------------------------------------------------------


// ---------------------------------------------------------------------------
// Create tests
// ---------------------------------------------------------------------------

func TestInConnectionHandler_Create_Success(t *testing.T) {
	db := connectTestDB(t)
	t.Cleanup(func() {
		db.Where("1=1").Delete(&model.InConnection{})
		db.Where("1=1").Delete(&model.InIntegration{})
	})

	mockCfg := &nangoConnMockConfig{}
	nangoSrv := httptest.NewServer(newNangoConnMock(mockCfg))
	t.Cleanup(nangoSrv.Close)
	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	nangoClient.FetchProviders(context.Background())

	h := handler.NewInConnectionHandler(db, nangoClient, catalog.Global())
	r := chi.NewRouter()
	r.Post("/v1/in/integrations/{id}/connections", h.Create)

	user := createTestUser(t, db, fmt.Sprintf("conn-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, db)
	integ := createTestInIntegration(t, db, "github")

	body, _ := json.Marshal(map[string]any{
		"nango_connection_id": "nango-conn-123",
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/in/integrations/"+integ.ID.String()+"/connections", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithUser(req, &user)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["in_integration_id"] != integ.ID.String() {
		t.Fatalf("expected in_integration_id=%s, got %v", integ.ID.String(), resp["in_integration_id"])
	}
	if resp["provider"] != "github" {
		t.Fatalf("expected provider=github, got %v", resp["provider"])
	}
	if resp["nango_connection_id"] != "nango-conn-123" {
		t.Fatalf("expected nango_connection_id=nango-conn-123, got %v", resp["nango_connection_id"])
	}

	// Verify in DB
	var conn model.InConnection
	if err := db.Where("id = ?", resp["id"]).First(&conn).Error; err != nil {
		t.Fatalf("connection not found in DB: %v", err)
	}
	if conn.UserID != user.ID {
		t.Fatalf("expected user_id=%s, got %s", user.ID, conn.UserID)
	}
}

func TestInConnectionHandler_Create_MissingNangoConnectionID(t *testing.T) {
	db := connectTestDB(t)
	t.Cleanup(func() {
		db.Where("1=1").Delete(&model.InConnection{})
		db.Where("1=1").Delete(&model.InIntegration{})
	})

	nangoSrv := httptest.NewServer(newNangoConnMock(&nangoConnMockConfig{}))
	t.Cleanup(nangoSrv.Close)
	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	nangoClient.FetchProviders(context.Background())

	h := handler.NewInConnectionHandler(db, nangoClient, catalog.Global())
	r := chi.NewRouter()
	r.Post("/v1/in/integrations/{id}/connections", h.Create)

	user := createTestUser(t, db, fmt.Sprintf("conn-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, db)
	integ := createTestInIntegration(t, db, "github")

	body, _ := json.Marshal(map[string]any{})
	req := httptest.NewRequest(http.MethodPost, "/v1/in/integrations/"+integ.ID.String()+"/connections", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithUser(req, &user)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestInConnectionHandler_Create_IntegrationNotFound(t *testing.T) {
	db := connectTestDB(t)
	t.Cleanup(func() {
		db.Where("1=1").Delete(&model.InConnection{})
		db.Where("1=1").Delete(&model.InIntegration{})
	})

	nangoSrv := httptest.NewServer(newNangoConnMock(&nangoConnMockConfig{}))
	t.Cleanup(nangoSrv.Close)
	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	nangoClient.FetchProviders(context.Background())

	h := handler.NewInConnectionHandler(db, nangoClient, catalog.Global())
	r := chi.NewRouter()
	r.Post("/v1/in/integrations/{id}/connections", h.Create)

	user := createTestUser(t, db, fmt.Sprintf("conn-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, db)

	body, _ := json.Marshal(map[string]any{"nango_connection_id": "nango-conn-123"})
	req := httptest.NewRequest(http.MethodPost, "/v1/in/integrations/"+uuid.New().String()+"/connections", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithUser(req, &user)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestInConnectionHandler_Create_DeletedIntegration(t *testing.T) {
	db := connectTestDB(t)
	t.Cleanup(func() {
		db.Where("1=1").Delete(&model.InConnection{})
		db.Where("1=1").Delete(&model.InIntegration{})
	})

	nangoSrv := httptest.NewServer(newNangoConnMock(&nangoConnMockConfig{}))
	t.Cleanup(nangoSrv.Close)
	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	nangoClient.FetchProviders(context.Background())

	h := handler.NewInConnectionHandler(db, nangoClient, catalog.Global())
	r := chi.NewRouter()
	r.Post("/v1/in/integrations/{id}/connections", h.Create)

	user := createTestUser(t, db, fmt.Sprintf("conn-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, db)
	integ := createTestInIntegration(t, db, "github")
	now := time.Now()
	db.Model(&integ).Update("deleted_at", now)

	body, _ := json.Marshal(map[string]any{"nango_connection_id": "nango-conn-123"})
	req := httptest.NewRequest(http.MethodPost, "/v1/in/integrations/"+integ.ID.String()+"/connections", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithUser(req, &user)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestInConnectionHandler_Create_DuplicateUserIntegration(t *testing.T) {
	db := connectTestDB(t)
	t.Cleanup(func() {
		db.Where("1=1").Delete(&model.InConnection{})
		db.Where("1=1").Delete(&model.InIntegration{})
	})

	nangoSrv := httptest.NewServer(newNangoConnMock(&nangoConnMockConfig{}))
	t.Cleanup(nangoSrv.Close)
	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	nangoClient.FetchProviders(context.Background())

	h := handler.NewInConnectionHandler(db, nangoClient, catalog.Global())
	r := chi.NewRouter()
	r.Post("/v1/in/integrations/{id}/connections", h.Create)

	user := createTestUser(t, db, fmt.Sprintf("conn-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, db)
	integ := createTestInIntegration(t, db, "github")

	// Create first connection directly
	db.Create(&model.InConnection{
		ID:                uuid.New(),
		OrgID:             org.ID,
		UserID:            user.ID,
		InIntegrationID:   integ.ID,
		NangoConnectionID: "first-conn",
	})

	// Second connection for the same user+integration is allowed (different nango connection).
	body, _ := json.Marshal(map[string]any{"nango_connection_id": "second-conn"})
	req := httptest.NewRequest(http.MethodPost, "/v1/in/integrations/"+integ.ID.String()+"/connections", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithUser(req, &user)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify both connections exist.
	var count int64
	db.Model(&model.InConnection{}).Where("user_id = ? AND in_integration_id = ?", user.ID, integ.ID).Count(&count)
	if count != 2 {
		t.Fatalf("expected 2 connections, got %d", count)
	}
}

func TestInConnectionHandler_Create_WithMeta(t *testing.T) {
	db := connectTestDB(t)
	t.Cleanup(func() {
		db.Where("1=1").Delete(&model.InConnection{})
		db.Where("1=1").Delete(&model.InIntegration{})
	})

	nangoSrv := httptest.NewServer(newNangoConnMock(&nangoConnMockConfig{}))
	t.Cleanup(nangoSrv.Close)
	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	nangoClient.FetchProviders(context.Background())

	h := handler.NewInConnectionHandler(db, nangoClient, catalog.Global())
	r := chi.NewRouter()
	r.Post("/v1/in/integrations/{id}/connections", h.Create)

	user := createTestUser(t, db, fmt.Sprintf("conn-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, db)
	integ := createTestInIntegration(t, db, "github")

	body, _ := json.Marshal(map[string]any{
		"nango_connection_id": "nango-conn-meta",
		"meta":               map[string]any{"resources": map[string]any{"repos": []string{"ziraloop"}}},
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/in/integrations/"+integ.ID.String()+"/connections", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithUser(req, &user)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	meta, ok := resp["meta"].(map[string]any)
	if !ok || meta["resources"] == nil {
		t.Fatalf("expected meta.resources to be set, got %v", resp["meta"])
	}
}

// ---------------------------------------------------------------------------
// List tests
// ---------------------------------------------------------------------------

func TestInConnectionHandler_List_Success(t *testing.T) {
	db := connectTestDB(t)
	t.Cleanup(func() {
		db.Where("1=1").Delete(&model.InConnection{})
		db.Where("1=1").Delete(&model.InIntegration{})
	})

	nangoSrv := httptest.NewServer(newNangoConnMock(&nangoConnMockConfig{}))
	t.Cleanup(nangoSrv.Close)
	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	nangoClient.FetchProviders(context.Background())

	h := handler.NewInConnectionHandler(db, nangoClient, catalog.Global())
	r := chi.NewRouter()
	r.Get("/v1/in/connections", h.List)

	user := createTestUser(t, db, fmt.Sprintf("conn-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, db)
	integ1 := createTestInIntegration(t, db, "github")
	integ2 := model.InIntegration{
		ID: uuid.New(), UniqueKey: fmt.Sprintf("slack-%s", uuid.New().String()[:8]),
		Provider: "slack", DisplayName: "Slack built-in",
	}
	db.Create(&integ2)

	for i, integ := range []model.InIntegration{integ1, integ2} {
		db.Create(&model.InConnection{
			ID: uuid.New(), OrgID: org.ID, UserID: user.ID, InIntegrationID: integ.ID,
			NangoConnectionID: fmt.Sprintf("conn-%d", i),
		})
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/in/connections", nil)
	req = middleware.WithUser(req, &user)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var page struct {
		Data []map[string]any `json:"data"`
	}
	json.NewDecoder(rr.Body).Decode(&page)
	if len(page.Data) != 2 {
		t.Fatalf("expected 2 connections, got %d", len(page.Data))
	}
}

func TestInConnectionHandler_List_UserIsolation(t *testing.T) {
	db := connectTestDB(t)
	t.Cleanup(func() {
		db.Where("1=1").Delete(&model.InConnection{})
		db.Where("1=1").Delete(&model.InIntegration{})
	})

	nangoSrv := httptest.NewServer(newNangoConnMock(&nangoConnMockConfig{}))
	t.Cleanup(nangoSrv.Close)
	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	nangoClient.FetchProviders(context.Background())

	h := handler.NewInConnectionHandler(db, nangoClient, catalog.Global())
	r := chi.NewRouter()
	r.Get("/v1/in/connections", h.List)

	user1 := createTestUser(t, db, fmt.Sprintf("user1-%s@test.com", uuid.New().String()[:8]))
	user2 := createTestUser(t, db, fmt.Sprintf("user2-%s@test.com", uuid.New().String()[:8]))
	org1 := createTestOrg(t, db)
	org2 := createTestOrg(t, db)
	integ := createTestInIntegration(t, db, "github")

	db.Create(&model.InConnection{
		ID: uuid.New(), OrgID: org1.ID, UserID: user1.ID, InIntegrationID: integ.ID, NangoConnectionID: "user1-conn",
	})

	// Create a second integration for user2 to avoid unique constraint
	integ2 := model.InIntegration{
		ID: uuid.New(), UniqueKey: fmt.Sprintf("slack-%s", uuid.New().String()[:8]),
		Provider: "slack", DisplayName: "Slack built-in",
	}
	db.Create(&integ2)
	db.Create(&model.InConnection{
		ID: uuid.New(), OrgID: org2.ID, UserID: user2.ID, InIntegrationID: integ2.ID, NangoConnectionID: "user2-conn",
	})

	// User2 should NOT see user1's connections (different org)
	req := httptest.NewRequest(http.MethodGet, "/v1/in/connections", nil)
	req = middleware.WithUser(req, &user2)
	req = middleware.WithOrg(req, &org2)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	var page struct {
		Data []map[string]any `json:"data"`
	}
	json.NewDecoder(rr.Body).Decode(&page)
	for _, item := range page.Data {
		if item["nango_connection_id"] == "user1-conn" {
			t.Fatal("user2 should not see user1's connection")
		}
	}
}

func TestInConnectionHandler_List_ExcludesRevoked(t *testing.T) {
	db := connectTestDB(t)
	t.Cleanup(func() {
		db.Where("1=1").Delete(&model.InConnection{})
		db.Where("1=1").Delete(&model.InIntegration{})
	})

	nangoSrv := httptest.NewServer(newNangoConnMock(&nangoConnMockConfig{}))
	t.Cleanup(nangoSrv.Close)
	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	nangoClient.FetchProviders(context.Background())

	h := handler.NewInConnectionHandler(db, nangoClient, catalog.Global())
	r := chi.NewRouter()
	r.Get("/v1/in/connections", h.List)

	user := createTestUser(t, db, fmt.Sprintf("conn-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, db)
	integ := createTestInIntegration(t, db, "github")

	now := time.Now()
	connID := uuid.New()
	db.Create(&model.InConnection{
		ID: connID, OrgID: org.ID, UserID: user.ID, InIntegrationID: integ.ID,
		NangoConnectionID: "revoked-conn", RevokedAt: &now,
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/in/connections", nil)
	req = middleware.WithUser(req, &user)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	var page struct {
		Data []map[string]any `json:"data"`
	}
	json.NewDecoder(rr.Body).Decode(&page)
	for _, item := range page.Data {
		if item["id"] == connID.String() {
			t.Fatal("revoked connection should not appear in list")
		}
	}
}

func TestInConnectionHandler_List_Pagination(t *testing.T) {
	db := connectTestDB(t)
	t.Cleanup(func() {
		db.Where("1=1").Delete(&model.InConnection{})
		db.Where("1=1").Delete(&model.InIntegration{})
	})

	nangoSrv := httptest.NewServer(newNangoConnMock(&nangoConnMockConfig{}))
	t.Cleanup(nangoSrv.Close)
	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	nangoClient.FetchProviders(context.Background())

	h := handler.NewInConnectionHandler(db, nangoClient, catalog.Global())
	r := chi.NewRouter()
	r.Get("/v1/in/connections", h.List)

	user := createTestUser(t, db, fmt.Sprintf("conn-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, db)

	// Create 5 connections with different integrations
	for i := 0; i < 5; i++ {
		provider := fmt.Sprintf("provider-pg-%d-%s", i, uuid.New().String()[:8])
		integ := model.InIntegration{
			ID: uuid.New(), UniqueKey: fmt.Sprintf("%s-%s", provider, uuid.New().String()[:8]),
			Provider: provider, DisplayName: provider,
		}
		db.Create(&integ)
		db.Create(&model.InConnection{
			ID: uuid.New(), OrgID: org.ID, UserID: user.ID, InIntegrationID: integ.ID,
			NangoConnectionID: fmt.Sprintf("pg-conn-%d", i),
		})
		time.Sleep(time.Millisecond)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/in/connections?limit=2", nil)
	req = middleware.WithUser(req, &user)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	var page1 struct {
		Data       []map[string]any `json:"data"`
		HasMore    bool             `json:"has_more"`
		NextCursor *string          `json:"next_cursor"`
	}
	json.NewDecoder(rr.Body).Decode(&page1)
	if len(page1.Data) != 2 {
		t.Fatalf("expected 2 items, got %d", len(page1.Data))
	}
	if !page1.HasMore {
		t.Fatal("expected has_more=true")
	}
}

func TestInConnectionHandler_List_FilterByProvider(t *testing.T) {
	db := connectTestDB(t)
	t.Cleanup(func() {
		db.Where("1=1").Delete(&model.InConnection{})
		db.Where("1=1").Delete(&model.InIntegration{})
	})

	nangoSrv := httptest.NewServer(newNangoConnMock(&nangoConnMockConfig{}))
	t.Cleanup(nangoSrv.Close)
	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	nangoClient.FetchProviders(context.Background())

	h := handler.NewInConnectionHandler(db, nangoClient, catalog.Global())
	r := chi.NewRouter()
	r.Get("/v1/in/connections", h.List)

	user := createTestUser(t, db, fmt.Sprintf("conn-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, db)
	ghInteg := createTestInIntegration(t, db, "github")
	slackInteg := model.InIntegration{
		ID: uuid.New(), UniqueKey: fmt.Sprintf("slack-%s", uuid.New().String()[:8]),
		Provider: "slack", DisplayName: "Slack built-in",
	}
	db.Create(&slackInteg)

	db.Create(&model.InConnection{
		ID: uuid.New(), OrgID: org.ID, UserID: user.ID, InIntegrationID: ghInteg.ID, NangoConnectionID: "gh-conn",
	})
	db.Create(&model.InConnection{
		ID: uuid.New(), OrgID: org.ID, UserID: user.ID, InIntegrationID: slackInteg.ID, NangoConnectionID: "slack-conn",
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/in/connections?provider=github", nil)
	req = middleware.WithUser(req, &user)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	var page struct {
		Data []map[string]any `json:"data"`
	}
	json.NewDecoder(rr.Body).Decode(&page)
	if len(page.Data) != 1 {
		t.Fatalf("expected 1 github connection, got %d", len(page.Data))
	}
	if page.Data[0]["provider"] != "github" {
		t.Fatalf("expected provider=github, got %v", page.Data[0]["provider"])
	}
}

// ---------------------------------------------------------------------------
// Get tests
// ---------------------------------------------------------------------------

func TestInConnectionHandler_Get_Success(t *testing.T) {
	db := connectTestDB(t)
	t.Cleanup(func() {
		db.Where("1=1").Delete(&model.InConnection{})
		db.Where("1=1").Delete(&model.InIntegration{})
	})

	nangoSrv := httptest.NewServer(newNangoConnMock(&nangoConnMockConfig{}))
	t.Cleanup(nangoSrv.Close)
	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	nangoClient.FetchProviders(context.Background())

	h := handler.NewInConnectionHandler(db, nangoClient, catalog.Global())
	r := chi.NewRouter()
	r.Get("/v1/in/connections/{id}", h.Get)

	user := createTestUser(t, db, fmt.Sprintf("conn-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, db)
	integ := createTestInIntegration(t, db, "github")
	connID := uuid.New()
	db.Create(&model.InConnection{
		ID: connID, OrgID: org.ID, UserID: user.ID, InIntegrationID: integ.ID, NangoConnectionID: "get-conn",
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/in/connections/"+connID.String(), nil)
	req = middleware.WithUser(req, &user)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["id"] != connID.String() {
		t.Fatalf("expected id=%s, got %v", connID.String(), resp["id"])
	}
	if resp["provider"] != "github" {
		t.Fatalf("expected provider=github, got %v", resp["provider"])
	}
}

func TestInConnectionHandler_Get_NotFound(t *testing.T) {
	db := connectTestDB(t)
	nangoSrv := httptest.NewServer(newNangoConnMock(&nangoConnMockConfig{}))
	t.Cleanup(nangoSrv.Close)
	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	nangoClient.FetchProviders(context.Background())

	h := handler.NewInConnectionHandler(db, nangoClient, catalog.Global())
	r := chi.NewRouter()
	r.Get("/v1/in/connections/{id}", h.Get)

	user := createTestUser(t, db, fmt.Sprintf("conn-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, db)

	req := httptest.NewRequest(http.MethodGet, "/v1/in/connections/"+uuid.New().String(), nil)
	req = middleware.WithUser(req, &user)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestInConnectionHandler_Get_WrongUser(t *testing.T) {
	db := connectTestDB(t)
	t.Cleanup(func() {
		db.Where("1=1").Delete(&model.InConnection{})
		db.Where("1=1").Delete(&model.InIntegration{})
	})

	nangoSrv := httptest.NewServer(newNangoConnMock(&nangoConnMockConfig{}))
	t.Cleanup(nangoSrv.Close)
	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	nangoClient.FetchProviders(context.Background())

	h := handler.NewInConnectionHandler(db, nangoClient, catalog.Global())
	r := chi.NewRouter()
	r.Get("/v1/in/connections/{id}", h.Get)

	user1 := createTestUser(t, db, fmt.Sprintf("user1-%s@test.com", uuid.New().String()[:8]))
	user2 := createTestUser(t, db, fmt.Sprintf("user2-%s@test.com", uuid.New().String()[:8]))
	org1 := createTestOrg(t, db)
	org2 := createTestOrg(t, db)
	integ := createTestInIntegration(t, db, "github")
	connID := uuid.New()
	db.Create(&model.InConnection{
		ID: connID, OrgID: org1.ID, UserID: user1.ID, InIntegrationID: integ.ID, NangoConnectionID: "user1-conn",
	})

	// User2 tries to access user1's connection from a different org
	req := httptest.NewRequest(http.MethodGet, "/v1/in/connections/"+connID.String(), nil)
	req = middleware.WithUser(req, &user2)
	req = middleware.WithOrg(req, &org2)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestInConnectionHandler_Get_RevokedNotFound(t *testing.T) {
	db := connectTestDB(t)
	t.Cleanup(func() {
		db.Where("1=1").Delete(&model.InConnection{})
		db.Where("1=1").Delete(&model.InIntegration{})
	})

	nangoSrv := httptest.NewServer(newNangoConnMock(&nangoConnMockConfig{}))
	t.Cleanup(nangoSrv.Close)
	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	nangoClient.FetchProviders(context.Background())

	h := handler.NewInConnectionHandler(db, nangoClient, catalog.Global())
	r := chi.NewRouter()
	r.Get("/v1/in/connections/{id}", h.Get)

	user := createTestUser(t, db, fmt.Sprintf("conn-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, db)
	integ := createTestInIntegration(t, db, "github")
	now := time.Now()
	connID := uuid.New()
	db.Create(&model.InConnection{
		ID: connID, OrgID: org.ID, UserID: user.ID, InIntegrationID: integ.ID,
		NangoConnectionID: "revoked-conn", RevokedAt: &now,
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/in/connections/"+connID.String(), nil)
	req = middleware.WithUser(req, &user)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestInConnectionHandler_Get_WithNangoProviderConfig(t *testing.T) {
	db := connectTestDB(t)
	t.Cleanup(func() {
		db.Where("1=1").Delete(&model.InConnection{})
		db.Where("1=1").Delete(&model.InIntegration{})
	})

	nangoSrv := httptest.NewServer(newNangoConnMock(&nangoConnMockConfig{}))
	t.Cleanup(nangoSrv.Close)
	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	nangoClient.FetchProviders(context.Background())

	h := handler.NewInConnectionHandler(db, nangoClient, catalog.Global())
	r := chi.NewRouter()
	r.Get("/v1/in/connections/{id}", h.Get)

	user := createTestUser(t, db, fmt.Sprintf("conn-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, db)
	integ := createTestInIntegration(t, db, "github")
	connID := uuid.New()
	db.Create(&model.InConnection{
		ID: connID, OrgID: org.ID, UserID: user.ID, InIntegrationID: integ.ID, NangoConnectionID: "pc-conn",
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/in/connections/"+connID.String(), nil)
	req = middleware.WithUser(req, &user)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	pc, ok := resp["provider_config"].(map[string]any)
	if !ok || pc == nil {
		t.Fatal("expected provider_config to be present")
	}
	if pc["provider"] != "github" {
		t.Fatalf("expected provider_config.provider=github, got %v", pc["provider"])
	}
}

func TestInConnectionHandler_Get_NangoFailure(t *testing.T) {
	db := connectTestDB(t)
	t.Cleanup(func() {
		db.Where("1=1").Delete(&model.InConnection{})
		db.Where("1=1").Delete(&model.InIntegration{})
	})

	mockCfg := &nangoConnMockConfig{getConnStatus: http.StatusInternalServerError}
	nangoSrv := httptest.NewServer(newNangoConnMock(mockCfg))
	t.Cleanup(nangoSrv.Close)
	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	nangoClient.FetchProviders(context.Background())

	h := handler.NewInConnectionHandler(db, nangoClient, catalog.Global())
	r := chi.NewRouter()
	r.Get("/v1/in/connections/{id}", h.Get)

	user := createTestUser(t, db, fmt.Sprintf("conn-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, db)
	integ := createTestInIntegration(t, db, "github")
	connID := uuid.New()
	db.Create(&model.InConnection{
		ID: connID, OrgID: org.ID, UserID: user.ID, InIntegrationID: integ.ID, NangoConnectionID: "fail-conn",
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/in/connections/"+connID.String(), nil)
	req = middleware.WithUser(req, &user)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	// Should still return 200 with connection data but without provider_config
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 (graceful degradation), got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["id"] != connID.String() {
		t.Fatalf("expected connection id, got %v", resp["id"])
	}
}

// ---------------------------------------------------------------------------
// Revoke tests
// ---------------------------------------------------------------------------

func TestInConnectionHandler_Revoke_Success(t *testing.T) {
	db := connectTestDB(t)
	t.Cleanup(func() {
		db.Where("1=1").Delete(&model.InConnection{})
		db.Where("1=1").Delete(&model.InIntegration{})
	})

	mockCfg := &nangoConnMockConfig{}
	nangoSrv := httptest.NewServer(newNangoConnMock(mockCfg))
	t.Cleanup(nangoSrv.Close)
	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	nangoClient.FetchProviders(context.Background())

	h := handler.NewInConnectionHandler(db, nangoClient, catalog.Global())
	r := chi.NewRouter()
	r.Delete("/v1/in/connections/{id}", h.Revoke)

	user := createTestUser(t, db, fmt.Sprintf("conn-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, db)
	integ := createTestInIntegration(t, db, "github")
	connID := uuid.New()
	db.Create(&model.InConnection{
		ID: connID, OrgID: org.ID, UserID: user.ID, InIntegrationID: integ.ID, NangoConnectionID: "revoke-conn",
	})

	req := httptest.NewRequest(http.MethodDelete, "/v1/in/connections/"+connID.String(), nil)
	req = middleware.WithUser(req, &user)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify soft-revoked in DB
	var conn model.InConnection
	db.Where("id = ?", connID).First(&conn)
	if conn.RevokedAt == nil {
		t.Fatal("expected revoked_at to be set")
	}

	// Verify Nango received DELETE
	mockCfg.mu.Lock()
	foundDelete := false
	for _, m := range mockCfg.capturedMethods {
		if m == http.MethodDelete {
			foundDelete = true
		}
	}
	mockCfg.mu.Unlock()
	if !foundDelete {
		t.Fatal("expected Nango to receive DELETE for connection")
	}
}

func TestInConnectionHandler_Revoke_NotFound(t *testing.T) {
	db := connectTestDB(t)
	nangoSrv := httptest.NewServer(newNangoConnMock(&nangoConnMockConfig{}))
	t.Cleanup(nangoSrv.Close)
	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	nangoClient.FetchProviders(context.Background())

	h := handler.NewInConnectionHandler(db, nangoClient, catalog.Global())
	r := chi.NewRouter()
	r.Delete("/v1/in/connections/{id}", h.Revoke)

	user := createTestUser(t, db, fmt.Sprintf("conn-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, db)

	req := httptest.NewRequest(http.MethodDelete, "/v1/in/connections/"+uuid.New().String(), nil)
	req = middleware.WithUser(req, &user)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestInConnectionHandler_Revoke_WrongUser(t *testing.T) {
	db := connectTestDB(t)
	t.Cleanup(func() {
		db.Where("1=1").Delete(&model.InConnection{})
		db.Where("1=1").Delete(&model.InIntegration{})
	})

	nangoSrv := httptest.NewServer(newNangoConnMock(&nangoConnMockConfig{}))
	t.Cleanup(nangoSrv.Close)
	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	nangoClient.FetchProviders(context.Background())

	h := handler.NewInConnectionHandler(db, nangoClient, catalog.Global())
	r := chi.NewRouter()
	r.Delete("/v1/in/connections/{id}", h.Revoke)

	user1 := createTestUser(t, db, fmt.Sprintf("user1-%s@test.com", uuid.New().String()[:8]))
	user2 := createTestUser(t, db, fmt.Sprintf("user2-%s@test.com", uuid.New().String()[:8]))
	org1 := createTestOrg(t, db)
	org2 := createTestOrg(t, db)
	integ := createTestInIntegration(t, db, "github")
	connID := uuid.New()
	db.Create(&model.InConnection{
		ID: connID, OrgID: org1.ID, UserID: user1.ID, InIntegrationID: integ.ID, NangoConnectionID: "user1-conn",
	})

	req := httptest.NewRequest(http.MethodDelete, "/v1/in/connections/"+connID.String(), nil)
	req = middleware.WithUser(req, &user2)
	req = middleware.WithOrg(req, &org2)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestInConnectionHandler_Revoke_AlreadyRevoked(t *testing.T) {
	db := connectTestDB(t)
	t.Cleanup(func() {
		db.Where("1=1").Delete(&model.InConnection{})
		db.Where("1=1").Delete(&model.InIntegration{})
	})

	nangoSrv := httptest.NewServer(newNangoConnMock(&nangoConnMockConfig{}))
	t.Cleanup(nangoSrv.Close)
	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	nangoClient.FetchProviders(context.Background())

	h := handler.NewInConnectionHandler(db, nangoClient, catalog.Global())
	r := chi.NewRouter()
	r.Delete("/v1/in/connections/{id}", h.Revoke)

	user := createTestUser(t, db, fmt.Sprintf("conn-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, db)
	integ := createTestInIntegration(t, db, "github")
	now := time.Now()
	connID := uuid.New()
	db.Create(&model.InConnection{
		ID: connID, OrgID: org.ID, UserID: user.ID, InIntegrationID: integ.ID,
		NangoConnectionID: "already-revoked", RevokedAt: &now,
	})

	req := httptest.NewRequest(http.MethodDelete, "/v1/in/connections/"+connID.String(), nil)
	req = middleware.WithUser(req, &user)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestInConnectionHandler_Revoke_NangoFailure(t *testing.T) {
	db := connectTestDB(t)
	t.Cleanup(func() {
		db.Where("1=1").Delete(&model.InConnection{})
		db.Where("1=1").Delete(&model.InIntegration{})
	})

	mockCfg := &nangoConnMockConfig{deleteConnStatus: http.StatusInternalServerError}
	nangoSrv := httptest.NewServer(newNangoConnMock(mockCfg))
	t.Cleanup(nangoSrv.Close)
	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	nangoClient.FetchProviders(context.Background())

	h := handler.NewInConnectionHandler(db, nangoClient, catalog.Global())
	r := chi.NewRouter()
	r.Delete("/v1/in/connections/{id}", h.Revoke)

	user := createTestUser(t, db, fmt.Sprintf("conn-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, db)
	integ := createTestInIntegration(t, db, "github")
	connID := uuid.New()
	db.Create(&model.InConnection{
		ID: connID, OrgID: org.ID, UserID: user.ID, InIntegrationID: integ.ID, NangoConnectionID: "nango-fail-conn",
	})

	req := httptest.NewRequest(http.MethodDelete, "/v1/in/connections/"+connID.String(), nil)
	req = middleware.WithUser(req, &user)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	// Should still revoke locally even if Nango fails (graceful degradation)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 (graceful degradation), got %d: %s", rr.Code, rr.Body.String())
	}

	var conn model.InConnection
	db.Where("id = ?", connID).First(&conn)
	if conn.RevokedAt == nil {
		t.Fatal("expected revoked_at to be set despite Nango failure")
	}
}

// ---------------------------------------------------------------------------
// CreateConnectSession tests
// ---------------------------------------------------------------------------

func TestInConnectionHandler_CreateConnectSession_Success(t *testing.T) {
	db := connectTestDB(t)
	t.Cleanup(func() {
		db.Where("1=1").Delete(&model.InConnection{})
		db.Where("1=1").Delete(&model.InIntegration{})
	})

	mockCfg := &nangoConnMockConfig{}
	nangoSrv := httptest.NewServer(newNangoConnMock(mockCfg))
	t.Cleanup(nangoSrv.Close)
	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	nangoClient.FetchProviders(context.Background())

	h := handler.NewInConnectionHandler(db, nangoClient, catalog.Global())
	r := chi.NewRouter()
	r.Post("/v1/in/integrations/{id}/connect-session", h.CreateConnectSession)

	user := createTestUser(t, db, fmt.Sprintf("conn-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, db)
	integ := createTestInIntegration(t, db, "github")

	req := httptest.NewRequest(http.MethodPost, "/v1/in/integrations/"+integ.ID.String()+"/connect-session", nil)
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithUser(req, &user)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["token"] == nil || resp["token"] == "" {
		t.Fatal("expected token in response")
	}
	pck, ok := resp["provider_config_key"].(string)
	if !ok || !strings.HasPrefix(pck, "in_") {
		t.Fatalf("expected provider_config_key with in_ prefix, got %v", resp["provider_config_key"])
	}

	// Verify Nango received correct end_user.id
	mockCfg.mu.Lock()
	found := false
	for i, p := range mockCfg.capturedPaths {
		if p == "/connect/sessions" && mockCfg.capturedMethods[i] == http.MethodPost {
			var reqBody map[string]any
			json.Unmarshal(mockCfg.capturedBodies[i], &reqBody)
			if endUser, ok := reqBody["end_user"].(map[string]any); ok {
				if endUser["id"] == user.ID.String() {
					found = true
				}
			}
		}
	}
	mockCfg.mu.Unlock()
	if !found {
		t.Fatal("expected Nango connect session to have end_user.id = user.ID")
	}
}

func TestInConnectionHandler_CreateConnectSession_IntegrationNotFound(t *testing.T) {
	db := connectTestDB(t)

	nangoSrv := httptest.NewServer(newNangoConnMock(&nangoConnMockConfig{}))
	t.Cleanup(nangoSrv.Close)
	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	nangoClient.FetchProviders(context.Background())

	h := handler.NewInConnectionHandler(db, nangoClient, catalog.Global())
	r := chi.NewRouter()
	r.Post("/v1/in/integrations/{id}/connect-session", h.CreateConnectSession)

	user := createTestUser(t, db, fmt.Sprintf("conn-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, db)

	req := httptest.NewRequest(http.MethodPost, "/v1/in/integrations/"+uuid.New().String()+"/connect-session", nil)
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithUser(req, &user)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestInConnectionHandler_CreateConnectSession_NangoFailure(t *testing.T) {
	db := connectTestDB(t)
	t.Cleanup(func() {
		db.Where("1=1").Delete(&model.InConnection{})
		db.Where("1=1").Delete(&model.InIntegration{})
	})

	mockCfg := &nangoConnMockConfig{connectStatus: http.StatusInternalServerError}
	nangoSrv := httptest.NewServer(newNangoConnMock(mockCfg))
	t.Cleanup(nangoSrv.Close)
	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	nangoClient.FetchProviders(context.Background())

	h := handler.NewInConnectionHandler(db, nangoClient, catalog.Global())
	r := chi.NewRouter()
	r.Post("/v1/in/integrations/{id}/connect-session", h.CreateConnectSession)

	user := createTestUser(t, db, fmt.Sprintf("conn-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, db)
	integ := createTestInIntegration(t, db, "github")

	req := httptest.NewRequest(http.MethodPost, "/v1/in/integrations/"+integ.ID.String()+"/connect-session", nil)
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithUser(req, &user)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Missing user context test
// ---------------------------------------------------------------------------

func TestInConnectionHandler_MissingUserContext(t *testing.T) {
	db := connectTestDB(t)
	t.Cleanup(func() {
		db.Where("1=1").Delete(&model.InConnection{})
		db.Where("1=1").Delete(&model.InIntegration{})
	})

	nangoSrv := httptest.NewServer(newNangoConnMock(&nangoConnMockConfig{}))
	t.Cleanup(nangoSrv.Close)
	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	nangoClient.FetchProviders(context.Background())

	h := handler.NewInConnectionHandler(db, nangoClient, catalog.Global())
	r := chi.NewRouter()
	r.Get("/v1/in/connections", h.List)
	r.Get("/v1/in/connections/{id}", h.Get)
	r.Delete("/v1/in/connections/{id}", h.Revoke)
	r.Post("/v1/in/integrations/{id}/connect-session", h.CreateConnectSession)
	r.Post("/v1/in/integrations/{id}/connections", h.Create)

	endpoints := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/v1/in/connections"},
		{http.MethodGet, "/v1/in/connections/" + uuid.New().String()},
		{http.MethodDelete, "/v1/in/connections/" + uuid.New().String()},
		{http.MethodPost, "/v1/in/integrations/" + uuid.New().String() + "/connect-session"},
		{http.MethodPost, "/v1/in/integrations/" + uuid.New().String() + "/connections"},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			var bodyReader io.Reader
			if ep.method == http.MethodPost {
				bodyReader = bytes.NewReader([]byte(`{}`))
			}
			req := httptest.NewRequest(ep.method, ep.path, bodyReader)
			req.Header.Set("Content-Type", "application/json")
			// No user context set
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			if rr.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401 for %s %s without user context, got %d", ep.method, ep.path, rr.Code)
			}
		})
	}
}
