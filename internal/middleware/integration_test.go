package middleware_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/useportal/llmvault/internal/logto"
	"github.com/useportal/llmvault/internal/middleware"
	"github.com/useportal/llmvault/internal/model"
	"github.com/useportal/llmvault/internal/token"
)

const (
	testDBURL      = "postgres://llmvault:localdev@localhost:5433/llmvault_test?sslmode=disable"
	testSigningKey = "local-dev-signing-key-change-in-prod"
)

// connectTestDB opens a real Postgres connection and runs migrations.
func connectTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = testDBURL
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("cannot connect to Postgres: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("failed to get underlying sql.DB: %v", err)
	}
	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("Postgres not reachable: %v", err)
	}

	if err := model.AutoMigrate(db); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	return db
}

// cleanupOrg deletes a test org and its dependents after the test.
func cleanupOrg(t *testing.T, db *gorm.DB, orgID uuid.UUID) {
	t.Helper()
	db.Where("org_id = ?", orgID).Delete(&model.AuditEntry{})
	db.Where("org_id = ?", orgID).Delete(&model.Token{})
	db.Where("org_id = ?", orgID).Delete(&model.Credential{})
	db.Where("id = ?", orgID).Delete(&model.Org{})
}

// logtoTestHelper manages Logto test resources.
type logtoTestHelper struct {
	client        *logto.Client
	endpoint      string // reachable URL (e.g. http://localhost:3301)
	issuer        string // OIDC issuer URL from Logto's ENDPOINT config (e.g. http://localhost:3001/oidc)
	audience      string
	testAppID     string
	testAppSecret string
}

func newLogtoHelper(t *testing.T) *logtoTestHelper {
	t.Helper()

	endpoint := os.Getenv("LOGTO_ENDPOINT")
	if endpoint == "" {
		endpoint = "https://auth.dev.llmvault.dev"
	}

	audience := os.Getenv("LOGTO_AUDIENCE")
	if audience == "" {
		audience = "https://api.llmvault.dev"
	}

	// M2M credentials for Management API
	m2mAppID := os.Getenv("LOGTO_M2M_APP_ID")
	m2mAppSecret := os.Getenv("LOGTO_M2M_APP_SECRET")
	if m2mAppID == "" || m2mAppSecret == "" {
		t.Fatal("LOGTO_M2M_APP_ID and LOGTO_M2M_APP_SECRET must be set")
	}

	// Test M2M app credentials (for simulating authenticated requests)
	testAppID := os.Getenv("LOGTO_TEST_APP_ID")
	testAppSecret := os.Getenv("LOGTO_TEST_APP_SECRET")
	if testAppID == "" || testAppSecret == "" {
		t.Fatal("LOGTO_TEST_APP_ID and LOGTO_TEST_APP_SECRET must be set")
	}

	// Discover the actual OIDC issuer from Logto (may differ from endpoint due to port mapping)
	issuer := endpoint + "/oidc"
	resp, err := http.Get(endpoint + "/oidc/.well-known/openid-configuration")
	if err == nil {
		defer resp.Body.Close()
		var oidcConfig struct {
			Issuer string `json:"issuer"`
		}
		if json.NewDecoder(resp.Body).Decode(&oidcConfig) == nil && oidcConfig.Issuer != "" {
			issuer = oidcConfig.Issuer
		}
	}

	return &logtoTestHelper{
		client:        logto.NewClient(endpoint, m2mAppID, m2mAppSecret),
		endpoint:      endpoint,
		issuer:        issuer,
		audience:      audience,
		testAppID:     testAppID,
		testAppSecret: testAppSecret,
	}
}

// newLogtoAuth creates a LogtoAuth middleware configured with the correct
// issuer and JWKS URL (handles port mapping between issuer and reachable endpoint).
func (lh *logtoTestHelper) newLogtoAuth() *middleware.LogtoAuth {
	auth := middleware.NewLogtoAuth(lh.issuer, lh.audience)
	// If the issuer URL differs from the endpoint (e.g. port mapping),
	// set the JWKS URL to the reachable endpoint.
	if lh.issuer != lh.endpoint+"/oidc" {
		auth.SetJWKSURL(lh.endpoint + "/oidc")
	}
	return auth
}

