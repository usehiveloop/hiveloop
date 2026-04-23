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
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/mcp/catalog"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/nango"
)

func createTestUser(t *testing.T, db *gorm.DB, email string) model.User {
	t.Helper()
	user := model.User{
		ID:           uuid.New(),
		Email:        email,
		PasswordHash: "$2a$10$dummy",
		Name:         "Test User",
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	t.Cleanup(func() {
		db.Where("id = ?", user.ID).Delete(&model.User{})
	})
	return user
}

func createTestInIntegration(t *testing.T, db *gorm.DB, provider string) model.InIntegration {
	t.Helper()
	integ := model.InIntegration{
		ID:          uuid.New(),
		UniqueKey:   fmt.Sprintf("%s-%s", provider, uuid.New().String()[:8]),
		Provider:    provider,
		DisplayName: provider + " built-in",
	}
	if err := db.Create(&integ).Error; err != nil {
		t.Fatalf("create in-integration: %v", err)
	}
	t.Cleanup(func() {
		db.Where("id = ?", integ.ID).Delete(&model.InIntegration{})
	})
	return integ
}

func cleanupInIntegrations(t *testing.T, db *gorm.DB) {
	t.Helper()
	t.Cleanup(func() {
		db.Where("1=1").Delete(&model.InConnection{})
		db.Where("1=1").Delete(&model.InIntegration{})
	})
}

type nangoMockConfig struct {
	mu              sync.Mutex
	capturedPaths   []string
	capturedMethods []string
	createStatus    int
	getStatus       int
	updateStatus    int
	deleteStatus    int
}

func newNangoMock(cfg *nangoMockConfig) http.Handler {
	if cfg.createStatus == 0 {
		cfg.createStatus = http.StatusOK
	}
	if cfg.getStatus == 0 {
		cfg.getStatus = http.StatusOK
	}
	if cfg.updateStatus == 0 {
		cfg.updateStatus = http.StatusOK
	}
	if cfg.deleteStatus == 0 {
		cfg.deleteStatus = http.StatusOK
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.mu.Lock()
		cfg.capturedPaths = append(cfg.capturedPaths, r.URL.Path)
		cfg.capturedMethods = append(cfg.capturedMethods, r.Method)
		cfg.mu.Unlock()

		if r.URL.Path == "/providers" && r.Method == http.MethodGet {
			json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{"name": "github", "display_name": "GitHub", "auth_mode": "OAUTH2"},
					{"name": "slack", "display_name": "Slack", "auth_mode": "OAUTH2"},
				},
			})
			return
		}

		if strings.HasPrefix(r.URL.Path, "/integrations") {
			switch r.Method {
			case http.MethodPost:
				w.WriteHeader(cfg.createStatus)
				if cfg.createStatus >= 400 {
					json.NewEncoder(w).Encode(map[string]any{"error": "nango error"})
					return
				}
				json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"unique_key": "test"}})
			case http.MethodGet:
				w.WriteHeader(cfg.getStatus)
				json.NewEncoder(w).Encode(map[string]any{
					"data": map[string]any{
						"unique_key": "test",
						"logo":       "https://example.com/logo.png",
					},
				})
			case http.MethodPatch:
				w.WriteHeader(cfg.updateStatus)
				if cfg.updateStatus >= 400 {
					json.NewEncoder(w).Encode(map[string]any{"error": "nango error"})
					return
				}
				json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"unique_key": "test"}})
			case http.MethodDelete:
				w.WriteHeader(cfg.deleteStatus)
				if cfg.deleteStatus >= 400 {
					json.NewEncoder(w).Encode(map[string]any{"error": "nango error"})
					return
				}
				json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
			}
			return
		}

		w.WriteHeader(http.StatusNotFound)
	})
}

type inIntegTestHarness struct {
	db      *gorm.DB
	handler *handler.InIntegrationHandler
	router  *chi.Mux
	nango   *httptest.Server
	mockCfg *nangoMockConfig
}

func newInIntegHarness(t *testing.T, mockCfg *nangoMockConfig) *inIntegTestHarness {
	t.Helper()
	db := connectTestDB(t)
	cleanupInIntegrations(t, db)

	if mockCfg == nil {
		mockCfg = &nangoMockConfig{}
	}
	nangoSrv := httptest.NewServer(newNangoMock(mockCfg))
	t.Cleanup(nangoSrv.Close)

	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	if err := nangoClient.FetchProviders(context.Background()); err != nil {
		t.Fatalf("fetch nango providers: %v", err)
	}

	cat := catalog.Global()
	h := handler.NewInIntegrationHandler(db, nangoClient, cat)

	r := chi.NewRouter()
	r.Post("/v1/in/integrations", h.Create)
	r.Get("/v1/in/integrations", h.List)
	r.Get("/v1/in/integrations/{id}", h.Get)
	r.Put("/v1/in/integrations/{id}", h.Update)
	r.Delete("/v1/in/integrations/{id}", h.Delete)
	r.Get("/v1/in/integrations/available", h.ListAvailable)

	return &inIntegTestHarness{
		db:      db,
		handler: h,
		router:  r,
		nango:   nangoSrv,
		mockCfg: mockCfg,
	}
}

func (h *inIntegTestHarness) doRequest(t *testing.T, method, path string, body any, user *model.User) *httptest.ResponseRecorder {
	t.Helper()
	var bodyReader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("encode body: %v", err)
		}
		bodyReader = bytes.NewReader(buf)
	}
	req := httptest.NewRequest(method, path, bodyReader)
	req.Header.Set("Content-Type", "application/json")
	if user != nil {
		req = middleware.WithUser(req, user)
	}
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}
