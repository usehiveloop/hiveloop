package middleware_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/useportal/proxy-bridge/internal/middleware"
	"github.com/useportal/proxy-bridge/internal/model"
	"github.com/useportal/proxy-bridge/internal/token"
	zitadelclient "github.com/useportal/proxy-bridge/internal/zitadel"
)

const (
	testDBURL      = "postgres://proxybridge:localdev@localhost:5433/proxybridge?sslmode=disable"
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

// zitadelTestHelper manages ZITADEL test resources.
type zitadelTestHelper struct {
	client    *zitadelclient.Client
	creds     *zitadelclient.Credentials
	adminPAT  string
	domain    string
}

func newZitadelHelper(t *testing.T) *zitadelTestHelper {
	t.Helper()

	domain := os.Getenv("ZITADEL_DOMAIN")
	if domain == "" {
		domain = "http://localhost:8085"
	}

	patPath := os.Getenv("ZITADEL_PAT_PATH")
	if patPath == "" {
		patPath = "../../docker/zitadel/bootstrap/admin.pat"
	}
	pat, err := os.ReadFile(patPath)
	if err != nil {
		t.Fatalf("cannot read ZITADEL admin PAT from %s: %v", patPath, err)
	}

	credsPath := os.Getenv("ZITADEL_CREDS_PATH")
	if credsPath == "" {
		credsPath = "../../docker/zitadel/bootstrap/api-credentials.json"
	}
	creds, err := zitadelclient.LoadCredentials(credsPath)
	if err != nil {
		t.Fatalf("cannot load ZITADEL credentials from %s: %v", credsPath, err)
	}

	return &zitadelTestHelper{
		client:   zitadelclient.NewClient(domain, strings.TrimSpace(string(pat))),
		creds:    creds,
		adminPAT: strings.TrimSpace(string(pat)),
		domain:   domain,
	}
}

// createTestOrg creates a ZITADEL org, machine user with PAT, grants project roles,
// and creates the corresponding LLMVault Org record.
func (z *zitadelTestHelper) createTestOrg(t *testing.T, db *gorm.DB, name string, roles []string) (model.Org, string) {
	t.Helper()

	// Append UUID suffix for uniqueness across test runs
	uniqueName := fmt.Sprintf("%s-%s", name, uuid.New().String()[:8])

	// Create org in ZITADEL
	zitadelOrgID, err := z.client.CreateOrganization(uniqueName)
	if err != nil {
		t.Fatalf("failed to create ZITADEL org: %v", err)
	}

	// Grant project to this new org
	_, err = z.client.GrantProjectToOrg(z.creds.ProjectID, zitadelOrgID, roles)
	if err != nil {
		t.Fatalf("failed to grant project to org: %v", err)
	}

	// Create machine user in the new org
	username := fmt.Sprintf("svc-%s", uuid.New().String()[:8])
	userID, err := z.client.CreateMachineUser(zitadelOrgID, username, uniqueName+" Service")
	if err != nil {
		t.Fatalf("failed to create machine user: %v", err)
	}

	// Grant roles to the user
	err = z.client.GrantProjectRoles(zitadelOrgID, userID, z.creds.ProjectID, roles)
	if err != nil {
		t.Fatalf("failed to grant roles: %v", err)
	}

	// Create PAT for the user
	userPAT, err := z.client.CreatePAT(zitadelOrgID, userID)
	if err != nil {
		t.Fatalf("failed to create PAT: %v", err)
	}

	// Create LLMVault Org record in Postgres
	org := model.Org{
		ID:           uuid.New(),
		Name:         uniqueName,
		ZitadelOrgID: zitadelOrgID,
		RateLimit:    1000,
		Active:       true,
	}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("failed to create org in DB: %v", err)
	}
	t.Cleanup(func() { cleanupOrg(t, db, org.ID) })

	return org, userPAT
}

// --------------------------------------------------------------------------
// ZITADEL Auth — real ZITADEL + real Postgres
// --------------------------------------------------------------------------