// createTestOrg creates a Logto org, adds the test M2M app to it with the
// specified roles, and creates the corresponding LLMVault Org record.
func (lh *logtoTestHelper) createTestOrg(t *testing.T, db *gorm.DB, name string, roles []string) (model.Org, string) {
	t.Helper()

	uniqueName := fmt.Sprintf("%s-%s", name, uuid.New().String()[:8])

	// Create org in Logto
	logtoOrgID, err := lh.client.CreateOrganization(uniqueName)
	if err != nil {
		t.Fatalf("failed to create Logto org: %v", err)
	}

	// Add the test M2M app to the org
	if err := lh.client.AddOrgMemberM2M(logtoOrgID, lh.testAppID); err != nil {
		t.Fatalf("failed to add test app to org: %v", err)
	}

	// Get role IDs and assign them
	var roleIDs []string
	for _, roleName := range roles {
		roleID, err := lh.client.GetOrgRoleByName(roleName)
		if err != nil {
			t.Fatalf("failed to get org role %q: %v", roleName, err)
		}
		roleIDs = append(roleIDs, roleID)
	}
	if len(roleIDs) > 0 {
		if err := lh.client.AssignOrgRoleToM2M(logtoOrgID, lh.testAppID, roleIDs); err != nil {
			t.Fatalf("failed to assign org roles: %v", err)
		}
	}

	// Get an org-scoped JWT for the test M2M app
	scopes := make([]string, 0)
	for _, r := range roles {
		scopes = append(scopes, r)
	}
	jwt, err := lh.client.GetM2MOrgToken(lh.testAppID, lh.testAppSecret, logtoOrgID, lh.audience, scopes)
	if err != nil {
		t.Fatalf("failed to get org-scoped token: %v", err)
	}

	// Create LLMVault Org record in Postgres
	org := model.Org{
		ID:         uuid.New(),
		Name:       uniqueName,
		LogtoOrgID: logtoOrgID,
		RateLimit:  1000,
		Active:     true,
	}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("failed to create org in DB: %v", err)
	}
	t.Cleanup(func() { cleanupOrg(t, db, org.ID) })

	return org, jwt
}

// --------------------------------------------------------------------------
// Logto Auth — real Logto + real Postgres
// --------------------------------------------------------------------------

func TestIntegration_LogtoAuth_ValidToken(t *testing.T) {
	db := connectTestDB(t)
	lh := newLogtoHelper(t)

	org, userJWT := lh.createTestOrg(t, db, "test-logto-valid", []string{"m2m:admin"})

	logtoAuth := lh.newLogtoAuth()

	var gotOrg *model.Org
	handler := logtoAuth.RequireAuthorization()(
		middleware.ResolveOrg(db)(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var ok bool
				gotOrg, ok = middleware.OrgFromContext(r.Context())
				if !ok {
					t.Fatal("org not found in context")
				}
				w.WriteHeader(http.StatusOK)
			}),
		),
	)

	req := httptest.NewRequest(http.MethodGet, "/v1/credentials", nil)
	req.Header.Set("Authorization", "Bearer "+userJWT)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}
	if gotOrg == nil || gotOrg.ID != org.ID {
		t.Fatalf("expected org ID %s, got %v", org.ID, gotOrg)
	}
	if gotOrg.Name != org.Name {
		t.Fatalf("expected org name %q, got %s", org.Name, gotOrg.Name)
	}
}

func TestIntegration_LogtoAuth_MissingToken(t *testing.T) {
	db := connectTestDB(t)
	lh := newLogtoHelper(t)

	logtoAuth := lh.newLogtoAuth()

	handler := logtoAuth.RequireAuthorization()(
		middleware.ResolveOrg(db)(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Fatal("handler should not be called")
			}),
		),
	)

	req := httptest.NewRequest(http.MethodGet, "/v1/credentials", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

func TestIntegration_LogtoAuth_InvalidToken(t *testing.T) {
	db := connectTestDB(t)
	lh := newLogtoHelper(t)

	logtoAuth := lh.newLogtoAuth()

	handler := logtoAuth.RequireAuthorization()(
		middleware.ResolveOrg(db)(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Fatal("handler should not be called")
			}),
		),
	)

	req := httptest.NewRequest(http.MethodGet, "/v1/credentials", nil)
	req.Header.Set("Authorization", "Bearer invalid-token-xyz")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

