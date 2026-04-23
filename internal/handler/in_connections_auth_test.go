package handler_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/mcp/catalog"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/nango"
)

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
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			if rr.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401 for %s %s without user context, got %d", ep.method, ep.path, rr.Code)
			}
		})
	}
}
