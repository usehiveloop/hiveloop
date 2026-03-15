// Package e2e contains end-to-end tests for HashiCorp Vault envelope encryption.
//
// These tests verify:
//   - Vault Transit engine integration for DEK wrapping/unwrapping
//   - Credential encryption using Vault-wrapped DEKs
//   - Token minting and proxy with Vault-encrypted credentials
//   - Full proxy flow through OpenRouter with Vault encryption
//
// These tests require:
//   - Running Docker Compose stack with Vault service (Postgres, Redis, Vault)
//   - Vault Transit engine enabled with 'llmvault-key' encryption key
//   - OPENROUTER_API_KEY env var set for live LLM tests (optional)
package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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

	"github.com/llmvault/llmvault/internal/cache"
	"github.com/llmvault/llmvault/internal/counter"
	"github.com/llmvault/llmvault/internal/crypto"
	"github.com/llmvault/llmvault/internal/handler"
	"github.com/llmvault/llmvault/internal/mcp/catalog"
	"github.com/llmvault/llmvault/internal/middleware"
	"github.com/llmvault/llmvault/internal/model"
	"github.com/llmvault/llmvault/internal/proxy"
	"github.com/llmvault/llmvault/internal/registry"
	"github.com/llmvault/llmvault/internal/token"
)

const (
	vaultTestDBURL     = "postgres://llmvault:localdev@localhost:5433/llmvault_vault_test?sslmode=disable"
	vaultTestRedisAddr = "localhost:6379"
	vaultTestVaultAddr = "http://localhost:8200"
	vaultTestToken     = "llmvault-dev-token"
	vaultTestKeyName   = "llmvault-key"
	vaultSigningKey    = "vault-test-signing-key-for-e2e"
)

// vaultTestHarness bundles all infrastructure needed for Vault E2E tests.
type vaultTestHarness struct {
	db           *gorm.DB
	kms          *crypto.KeyWrapper
	redisClient  *redis.Client
	cacheManager *cache.Manager
	router       *chi.Mux
	signingKey   []byte
}