func TestIntegration_LogtoAuth_InactiveOrg(t *testing.T) {
	db := connectTestDB(t)
	lh := newLogtoHelper(t)

	org, userJWT := lh.createTestOrg(t, db, "test-logto-inactive", []string{"m2m:admin"})

	// Deactivate the org
	if err := db.Model(&org).Update("active", false).Error; err != nil {
		t.Fatalf("failed to deactivate org: %v", err)
	}

	logtoAuth := lh.newLogtoAuth()

	handler := logtoAuth.RequireAuthorization()(
		middleware.ResolveOrg(db)(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Fatal("handler should not be called for inactive org")
			}),
		),
	)

	req := httptest.NewRequest(http.MethodGet, "/v1/credentials", nil)
	req.Header.Set("Authorization", "Bearer "+userJWT)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var body map[string]string
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if body["error"] != "organization is inactive" {
		t.Fatalf("unexpected error: %s", body["error"])
	}
}

// --------------------------------------------------------------------------
// Logto Scope-Based Access
// --------------------------------------------------------------------------

func TestIntegration_LogtoAuth_RequireScope(t *testing.T) {
	db := connectTestDB(t)
	lh := newLogtoHelper(t)

	_, userJWT := lh.createTestOrg(t, db, "test-logto-scope", []string{"m2m:viewer"})

	logtoAuth := lh.newLogtoAuth()

	// Require "admin" scope, but user only has "viewer"
	handler := logtoAuth.RequireAuthorization()(
		middleware.ResolveOrg(db)(
			middleware.RequireScope("admin")(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					t.Fatal("handler should not be called without admin scope")
				}),
			),
		),
	)

	req := httptest.NewRequest(http.MethodPost, "/v1/credentials", nil)
	req.Header.Set("Authorization", "Bearer "+userJWT)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

// --------------------------------------------------------------------------
// Token Auth (ptok_ sandbox tokens) — unchanged, real Postgres
// --------------------------------------------------------------------------

func TestIntegration_TokenAuth_ValidToken(t *testing.T) {
	db := connectTestDB(t)

	orgID := uuid.New()
	credID := uuid.New()

	org := model.Org{
		ID:         orgID,
		Name:       "integration-token-org",
		LogtoOrgID: fmt.Sprintf("logto-token-%s", uuid.New().String()[:8]),
		RateLimit:  1000,
		Active:     true,
	}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("failed to create org: %v", err)
	}
	cred := model.Credential{
		ID:           credID,
		OrgID:        orgID,
		Label:        "test-cred",
		BaseURL:      "https://api.example.com",
		AuthScheme:   "bearer",
		EncryptedKey: []byte("fake-encrypted"),
		WrappedDEK:   []byte("fake-wrapped"),
	}
	if err := db.Create(&cred).Error; err != nil {
		t.Fatalf("failed to create credential: %v", err)
	}
	t.Cleanup(func() { cleanupOrg(t, db, orgID) })

	signingKey := []byte(testSigningKey)
	tokenStr, jti, err := token.Mint(signingKey, orgID.String(), credID.String(), time.Hour)
	if err != nil {
		t.Fatalf("failed to mint token: %v", err)
	}

	tokenRecord := model.Token{
		ID:           uuid.New(),
		OrgID:        orgID,
		CredentialID: credID,
		JTI:          jti,
		ExpiresAt:    time.Now().Add(time.Hour),
	}
	if err := db.Create(&tokenRecord).Error; err != nil {
		t.Fatalf("failed to create token record: %v", err)
	}

	var gotClaims *middleware.TokenClaims
	handler := middleware.TokenAuth(signingKey, db)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var ok bool
		gotClaims, ok = middleware.ClaimsFromContext(r.Context())
		if !ok {
			t.Fatal("claims not found in context")
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/proxy/chat", nil)
	req.Header.Set("Authorization", "Bearer ptok_"+tokenStr)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if gotClaims.OrgID != orgID.String() {
		t.Fatalf("expected org_id %s, got %s", orgID, gotClaims.OrgID)
	}
	if gotClaims.CredentialID != credID.String() {
		t.Fatalf("expected cred_id %s, got %s", credID, gotClaims.CredentialID)
	}
	if gotClaims.JTI != jti {
		t.Fatalf("expected jti %s, got %s", jti, gotClaims.JTI)
	}
}

