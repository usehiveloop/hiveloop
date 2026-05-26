package handler_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/mcp/catalog"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/nango"
)

func TestConnectionHandler_List_Success(t *testing.T) {
	db := connectTestDB(t)
	t.Cleanup(func() {
		db.Where("1=1").Delete(&model.Connection{})
		db.Where("1=1").Delete(&model.Integration{})
	})

	nangoSrv := httptest.NewServer(newNangoConnMock(&nangoConnMockConfig{}))
	t.Cleanup(nangoSrv.Close)
	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	_ = nangoClient.FetchProviders(context.Background())

	h := handler.NewConnectionHandler(db, nangoClient, catalog.Global(), nil)
	r := chi.NewRouter()
	r.Get("/v1/connections", h.List)

	user := createTestUser(t, db, fmt.Sprintf("conn-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, db)
	integ1 := createTestIntegration(t, db, "notion")
	integ2 := model.Integration{
		ID: uuid.New(), UniqueKey: fmt.Sprintf("linear-%s", uuid.New().String()[:8]),
		Provider: "linear", DisplayName: "Linear built-in",
	}
	db.Create(&integ2)

	for i, integ := range []model.Integration{integ1, integ2} {
		db.Create(&model.Connection{
			ID: uuid.New(), OrgID: org.ID, UserID: user.ID, IntegrationID: integ.ID,
			NangoConnectionID: fmt.Sprintf("conn-%d", i),
		})
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/connections", nil)
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
	_ = json.NewDecoder(rr.Body).Decode(&page)
	if len(page.Data) != 2 {
		t.Fatalf("expected 2 connections, got %d", len(page.Data))
	}
}

func TestConnectionHandler_List_UserIsolation(t *testing.T) {
	db := connectTestDB(t)
	t.Cleanup(func() {
		db.Where("1=1").Delete(&model.Connection{})
		db.Where("1=1").Delete(&model.Integration{})
	})

	nangoSrv := httptest.NewServer(newNangoConnMock(&nangoConnMockConfig{}))
	t.Cleanup(nangoSrv.Close)
	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	_ = nangoClient.FetchProviders(context.Background())

	h := handler.NewConnectionHandler(db, nangoClient, catalog.Global(), nil)
	r := chi.NewRouter()
	r.Get("/v1/connections", h.List)

	user1 := createTestUser(t, db, fmt.Sprintf("user1-%s@test.com", uuid.New().String()[:8]))
	user2 := createTestUser(t, db, fmt.Sprintf("user2-%s@test.com", uuid.New().String()[:8]))
	org1 := createTestOrg(t, db)
	org2 := createTestOrg(t, db)
	integ := createTestIntegration(t, db, "notion")

	db.Create(&model.Connection{
		ID: uuid.New(), OrgID: org1.ID, UserID: user1.ID, IntegrationID: integ.ID, NangoConnectionID: "user1-conn",
	})

	integ2 := model.Integration{
		ID: uuid.New(), UniqueKey: fmt.Sprintf("linear-%s", uuid.New().String()[:8]),
		Provider: "linear", DisplayName: "Linear built-in",
	}
	db.Create(&integ2)
	db.Create(&model.Connection{
		ID: uuid.New(), OrgID: org2.ID, UserID: user2.ID, IntegrationID: integ2.ID, NangoConnectionID: "user2-conn",
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/connections", nil)
	req = middleware.WithUser(req, &user2)
	req = middleware.WithOrg(req, &org2)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	var page struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&page)
	for _, item := range page.Data {
		if item["nango_connection_id"] == "user1-conn" {
			t.Fatal("user2 should not see user1's connection")
		}
	}
}

func TestConnectionHandler_List_ExcludesRevoked(t *testing.T) {
	db := connectTestDB(t)
	t.Cleanup(func() {
		db.Where("1=1").Delete(&model.Connection{})
		db.Where("1=1").Delete(&model.Integration{})
	})

	nangoSrv := httptest.NewServer(newNangoConnMock(&nangoConnMockConfig{}))
	t.Cleanup(nangoSrv.Close)
	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	_ = nangoClient.FetchProviders(context.Background())

	h := handler.NewConnectionHandler(db, nangoClient, catalog.Global(), nil)
	r := chi.NewRouter()
	r.Get("/v1/connections", h.List)

	user := createTestUser(t, db, fmt.Sprintf("conn-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, db)
	integ := createTestIntegration(t, db, "notion")

	now := time.Now()
	connID := uuid.New()
	db.Create(&model.Connection{
		ID: connID, OrgID: org.ID, UserID: user.ID, IntegrationID: integ.ID,
		NangoConnectionID: "revoked-conn", RevokedAt: &now,
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/connections", nil)
	req = middleware.WithUser(req, &user)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	var page struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&page)
	for _, item := range page.Data {
		if item["id"] == connID.String() {
			t.Fatal("revoked connection should not appear in list")
		}
	}
}

func TestConnectionHandler_List_Pagination(t *testing.T) {
	db := connectTestDB(t)
	t.Cleanup(func() {
		db.Where("1=1").Delete(&model.Connection{})
		db.Where("1=1").Delete(&model.Integration{})
	})

	nangoSrv := httptest.NewServer(newNangoConnMock(&nangoConnMockConfig{}))
	t.Cleanup(nangoSrv.Close)
	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	_ = nangoClient.FetchProviders(context.Background())

	h := handler.NewConnectionHandler(db, nangoClient, catalog.Global(), nil)
	r := chi.NewRouter()
	r.Get("/v1/connections", h.List)

	user := createTestUser(t, db, fmt.Sprintf("conn-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, db)

	for i := 0; i < 5; i++ {
		provider := fmt.Sprintf("provider-pg-%d-%s", i, uuid.New().String()[:8])
		integ := model.Integration{
			ID: uuid.New(), UniqueKey: fmt.Sprintf("%s-%s", provider, uuid.New().String()[:8]),
			Provider: provider, DisplayName: provider,
		}
		db.Create(&integ)
		db.Create(&model.Connection{
			ID: uuid.New(), OrgID: org.ID, UserID: user.ID, IntegrationID: integ.ID,
			NangoConnectionID: fmt.Sprintf("pg-conn-%d", i),
		})
		time.Sleep(time.Millisecond)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/connections?limit=2", nil)
	req = middleware.WithUser(req, &user)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	var page1 struct {
		Data       []map[string]any `json:"data"`
		HasMore    bool             `json:"has_more"`
		NextCursor *string          `json:"next_cursor"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&page1)
	if len(page1.Data) != 2 {
		t.Fatalf("expected 2 items, got %d", len(page1.Data))
	}
	if !page1.HasMore {
		t.Fatal("expected has_more=true")
	}
}

func TestConnectionHandler_List_FilterByProvider(t *testing.T) {
	db := connectTestDB(t)
	t.Cleanup(func() {
		db.Where("1=1").Delete(&model.Connection{})
		db.Where("1=1").Delete(&model.Integration{})
	})

	nangoSrv := httptest.NewServer(newNangoConnMock(&nangoConnMockConfig{}))
	t.Cleanup(nangoSrv.Close)
	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	_ = nangoClient.FetchProviders(context.Background())

	h := handler.NewConnectionHandler(db, nangoClient, catalog.Global(), nil)
	r := chi.NewRouter()
	r.Get("/v1/connections", h.List)

	user := createTestUser(t, db, fmt.Sprintf("conn-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, db)
	ghInteg := createTestIntegration(t, db, "notion")
	slackInteg := model.Integration{
		ID: uuid.New(), UniqueKey: fmt.Sprintf("linear-%s", uuid.New().String()[:8]),
		Provider: "linear", DisplayName: "Linear built-in",
	}
	db.Create(&slackInteg)

	db.Create(&model.Connection{
		ID: uuid.New(), OrgID: org.ID, UserID: user.ID, IntegrationID: ghInteg.ID, NangoConnectionID: "notion-conn",
	})
	db.Create(&model.Connection{
		ID: uuid.New(), OrgID: org.ID, UserID: user.ID, IntegrationID: slackInteg.ID, NangoConnectionID: "linear-conn",
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/connections?provider=notion", nil)
	req = middleware.WithUser(req, &user)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	var page struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&page)
	if len(page.Data) != 1 {
		t.Fatalf("expected 1 notion connection, got %d", len(page.Data))
	}
	if page.Data[0]["provider"] != "notion" {
		t.Fatalf("expected provider=notion, got %v", page.Data[0]["provider"])
	}
}