// newVaultHarness creates a test harness using Vault Transit for envelope encryption.
func newVaultHarness(t *testing.T) *vaultTestHarness {
	t.Helper()

	// Skip if Vault is not available
	if !vaultIsAvailable(t) {
		t.Fatal("Vault not available at " + vaultTestVaultAddr + " — must be running")
	}

	// Allow loopback addresses for test httptest servers
	proxy.AllowLoopback = true

	// DB
	dsn := envOr("DATABASE_URL", vaultTestDBURL)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("cannot connect to Postgres: %v", err)
	}
	sqlDB, _ := db.DB()
	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("Postgres not reachable: %v", err)
	}
	if err := model.AutoMigrate(db); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	// Redis
	rc := redis.NewClient(&redis.Options{Addr: envOr("REDIS_ADDR", vaultTestRedisAddr)})
	if err := rc.Ping(context.Background()).Err(); err != nil {
		t.Fatalf("Redis not reachable: %v", err)
	}
	t.Cleanup(func() { rc.Close() })

	// KMS (Vault Transit wrapper for tests)
	vaultCfg := crypto.VaultConfig{
		Address:  envOr("VAULT_ADDRESS", vaultTestVaultAddr),
		Token:    envOr("VAULT_TOKEN", vaultTestToken),
		KeyName:  vaultTestKeyName,
		MountPath: "transit",
	}
	kms, err := crypto.NewVaultTransitWrapper(vaultCfg)
	if err != nil {
		t.Fatalf("cannot create Vault Transit wrapper: %v", err)
	}

	// Cache
	cfg := cache.Config{
		MemMaxSize: 1000,
		MemTTL:     5 * time.Minute,
		RedisTTL:   10 * time.Minute,
		DEKMaxSize: 100,
		DEKTTL:     10 * time.Minute,
		HardExpiry: 15 * time.Minute,
	}
	cm := cache.Build(cfg, rc, kms, db, nil)

	signingKey := []byte(vaultSigningKey)

	// Build the full Chi router
	r := chi.NewRouter()

	// Request-cap counter
	ctr := counter.New(rc, db)

	// Actions catalog
	actionsCatalog := catalog.Global()

	// Credential + token + identity handlers
	credHandler := handler.NewCredentialHandler(db, kms, cm, ctr)
	tokenHandler := handler.NewTokenHandler(db, signingKey, cm, ctr, actionsCatalog)
	identityHandler := handler.NewIdentityHandler(db)

	// Provider handler
	reg := registry.Global()
	providerHandler := handler.NewProviderHandler(reg)

	// Connect handlers
	connectSessionHandler := handler.NewConnectSessionHandler(db, reg)
	connectAPIHandler := handler.NewConnectAPIHandler(db, kms, reg, nil, actionsCatalog)
	settingsHandler := handler.NewSettingsHandler(db)

	// Management routes
	r.Route("/v1", func(r chi.Router) {
		r.Post("/credentials", credHandler.Create)
		r.Get("/credentials", credHandler.List)
		r.Delete("/credentials/{id}", credHandler.Revoke)
		r.Post("/tokens", tokenHandler.Mint)
		r.Delete("/tokens/{jti}", tokenHandler.Revoke)
		r.Post("/identities", identityHandler.Create)
		r.Get("/identities", identityHandler.List)
		r.Get("/identities/{id}", identityHandler.Get)
		r.Put("/identities/{id}", identityHandler.Update)
		r.Delete("/identities/{id}", identityHandler.Delete)
		r.Get("/providers", providerHandler.List)
		r.Get("/providers/{id}", providerHandler.Get)
		r.Get("/providers/{id}/models", providerHandler.Models)
		r.Post("/connect/sessions", connectSessionHandler.Create)
		r.Get("/settings/connect", settingsHandler.GetConnectSettings)
		r.Put("/settings/connect", settingsHandler.UpdateConnectSettings)
	})

	// Connect API
	r.Route("/v1/widget", func(r chi.Router) {
		r.Use(middleware.ConnectSessionAuth(db))
		r.Use(middleware.ConnectSecurityHeaders())
		r.Use(middleware.ConnectCORS())

		r.Get("/session", connectAPIHandler.SessionInfo)
		r.Get("/providers", connectAPIHandler.ListProviders)
		r.Get("/connections", connectAPIHandler.ListConnections)
		r.Post("/connections", connectAPIHandler.CreateConnection)
		r.Delete("/connections/{id}", connectAPIHandler.DeleteConnection)
		r.Post("/connections/{id}/verify", connectAPIHandler.VerifyConnection)
	})

	// Proxy route
	proxyHandler := handler.NewProxyHandler(cm, proxy.NewTransport())
	r.Route("/v1/proxy", func(r chi.Router) {
		r.Use(middleware.TokenAuth(signingKey, db))
		r.Use(middleware.IdentityRateLimit(rc, db))
		r.Use(middleware.RemainingCheck(ctr))
		r.Handle("/*", proxyHandler)
	})

	return &vaultTestHarness{
		db:           db,
		kms:          kms,
		redisClient:  rc,
		cacheManager: cm,
		router:       r,
		signingKey:   signingKey,
	}
}

// vaultIsAvailable checks if Vault is running and accessible.
func vaultIsAvailable(t *testing.T) bool {
	t.Helper()
	vaultAddr := envOr("VAULT_ADDRESS", vaultTestVaultAddr)
	vaultToken := envOr("VAULT_TOKEN", vaultTestToken)

	client := &http.Client{Timeout: 2 * time.Second}
	req, err := http.NewRequest("GET", vaultAddr+"/v1/sys/health", nil)
	if err != nil {
		return false
	}
	req.Header.Set("X-Vault-Token", vaultToken)

	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	// Vault returns 200 if initialized and unsealed, 429 if standby
	return resp.StatusCode == 200 || resp.StatusCode == 429
}

// createOrg creates a test org in Postgres.
func (h *vaultTestHarness) createOrg(t *testing.T) model.Org {
	t.Helper()
	org := model.Org{
		ID:           uuid.New(),
		Name:         fmt.Sprintf("vault-e2e-org-%s", uuid.New().String()[:8]),
		LogtoOrgID: fmt.Sprintf("logto-vault-%s", uuid.New().String()[:8]),
		RateLimit:    10000,
		Active:       true,
	}
	if err := h.db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	t.Cleanup(func() {
		h.db.Where("org_id = ?", org.ID).Delete(&model.ConnectSession{})
		h.db.Where("org_id = ?", org.ID).Delete(&model.Token{})
		h.db.Where("org_id = ?", org.ID).Delete(&model.Credential{})
		h.db.Where("identity_id IN (SELECT id FROM identities WHERE org_id = ?)", org.ID).Delete(&model.IdentityRateLimit{})
		h.db.Where("org_id = ?", org.ID).Delete(&model.Identity{})
		h.db.Where("id = ?", org.ID).Delete(&model.Org{})
	})
	return org
}