func TestIntegration_ZitadelAuth_ValidToken(t *testing.T) {
	db := connectTestDB(t)
	zh := newZitadelHelper(t)

	org, userPAT := zh.createTestOrg(t, db, "test-zitadel-valid", []string{"admin"})

	mw, err := middleware.NewZitadelAuth(context.Background(), zh.domain, zh.creds.ClientID, zh.creds.ClientSecret)
	if err != nil {
		t.Fatalf("failed to init ZITADEL auth: %v", err)
	}

	var gotOrg *model.Org
	handler := mw.RequireAuthorization()(
		middleware.ResolveOrg(mw, db)(
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
	req.Header.Set("Authorization", "Bearer "+userPAT)
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

func TestIntegration_ZitadelAuth_MissingToken(t *testing.T) {
	db := connectTestDB(t)
	zh := newZitadelHelper(t)

	mw, err := middleware.NewZitadelAuth(context.Background(), zh.domain, zh.creds.ClientID, zh.creds.ClientSecret)
	if err != nil {
		t.Fatalf("failed to init ZITADEL auth: %v", err)
	}

	handler := mw.RequireAuthorization()(
		middleware.ResolveOrg(mw, db)(
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

func TestIntegration_ZitadelAuth_InvalidToken(t *testing.T) {
	db := connectTestDB(t)
	zh := newZitadelHelper(t)

	mw, err := middleware.NewZitadelAuth(context.Background(), zh.domain, zh.creds.ClientID, zh.creds.ClientSecret)
	if err != nil {
		t.Fatalf("failed to init ZITADEL auth: %v", err)
	}

	handler := mw.RequireAuthorization()(
		middleware.ResolveOrg(mw, db)(
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

func TestIntegration_ZitadelAuth_InactiveOrg(t *testing.T) {
	db := connectTestDB(t)
	zh := newZitadelHelper(t)

	org, userPAT := zh.createTestOrg(t, db, "test-zitadel-inactive", []string{"admin"})

	// Deactivate the org
	if err := db.Model(&org).Update("active", false).Error; err != nil {
		t.Fatalf("failed to deactivate org: %v", err)
	}

	mw, err := middleware.NewZitadelAuth(context.Background(), zh.domain, zh.creds.ClientID, zh.creds.ClientSecret)
	if err != nil {
		t.Fatalf("failed to init ZITADEL auth: %v", err)
	}

	handler := mw.RequireAuthorization()(
		middleware.ResolveOrg(mw, db)(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Fatal("handler should not be called for inactive org")
			}),
		),
	)

	req := httptest.NewRequest(http.MethodGet, "/v1/credentials", nil)
	req.Header.Set("Authorization", "Bearer "+userPAT)
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
// ZITADEL Role-Based Access
// --------------------------------------------------------------------------

func TestIntegration_ZitadelAuth_RequireRole(t *testing.T) {
	db := connectTestDB(t)
	zh := newZitadelHelper(t)

	_, userPAT := zh.createTestOrg(t, db, "test-zitadel-role", []string{"viewer"})

	mw, err := middleware.NewZitadelAuth(context.Background(), zh.domain, zh.creds.ClientID, zh.creds.ClientSecret)
	if err != nil {
		t.Fatalf("failed to init ZITADEL auth: %v", err)
	}

	// Require "admin" role, but user only has "viewer"
	handler := mw.RequireAuthorization()(
		middleware.ResolveOrg(mw, db)(
			middleware.RequireRole(mw, "admin")(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					t.Fatal("handler should not be called without admin role")
				}),
			),
		),
	)

	req := httptest.NewRequest(http.MethodPost, "/v1/credentials", nil)
	req.Header.Set("Authorization", "Bearer "+userPAT)
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
		ID:           orgID,
		Name:         "integration-token-org",
		ZitadelOrgID: fmt.Sprintf("ztdl-token-%s", uuid.New().String()[:8]),
		RateLimit:    1000,
		Active:       true,
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
		ID:           orgID,
		Name:         "integration-revoked-token-org",
		ZitadelOrgID: fmt.Sprintf("ztdl-revoked-%s", uuid.New().String()[:8]),
		RateLimit:    1000,
		Active:       true,
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
		ID:           orgID,
		Name:         "integration-audit-org",
		ZitadelOrgID: fmt.Sprintf("ztdl-audit-%s", uuid.New().String()[:8]),
		RateLimit:    1000,
		Active:       true,
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
	handler := middleware.Audit(aw)(inner)

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
		ID:           orgID,
		Name:         "integration-audit-multi",
		ZitadelOrgID: fmt.Sprintf("ztdl-audit-multi-%s", uuid.New().String()[:8]),
		RateLimit:    1000,
		Active:       true,
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
		ID:           orgID,
		Name:         "integration-ratelimit-org",
		ZitadelOrgID: fmt.Sprintf("ztdl-rl-%s", uuid.New().String()[:8]),
		RateLimit:    1, // 1 per minute → burst of 1
		Active:       true,
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
		ID:           uuid.New(),
		Name:         "integration-rl-org1",
		ZitadelOrgID: fmt.Sprintf("ztdl-rl1-%s", uuid.New().String()[:8]),
		RateLimit:    1,
		Active:       true,
	}
	if err := db.Create(&org1).Error; err != nil {
		t.Fatalf("failed to create org1: %v", err)
	}
	t.Cleanup(func() { cleanupOrg(t, db, org1.ID) })

	org2 := model.Org{
		ID:           uuid.New(),
		Name:         "integration-rl-org2",
		ZitadelOrgID: fmt.Sprintf("ztdl-rl2-%s", uuid.New().String()[:8]),
		RateLimit:    6000,
		Active:       true,
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
