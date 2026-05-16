package handler_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/mcp/catalog"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/nango"
)

func TestInConnectionHandler_List_ExcludesProfileProviders(t *testing.T) {
	db := connectTestDB(t)
	t.Cleanup(func() {
		db.Where("1=1").Delete(&model.InConnection{})
		db.Where("1=1").Delete(&model.InIntegration{})
	})

	nangoSrv := httptest.NewServer(newNangoConnMock(&nangoConnMockConfig{}))
	t.Cleanup(nangoSrv.Close)
	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	_ = nangoClient.FetchProviders(context.Background())

	h := handler.NewInConnectionHandler(db, nangoClient, catalog.Global())
	r := chi.NewRouter()
	r.Get("/v1/in/connections", h.List)

	user := createTestUser(t, db, fmt.Sprintf("conn-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, db)
	profileInteg := createTestInIntegration(t, db, "linear-profile")
	regularInteg := createTestInIntegration(t, db, "notion")
	profileConnID := uuid.New()
	regularConnID := uuid.New()
	db.Create(&model.InConnection{
		ID: profileConnID, OrgID: org.ID, UserID: user.ID, InIntegrationID: profileInteg.ID, NangoConnectionID: "profile-conn",
	})
	db.Create(&model.InConnection{
		ID: regularConnID, OrgID: org.ID, UserID: user.ID, InIntegrationID: regularInteg.ID, NangoConnectionID: "regular-conn",
	})

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
	_ = json.NewDecoder(rr.Body).Decode(&page)
	for _, item := range page.Data {
		if item["id"] == profileConnID.String() {
			t.Fatal("profile connection should not appear in global connection list")
		}
	}
	if len(page.Data) != 1 || page.Data[0]["id"] != regularConnID.String() {
		t.Fatalf("expected only regular connection %s, got %#v", regularConnID, page.Data)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/in/connections?provider=linear-profile", nil)
	req = middleware.WithUser(req, &user)
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 filtering by profile provider, got %d: %s", rr.Code, rr.Body.String())
	}
}
