package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/cache"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

type apiKeyTestHarness struct {
	db      *gorm.DB
	cache   *cache.APIKeyCache
	manager *cache.Manager
	handler *handler.APIKeyHandler
	router  *chi.Mux
}

func newAPIKeyHarness(t *testing.T) *apiKeyTestHarness {
	t.Helper()

	db := connectTestDB(t)
	rc := connectTestRedis(t)
	keyCache := cache.NewAPIKeyCache(100, 5*time.Minute)
	cm := cache.Build(cache.Config{
		MemMaxSize: 100,
		MemTTL:     5 * time.Minute,
		RedisTTL:   10 * time.Minute,
		DEKMaxSize: 100,
		DEKTTL:     10 * time.Minute,
		HardExpiry: 15 * time.Minute,
	}, rc, nil, db, keyCache)

	h := handler.NewAPIKeyHandler(db, keyCache, cm)

	r := chi.NewRouter()
	r.Route("/v1/api-keys", func(r chi.Router) {
		r.Post("/", h.Create)
		r.Get("/", h.List)
		r.Delete("/{id}", h.Revoke)
	})

	return &apiKeyTestHarness{
		db:      db,
		cache:   keyCache,
		manager: cm,
		handler: h,
		router:  r,
	}
}

func (h *apiKeyTestHarness) doRequest(t *testing.T, method, path string, body any, org *model.Org) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if org != nil {
		req = middleware.WithOrg(req, org)
	}
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}
