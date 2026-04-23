package handler_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/mcp/catalog"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/nango"
)

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

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 (graceful degradation), got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["id"] != connID.String() {
		t.Fatalf("expected connection id, got %v", resp["id"])
	}
}
