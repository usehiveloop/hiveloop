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

	"github.com/usehiveloop/hiveloop/internal/cache"
	"github.com/usehiveloop/hiveloop/internal/counter"
	"github.com/usehiveloop/hiveloop/internal/crypto"
	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/mcp/catalog"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/nango"
	"github.com/usehiveloop/hiveloop/internal/proxy"
	"github.com/usehiveloop/hiveloop/internal/registry"
	"github.com/usehiveloop/hiveloop/internal/token"
)

const (
	testDBURL      = "postgres://hiveloop:localdev@localhost:5433/hiveloop_test?sslmode=disable" //nolint:gosec // G101: local-only test fixture, not a real credential
	testRedisAddr  = "localhost:6379"
	testSigningKey = "e2e-signing-key-for-tests"
)

// testHarness bundles all infrastructure needed for E2E tests.
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

	dsn := envOr("DATABASE_URL", testDBURL)
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
	if err := model.AutoMigrate(db); err != nil {
		t.Fatalf("migration failed: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	rc := redis.NewClient(&redis.Options{Addr: envOr("REDIS_ADDR", testRedisAddr)})
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

// createOrg creates a test org in Postgres.
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

// storeCredential encrypts and stores an API key as a credential.
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

// mintToken creates a sandbox proxy token for a credential.
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

// proxyRequest sends a request through the reverse proxy using a sandbox token.
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

func TestE2E_CredentialLifecycle(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)

	cred := h.storeCredential(t, org, "https://api.example.com", "bearer", "sk-fake-key-12345")
	if cred.ID == uuid.Nil {
		t.Fatal("credential not created")
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/credentials", nil)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", rr.Code)
	}
	creds := decodePaginatedList(t, rr)
	found := false
	for _, c := range creds {
		if c["id"] == cred.ID.String() {
			found = true
		}
	}
	if !found {
		t.Fatal("created credential not in list")
	}

	req = httptest.NewRequest(http.MethodDelete, "/v1/credentials/"+cred.ID.String(), nil)
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("revoke: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	body := fmt.Sprintf(`{"credential_id":%q,"ttl":"1h"}`, cred.ID.String())
	req = httptest.NewRequest(http.MethodPost, "/v1/tokens", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("mint after revoke: expected 404, got %d", rr.Code)
	}
}

func TestE2E_TokenLifecycle(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)
	cred := h.storeCredential(t, org, "https://api.example.com", "bearer", "sk-fake-key-12345")

	tok := h.mintToken(t, org, cred.ID)
	if !strings.HasPrefix(tok, "ptok_") {
		t.Fatalf("expected ptok_ prefix, got %s", tok[:10])
	}

	jwtStr := strings.TrimPrefix(tok, "ptok_")
	claims, err := token.Validate(h.signingKey, jwtStr)
	if err != nil {
		t.Fatalf("validate minted token: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/v1/tokens/"+claims.ID, nil)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("revoke token: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	proxyPath := "/v1/proxy/v1/chat/completions"
	rr = h.proxyRequest(t, http.MethodPost, proxyPath, tok, strings.NewReader(`{}`))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("proxy with revoked token: expected 401, got %d", rr.Code)
	}
}

// decodePaginatedList decodes a paginated list response and returns the data array.
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