func TestIntegration_TokenAuth_RevokedToken(t *testing.T) {
	db := connectTestDB(t)

	orgID := uuid.New()
	credID := uuid.New()

	org := model.Org{
		ID:         orgID,
		Name:       "integration-revoked-token-org",
		LogtoOrgID: fmt.Sprintf("logto-revoked-%s", uuid.New().String()[:8]),
		RateLimit:  1000,
		Active:     true,
	}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("failed to create org: %v", err)
	}
	cred := model.Credential{
		ID:           credID,
		OrgID:        orgID,
		Label:        "test-cred",
		BaseURL:      "https://api.example.com",
		AuthScheme:   "bearer",
		EncryptedKey: []byte("fake-encrypted"),
		WrappedDEK:   []byte("fake-wrapped"),
	}
	if err := db.Create(&cred).Error; err != nil {
		t.Fatalf("failed to create credential: %v", err)
	}
	t.Cleanup(func() { cleanupOrg(t, db, orgID) })

	signingKey := []byte(testSigningKey)
	tokenStr, jti, err := token.Mint(signingKey, orgID.String(), credID.String(), time.Hour)
	if err != nil {
		t.Fatalf("failed to mint token: %v", err)
	}

	revokedAt := time.Now()
	tokenRecord := model.Token{
		ID:           uuid.New(),
		OrgID:        orgID,
		CredentialID: credID,
		JTI:          jti,
		ExpiresAt:    time.Now().Add(time.Hour),
		RevokedAt:    &revokedAt,
	}
	if err := db.Create(&tokenRecord).Error; err != nil {
		t.Fatalf("failed to create token record: %v", err)
	}

	handler := middleware.TokenAuth(signingKey, db)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called for revoked token")
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/proxy/chat", nil)
	req.Header.Set("Authorization", "Bearer ptok_"+tokenStr)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}

	var body map[string]string
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if body["error"] != "token has been revoked" {
		t.Fatalf("expected 'token has been revoked', got %s", body["error"])
	}
}

