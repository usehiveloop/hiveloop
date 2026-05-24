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

	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/mcp/catalog"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/nango"
)

func TestConnectionHandler_CreateConnectSession_Success(t *testing.T) {
	db := connectTestDB(t)
	t.Cleanup(func() {
		db.Where("1=1").Delete(&model.Connection{})
		db.Where("1=1").Delete(&model.Integration{})
	})

	mockCfg := &nangoConnMockConfig{}
	nangoSrv := httptest.NewServer(newNangoConnMock(mockCfg))
	t.Cleanup(nangoSrv.Close)
	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	_ = nangoClient.FetchProviders(context.Background())

	h := handler.NewConnectionHandler(db, nangoClient, catalog.Global())
	r := chi.NewRouter()
	r.Post("/v1/integrations/{id}/connect-session", h.CreateConnectSession)

	user := createTestUser(t, db, fmt.Sprintf("conn-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, db)
	integ := createTestIntegration(t, db, "notion")

	req := httptest.NewRequest(http.MethodPost, "/v1/integrations/"+integ.ID.String()+"/connect-session", nil)
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithUser(req, &user)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	if resp["token"] == nil || resp["token"] == "" {
		t.Fatal("expected token in response")
	}
	pck, ok := resp["provider_config_key"].(string)
	if !ok || pck != integ.UniqueKey {
		t.Fatalf("expected provider_config_key %q, got %v", integ.UniqueKey, resp["provider_config_key"])
	}

	mockCfg.mu.Lock()
	found := false
	for i, p := range mockCfg.capturedPaths {
		if p == "/connect/sessions" && mockCfg.capturedMethods[i] == http.MethodPost {
			var reqBody map[string]any
			_ = json.Unmarshal(mockCfg.capturedBodies[i], &reqBody)
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

func TestConnectionHandler_CreateConnectSession_IntegrationNotFound(t *testing.T) {
	db := connectTestDB(t)

	nangoSrv := httptest.NewServer(newNangoConnMock(&nangoConnMockConfig{}))
	t.Cleanup(nangoSrv.Close)
	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	_ = nangoClient.FetchProviders(context.Background())

	h := handler.NewConnectionHandler(db, nangoClient, catalog.Global())
	r := chi.NewRouter()
	r.Post("/v1/integrations/{id}/connect-session", h.CreateConnectSession)

	user := createTestUser(t, db, fmt.Sprintf("conn-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, db)

	req := httptest.NewRequest(http.MethodPost, "/v1/integrations/"+uuid.New().String()+"/connect-session", nil)
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithUser(req, &user)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestConnectionHandler_CreateConnectSession_NangoFailure(t *testing.T) {
	db := connectTestDB(t)
	t.Cleanup(func() {
		db.Where("1=1").Delete(&model.Connection{})
		db.Where("1=1").Delete(&model.Integration{})
	})

	mockCfg := &nangoConnMockConfig{connectStatus: http.StatusInternalServerError}
	nangoSrv := httptest.NewServer(newNangoConnMock(mockCfg))
	t.Cleanup(nangoSrv.Close)
	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	_ = nangoClient.FetchProviders(context.Background())

	h := handler.NewConnectionHandler(db, nangoClient, catalog.Global())
	r := chi.NewRouter()
	r.Post("/v1/integrations/{id}/connect-session", h.CreateConnectSession)

	user := createTestUser(t, db, fmt.Sprintf("conn-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, db)
	integ := createTestIntegration(t, db, "notion")

	req := httptest.NewRequest(http.MethodPost, "/v1/integrations/"+integ.ID.String()+"/connect-session", nil)
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithUser(req, &user)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d: %s", rr.Code, rr.Body.String())
	}
}