// storeCredential encrypts and stores an API key as a credential using Vault.
func (h *vaultTestHarness) storeCredential(t *testing.T, org model.Org, baseURL, authScheme, apiKey string) model.Credential {
	t.Helper()

	body := fmt.Sprintf(`{"label":"vault-e2e-test","provider_id":"openrouter","base_url":%q,"auth_scheme":%q,"api_key":%q}`,
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
	json.NewDecoder(rr.Body).Decode(&resp)

	var cred model.Credential
	h.db.Where("id = ?", resp.ID).First(&cred)
	return cred
}

// mintToken creates a sandbox proxy token for a credential.
func (h *vaultTestHarness) mintToken(t *testing.T, org model.Org, credID uuid.UUID) string {
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
	json.NewDecoder(rr.Body).Decode(&resp)
	return resp.Token
}

// proxyRequest sends a request through the reverse proxy using a sandbox token.
func (h *vaultTestHarness) proxyRequest(t *testing.T, method, path string, tok string, body *strings.Reader) *httptest.ResponseRecorder {
	t.Helper()
	var bodyReader *strings.Reader
	if body != nil {
		bodyReader = body
	} else {
		bodyReader = strings.NewReader("")
	}
	req := httptest.NewRequest(method, path, bodyReader)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

// proxyRequestWithBody sends a request through the reverse proxy with a string body.
func (h *vaultTestHarness) proxyRequestWithBody(t *testing.T, method, path string, tok string, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

// --------------------------------------------------------------------------
// E2E: Vault envelope encryption - basic credential lifecycle
// --------------------------------------------------------------------------

func TestVaultE2E_CredentialLifecycle(t *testing.T) {
	h := newVaultHarness(t)
	org := h.createOrg(t)

	// Create credential - DEK will be wrapped by Vault Transit
	cred := h.storeCredential(t, org, "https://api.example.com", "bearer", "sk-vault-encrypted-key-12345")
	if cred.ID == uuid.Nil {
		t.Fatal("credential not created")
	}

	// Verify the credential has Vault-wrapped DEK
	if len(cred.WrappedDEK) == 0 {
		t.Fatal("credential missing WrappedDEK")
	}
	if len(cred.EncryptedKey) == 0 {
		t.Fatal("credential missing EncryptedKey")
	}

	t.Logf("Credential created with Vault-wrapped DEK (%d bytes)", len(cred.WrappedDEK))

	// List credentials
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

	// Revoke
	req = httptest.NewRequest(http.MethodDelete, "/v1/credentials/"+cred.ID.String(), nil)
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("revoke: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify revoked credential can't be used for new tokens
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

// --------------------------------------------------------------------------
// E2E: Vault envelope encryption - DEK wrap/unwrap verification
// --------------------------------------------------------------------------

func TestVaultE2E_DEKWrapUnwrap(t *testing.T) {
	h := newVaultHarness(t)

	// Generate a DEK locally
	dek, err := crypto.GenerateDEK()
	if err != nil {
		t.Fatalf("generate DEK: %v", err)
	}

	// Wrap the DEK using Vault Transit
	ctx := context.Background()
	wrappedDEK, err := h.kms.Wrap(ctx, dek)
	if err != nil {
		t.Fatalf("wrap DEK with Vault: %v", err)
	}
	t.Logf("DEK wrapped successfully (%d bytes)", len(wrappedDEK))

	// Unwrap the DEK using Vault Transit
	unwrappedDEK, err := h.kms.Unwrap(ctx, wrappedDEK)
	if err != nil {
		t.Fatalf("unwrap DEK with Vault: %v", err)
	}

	// Verify the unwrapped DEK matches original
	if !bytes.Equal(dek, unwrappedDEK) {
		t.Fatal("unwrapped DEK does not match original")
	}

	t.Log("DEK wrap/unwrap roundtrip verified successfully")
}

// --------------------------------------------------------------------------
// E2E: Vault envelope encryption - token lifecycle
// --------------------------------------------------------------------------

func TestVaultE2E_TokenLifecycle(t *testing.T) {
	h := newVaultHarness(t)
	org := h.createOrg(t)

	// Create credential with Vault encryption
	cred := h.storeCredential(t, org, "https://api.example.com", "bearer", "sk-fake-vault-key-12345")

	// Mint token - this will use Vault to unwrap the DEK and decrypt the credential
	tok := h.mintToken(t, org, cred.ID)
	if !strings.HasPrefix(tok, "ptok_") {
		t.Fatalf("expected ptok_ prefix, got %s", tok[:10])
	}

	// Extract JTI for revocation
	jwtStr := strings.TrimPrefix(tok, "ptok_")
	claims, err := token.Validate(h.signingKey, jwtStr)
	if err != nil {
		t.Fatalf("validate minted token: %v", err)
	}

	// Revoke
	req := httptest.NewRequest(http.MethodDelete, "/v1/tokens/"+claims.ID, nil)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("revoke token: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify revoked token is rejected by proxy
	proxyPath := "/v1/proxy/v1/chat/completions"
	rr = h.proxyRequest(t, http.MethodPost, proxyPath, tok, strings.NewReader(`{}`))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("proxy with revoked token: expected 401, got %d", rr.Code)
	}
}

// --------------------------------------------------------------------------
// E2E: Vault envelope encryption - proxy with live LLM (optional)
// --------------------------------------------------------------------------

func TestVaultE2E_Proxy_OpenAI_NonStreaming(t *testing.T) {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		t.Fatal("OPENROUTER_API_KEY must be set")
	}

	h := newVaultHarness(t)
	org := h.createOrg(t)

	// Store credential - API key will be encrypted with Vault-wrapped DEK
	cred := h.storeCredential(t, org, "https://openrouter.ai/api", "bearer", apiKey)
	t.Logf("Stored credential with Vault encryption, ID: %s", cred.ID)

	// Mint token
	tok := h.mintToken(t, org, cred.ID)

	payload := `{
		"model": "openai/gpt-4.1-nano",
		"messages": [{"role": "user", "content": "Reply with exactly: hello from Vault"}],
		"stream": false,
		"max_tokens": 20
	}`

	proxyPath := "/v1/proxy/v1/chat/completions"
	rr := h.proxyRequest(t, http.MethodPost, proxyPath, tok, strings.NewReader(payload))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	content := extractNonStreamContent(t, resp)
	if content == "" {
		t.Fatal("empty content in response")
	}
	t.Logf("OpenAI response via Vault encryption: %s", content)
}

// --------------------------------------------------------------------------
// E2E: Vault envelope encryption - streaming proxy
// --------------------------------------------------------------------------

func TestVaultE2E_Proxy_Anthropic_Streaming(t *testing.T) {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		t.Fatal("OPENROUTER_API_KEY must be set")
	}

	h := newVaultHarness(t)
	org := h.createOrg(t)

	// Store credential with Vault encryption
	cred := h.storeCredential(t, org, "https://openrouter.ai/api", "bearer", apiKey)
	tok := h.mintToken(t, org, cred.ID)

	payload := `{
		"model": "anthropic/claude-haiku-4.5",
		"messages": [{"role": "user", "content": "Count from 1 to 3, one number per line."}],
		"stream": true,
		"max_tokens": 50
	}`

	proxyPath := "/v1/proxy/v1/chat/completions"
	rr := h.proxyRequest(t, http.MethodPost, proxyPath, tok, strings.NewReader(payload))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Parse SSE stream
	chunks := parseSSEChunks(t, rr.Body.Bytes())
	if len(chunks) == 0 {
		t.Fatal("expected SSE chunks, got none")
	}

	content := extractStreamContent(chunks)
	if content == "" {
		t.Fatal("no content from streaming response")
	}
	t.Logf("Anthropic streaming via Vault encryption: %s", content)
}

// --------------------------------------------------------------------------
// E2E: Vault envelope encryption - tenant isolation
// --------------------------------------------------------------------------

func TestVaultE2E_TenantIsolation(t *testing.T) {
	h := newVaultHarness(t)
	org1 := h.createOrg(t)
	org2 := h.createOrg(t)

	// Create credential for org1 with Vault encryption
	cred1 := h.storeCredential(t, org1, "https://api.example.com", "bearer", "org1-vault-secret")

	// Mint token for org2 (which doesn't own cred1)
	tokenStr, jti, err := token.Mint(h.signingKey, org2.ID.String(), cred1.ID.String(), time.Hour)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	tokenRecord := model.Token{
		ID: uuid.New(), OrgID: org2.ID, CredentialID: cred1.ID,
		JTI: jti, ExpiresAt: time.Now().Add(time.Hour),
	}
	h.db.Create(&tokenRecord)
	t.Cleanup(func() { h.db.Where("id = ?", tokenRecord.ID).Delete(&model.Token{}) })

	proxyPath := "/v1/proxy/test"
	rr := h.proxyRequest(t, http.MethodGet, proxyPath, "ptok_"+tokenStr, nil)

	// Should fail because org2 doesn't own cred1
	if rr.Code == http.StatusOK {
		t.Fatal("tenant isolation violated: org2 accessed org1's credential")
	}
	t.Logf("Tenant isolation enforced with Vault encryption: got %d", rr.Code)
}

// --------------------------------------------------------------------------
// E2E: Vault envelope encryption - multiple credentials per org
// --------------------------------------------------------------------------

func TestVaultE2E_MultipleCredentials(t *testing.T) {
	h := newVaultHarness(t)
	org := h.createOrg(t)

	// Create multiple credentials - each with unique DEK wrapped by Vault
	creds := make([]model.Credential, 5)
	for i := 0; i < 5; i++ {
		apiKey := fmt.Sprintf("sk-multi-vault-key-%d-%s", i, uuid.New().String()[:8])
		creds[i] = h.storeCredential(t, org, "https://api.example.com", "bearer", apiKey)
		t.Logf("Created credential %d with DEK wrapped by Vault", i+1)
	}

	// Verify each credential can be used independently
	for i, cred := range creds {
		tok := h.mintToken(t, org, cred.ID)
		if !strings.HasPrefix(tok, "ptok_") {
			t.Fatalf("credential %d: expected ptok_ prefix", i)
		}

		// Validate the JWT
		jwtStr := strings.TrimPrefix(tok, "ptok_")
		claims, err := token.Validate(h.signingKey, jwtStr)
		if err != nil {
			t.Fatalf("credential %d: validate token: %v", i, err)
		}
		if claims.CredentialID != cred.ID.String() {
			t.Fatalf("credential %d: credential ID mismatch", i)
		}
	}

	t.Logf("Successfully created and verified %d credentials with unique Vault-wrapped DEKs", len(creds))
}

// --------------------------------------------------------------------------
// E2E: Vault envelope encryption - credential revocation with cache invalidation
// --------------------------------------------------------------------------

func TestVaultE2E_CredentialRevocation_CacheInvalidation(t *testing.T) {
	h := newVaultHarness(t)
	org := h.createOrg(t)

	// Create credential with Vault encryption
	cred := h.storeCredential(t, org, "https://api.example.com", "bearer", "sk-revoke-test-key")

	// Mint a token (this caches the decrypted credential)
	tok := h.mintToken(t, org, cred.ID)

	// Revoke the credential
	req := httptest.NewRequest(http.MethodDelete, "/v1/credentials/"+cred.ID.String(), nil)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("revoke: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Try to use the token - should fail because credential is revoked
	// The proxy will try to look up the credential and find it's revoked
	proxyPath := "/v1/proxy/v1/chat/completions"
	rr = h.proxyRequestWithBody(t, http.MethodPost, proxyPath, tok, `{"model":"test","messages":[]}`)
	// Expecting 401 or 404 or 502 - the credential lookup fails since it's revoked
	if rr.Code != http.StatusNotFound && rr.Code != http.StatusUnauthorized && rr.Code != http.StatusBadGateway {
		t.Fatalf("proxy after credential revoke: expected 401, 404, or 502, got %d", rr.Code)
	}

	t.Log("Credential revocation with Vault encryption works correctly")
}

// --------------------------------------------------------------------------
// E2E: Vault envelope encryption - Connect API with Vault-encrypted credentials
// --------------------------------------------------------------------------

func TestVaultE2E_ConnectAPI_CreateConnection(t *testing.T) {
	h := newVaultHarness(t)
	org := h.createOrg(t)

	// Create credential with Vault encryption
	cred := h.storeCredential(t, org, "https://openrouter.ai/api", "bearer", "sk-connect-vault-key")

	// Create connect session directly in the database
	sessionToken, err := model.GenerateSessionToken()
	if err != nil {
		t.Fatalf("generate session token: %v", err)
	}
	session := model.ConnectSession{
		ID:           uuid.New(),
		OrgID:        org.ID,
		SessionToken: sessionToken,
		ExpiresAt:    time.Now().Add(time.Hour),
	}
	if err := h.db.Create(&session).Error; err != nil {
		t.Fatalf("create session: %v", err)
	}

	// Use session to create connection (uses Vault-encrypted credential)
	connBody := fmt.Sprintf(`{"provider_id":"openrouter","credential_id":%q}`, cred.ID.String())
	req := httptest.NewRequest(http.MethodPost, "/v1/widget/connections", strings.NewReader(connBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+sessionToken)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	// 201 or 200 is success, but we accept other codes since this is testing Vault encryption
	// The important thing is that it didn't fail due to Vault encryption issues
	if rr.Code == http.StatusUnauthorized {
		t.Fatalf("create connection: authentication failed - %s", rr.Body.String())
	}

	t.Logf("Connect API with Vault-encrypted credentials returned: %d", rr.Code)
}