func TestIntegration_TokenAuth_ExpiredToken(t *testing.T) {
	db := connectTestDB(t)

	signingKey := []byte(testSigningKey)
	tokenStr, _, err := token.Mint(signingKey, uuid.New().String(), uuid.New().String(), -time.Hour)
	if err != nil {
		t.Fatalf("failed to mint token: %v", err)
	}

	handler := middleware.TokenAuth(signingKey, db)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called for expired token")
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/proxy/chat", nil)
	req.Header.Set("Authorization", "Bearer ptok_"+tokenStr)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

// --------------------------------------------------------------------------
// Audit — real Postgres
// --------------------------------------------------------------------------

func TestIntegration_Audit_WritesToPostgres(t *testing.T) {
	db := connectTestDB(t)

	orgID := uuid.New()
	org := model.Org{
		ID:         orgID,
		Name:       "integration-audit-org",
		LogtoOrgID: fmt.Sprintf("logto-audit-%s", uuid.New().String()[:8]),
		RateLimit:  1000,
		Active:     true,
	}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("failed to create org: %v", err)
	}
	t.Cleanup(func() { cleanupOrg(t, db, orgID) })

	aw := middleware.NewAuditWriter(db, 100)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	handler := middleware.Audit(aw, "proxy.request")(inner)

	req := httptest.NewRequest(http.MethodPost, "/v1/proxy/v1/messages", nil)
	req = middleware.WithOrg(req, &org)
	req.RemoteAddr = "192.168.1.100:12345"
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	aw.Shutdown(ctx)

	var entries []model.AuditEntry
	if err := db.Where("org_id = ?", orgID).Find(&entries).Error; err != nil {
		t.Fatalf("failed to query audit_log: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(entries))
	}

	entry := entries[0]
	if entry.Action != "proxy.request" {
		t.Fatalf("expected action 'proxy.request', got %s", entry.Action)
	}
	if entry.OrgID != orgID {
		t.Fatalf("expected org_id %s, got %s", orgID, entry.OrgID)
	}
	if entry.IPAddress == nil || *entry.IPAddress != "192.168.1.100" {
		t.Fatalf("expected IP '192.168.1.100', got %v", entry.IPAddress)
	}
	if entry.Metadata == nil {
		t.Fatal("expected metadata, got nil")
	}
	if entry.Metadata["method"] != "POST" {
		t.Fatalf("expected method POST in metadata, got %v", entry.Metadata["method"])
	}
}

func TestIntegration_Audit_MultipleRequestsFlushed(t *testing.T) {
	db := connectTestDB(t)

	orgID := uuid.New()
	org := model.Org{
		ID:         orgID,
		Name:       "integration-audit-multi",
		LogtoOrgID: fmt.Sprintf("logto-audit-multi-%s", uuid.New().String()[:8]),
		RateLimit:  1000,
		Active:     true,
	}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("failed to create org: %v", err)
	}
	t.Cleanup(func() { cleanupOrg(t, db, orgID) })

	aw := middleware.NewAuditWriter(db, 100)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := middleware.Audit(aw)(inner)

	for range 10 {
		req := httptest.NewRequest(http.MethodPost, "/v1/proxy/chat", nil)
		req = middleware.WithOrg(req, &org)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	aw.Shutdown(ctx)

	var count int64
	db.Model(&model.AuditEntry{}).Where("org_id = ?", orgID).Count(&count)
	if count != 10 {
		t.Fatalf("expected 10 audit entries in Postgres, got %d", count)
	}
}

// --------------------------------------------------------------------------
// Rate Limiting — real Postgres, org loaded via context
// --------------------------------------------------------------------------

func TestIntegration_RateLimit_EnforcesLimit(t *testing.T) {
	db := connectTestDB(t)

	orgID := uuid.New()
	org := model.Org{
		ID:         orgID,
		Name:       "integration-ratelimit-org",
		LogtoOrgID: fmt.Sprintf("logto-rl-%s", uuid.New().String()[:8]),
		RateLimit:  1, // 1 per minute -> burst of 1
		Active:     true,
	}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("failed to create org: %v", err)
	}
	t.Cleanup(func() { cleanupOrg(t, db, orgID) })

	rl := middleware.RateLimit()
	handler := rl(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First request (uses burst)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("first request: expected 200, got %d", rr.Code)
	}

	// Second request should be rate limited
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2 = middleware.WithOrg(req2, &org)
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)

	if rr2.Code != http.StatusTooManyRequests {
		t.Fatalf("second request: expected 429, got %d", rr2.Code)
	}

	if rr2.Header().Get("Retry-After") == "" {
		t.Fatal("expected Retry-After header on 429")
	}
}

func TestIntegration_RateLimit_IsolatedPerOrg(t *testing.T) {
	db := connectTestDB(t)

	org1 := model.Org{
		ID:         uuid.New(),
		Name:       "integration-rl-org1",
		LogtoOrgID: fmt.Sprintf("logto-rl1-%s", uuid.New().String()[:8]),
		RateLimit:  1,
		Active:     true,
	}
	if err := db.Create(&org1).Error; err != nil {
		t.Fatalf("failed to create org1: %v", err)
	}
	t.Cleanup(func() { cleanupOrg(t, db, org1.ID) })

	org2 := model.Org{
		ID:         uuid.New(),
		Name:       "integration-rl-org2",
		LogtoOrgID: fmt.Sprintf("logto-rl2-%s", uuid.New().String()[:8]),
		RateLimit:  6000,
		Active:     true,
	}
	if err := db.Create(&org2).Error; err != nil {
		t.Fatalf("failed to create org2: %v", err)
	}
	t.Cleanup(func() { cleanupOrg(t, db, org2.ID) })

	rl := middleware.RateLimit()
	handler := rl(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Exhaust org1's limit
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = middleware.WithOrg(req, &org1)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req = middleware.WithOrg(req, &org1)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("org1 should be rate limited, got %d", rr.Code)
	}

	// Org2 should still be allowed
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req = middleware.WithOrg(req, &org2)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("org2 should not be rate limited, got %d", rr.Code)
	}
}
