package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/cache"
	"github.com/usehivy/hivy/internal/counter"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/mcp/catalog"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/nango"
	"github.com/usehivy/hivy/internal/proxy"
	"github.com/usehivy/hivy/internal/registry"
	"github.com/usehivy/hivy/internal/testdb"
)

const (
	testDBURL      = "postgres://hivy:localdev@localhost:15432/hivy_test?sslmode=disable" //nolint:gosec // G101: local-only test fixture, not a real credential
	testRedisAddr  = "localhost:16379"
	testSigningKey = "e2e-signing-key-for-tests"
)

type testHarness struct {
	db           *gorm.DB
	kms          *crypto.KeyWrapper
	redisClient  *redis.Client
	cacheManager *cache.Manager
	auditWriter  *middleware.AuditWriter
	router       *chi.Mux
	signingKey   []byte
	nangoClient  *nango.Client
	catalog      *catalog.Catalog
}

func loadEnv(t *testing.T) {
	t.Helper()

	data, err := os.ReadFile("../.env")
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 && os.Getenv(parts[0]) == "" {
			os.Setenv(parts[0], parts[1])
		}
	}
}

func newHarness(t *testing.T) *testHarness {
	t.Helper()
	loadEnv(t)

	proxy.AllowLoopback = true

	dsn := envOr("HIVY_DATABASE_URL", testDBURL)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("cannot connect to Postgres: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(3)
	sqlDB.SetMaxIdleConns(1)
	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("Postgres not reachable: %v", err)
	}
	testdb.ApplyMigrations(t, db)
	t.Cleanup(func() { sqlDB.Close() })

	rc := redis.NewClient(&redis.Options{Addr: envOr("HIVY_REDIS_ADDR", testRedisAddr)})
	if err := rc.Ping(context.Background()).Err(); err != nil {
		t.Fatalf("Redis not reachable: %v", err)
	}
	t.Cleanup(func() { rc.Close() })

	kms, err := crypto.NewAEADWrapper(t.Context(), "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=", "e2e-test-key")
	if err != nil {
		t.Fatalf("cannot create AEAD wrapper: %v", err)
	}

	cfg := cache.Config{
		MemMaxSize: 1000,
		MemTTL:     5 * time.Minute,
		RedisTTL:   10 * time.Minute,
		DEKMaxSize: 100,
		DEKTTL:     10 * time.Minute,
		HardExpiry: 15 * time.Minute,
	}
	cm := cache.Build(cfg, rc, kms, db, nil)

	signingKey := []byte(testSigningKey)

	aw := middleware.NewAuditWriter(t.Context(), db, 1000, 10*time.Millisecond)

	r := chi.NewRouter()

	ctr := counter.New(rc, db)

	actionsCatalog := catalog.Global()

	credHandler := handler.NewCredentialHandler(db, kms, cm, ctr)
	tokenHandler := handler.NewTokenHandler(db, signingKey, cm, ctr, actionsCatalog, "", nil)

	reg := registry.Global()
	providerHandler := handler.NewProviderHandler(reg, db)

	nangoMockServer := newNangoMock(t)
	nangoClient := nango.NewClient(nangoMockServer.URL(), "mock-secret-key")
	if err := nangoClient.FetchProviders(context.Background()); err != nil {
		t.Fatalf("failed to fetch Nango providers: %v", err)
	}

	t.Logf("Nango provider cache loaded: %d providers", len(nangoClient.GetProviders()))

	r.Route("/v1", func(r chi.Router) {
		r.Post("/credentials", credHandler.Create)
		r.Get("/credentials", credHandler.List)
		r.Delete("/credentials/{id}", credHandler.Revoke)
		r.Post("/tokens", tokenHandler.Mint)
		r.Delete("/tokens/{jti}", tokenHandler.Revoke)
		r.Get("/providers", providerHandler.List)
		r.Get("/providers/{id}", providerHandler.Get)
		r.Get("/providers/{id}/models", providerHandler.Models)
	})

	r.Route("/v1/widget", func(r chi.Router) {
		r.Route("/integrations", func(r chi.Router) {
		})
	})

	proxyHandler := handler.NewProxyHandler(cm, proxy.NewTransport())
	r.Route("/v1/proxy", func(r chi.Router) {
		r.Use(middleware.TokenAuth(signingKey, db))
		r.Use(middleware.RemainingCheck(ctr))
		r.Use(middleware.Audit(aw, "proxy.request"))
		r.Handle("/*", proxyHandler)
	})

	t.Cleanup(func() {
		aw.Shutdown(context.Background())
	})

	return &testHarness{
		db:           db,
		kms:          kms,
		redisClient:  rc,
		cacheManager: cm,
		auditWriter:  aw,
		router:       r,
		signingKey:   signingKey,
		nangoClient:  nangoClient,
		catalog:      actionsCatalog,
	}
}

func (h *testHarness) createOrg(t *testing.T) model.Org {
	t.Helper()
	org := model.Org{
		ID:        uuid.New(),
		Name:      fmt.Sprintf("e2e-org-%s", uuid.New().String()[:8]),
		RateLimit: 10000,
		Active:    true,
	}
	if err := h.db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	t.Cleanup(func() {
		h.db.Where("org_id = ?", org.ID).Delete(&model.AuditEntry{})
		h.db.Where("org_id = ?", org.ID).Delete(&model.Token{})
		h.db.Where("org_id = ?", org.ID).Delete(&model.Credential{})
		h.db.Where("id = ?", org.ID).Delete(&model.Org{})
	})
	return org
}

func (h *testHarness) storeCredential(t *testing.T, org model.Org, baseURL, authScheme, apiKey string) model.Credential {
	t.Helper()

	body := fmt.Sprintf(`{"label":"e2e-test","provider_id":"openrouter","base_url":%q,"auth_scheme":%q,"api_key":%q}`,
		baseURL, authScheme, apiKey)

	req := httptest.NewRequest(http.MethodPost, "/v1/credentials", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("store credential: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&resp)

	var cred model.Credential
	h.db.Where("id = ?", resp.ID).First(&cred)
	return cred
}

func (h *testHarness) mintToken(t *testing.T, org model.Org, credID uuid.UUID) string {
	t.Helper()

	body := fmt.Sprintf(`{"credential_id":%q,"ttl":"1h"}`, credID.String())
	req := httptest.NewRequest(http.MethodPost, "/v1/tokens", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("mint token: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Token string `json:"token"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	return resp.Token
}

func (h *testHarness) proxyRequest(t *testing.T, method, path string, tok string, body io.Reader) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func decodePaginatedList(t *testing.T, rr *httptest.ResponseRecorder) []map[string]any {
	t.Helper()
	var resp struct {
		Data    []map[string]any `json:"data"`
		HasMore bool             `json:"has_more"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode paginated response: %v", err)
	}
	return resp.Data
}
