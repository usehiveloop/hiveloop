package middleware_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
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

	"github.com/llmvault/llmvault/internal/auth"
	"github.com/llmvault/llmvault/internal/middleware"
	"github.com/llmvault/llmvault/internal/model"
	"github.com/llmvault/llmvault/internal/token"
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

// authTestHelper manages RSA key pairs and JWT minting for tests.
type authTestHelper struct {
	privKey  *rsa.PrivateKey
	issuer   string
	audience string
}

func newAuthHelper(t *testing.T) *authTestHelper {
	t.Helper()
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}
	return &authTestHelper{
		privKey:  privKey,
		issuer:   "test-issuer",
		audience: "test-audience",
	}
}

// createTestOrg creates an LLMVault Org in Postgres and mints a JWT for it.
func (ah *authTestHelper) createTestOrg(t *testing.T, db *gorm.DB, name, role string) (model.Org, string) {
	t.Helper()

	uniqueName := fmt.Sprintf("%s-%s", name, uuid.New().String()[:8])
	orgID := uuid.New()
	userID := uuid.New().String()

	org := model.Org{
		ID:        orgID,
		Name:      uniqueName,
		RateLimit: 1000,
		Active:    true,
	}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("failed to create org in DB: %v", err)
	}
	t.Cleanup(func() { cleanupOrg(t, db, orgID) })

	jwtToken, err := auth.IssueAccessToken(ah.privKey, ah.issuer, ah.audience, userID, orgID.String(), role, time.Hour)
	if err != nil {
		t.Fatalf("failed to issue access token: %v", err)
	}

	return org, jwtToken
}

// --------------------------------------------------------------------------
// Auth — RSA JWT + real Postgres
// --------------------------------------------------------------------------

func TestIntegration_Auth_ValidToken(t *testing.T) {
	db := connectTestDB(t)
	ah := newAuthHelper(t)

	org, userJWT := ah.createTestOrg(t, db, "test-auth-valid", "admin")

	var gotOrg *model.Org
	handler := middleware.RequireAuth(&ah.privKey.PublicKey, ah.issuer, ah.audience)(
		middleware.ResolveOrgFromClaims(db)(
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

func TestIntegration_Auth_MissingToken(t *testing.T) {
	db := connectTestDB(t)
	ah := newAuthHelper(t)

	handler := middleware.RequireAuth(&ah.privKey.PublicKey, ah.issuer, ah.audience)(
		middleware.ResolveOrgFromClaims(db)(
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

func TestIntegration_Auth_InvalidToken(t *testing.T) {
	db := connectTestDB(t)
	ah := newAuthHelper(t)

	handler := middleware.RequireAuth(&ah.privKey.PublicKey, ah.issuer, ah.audience)(
		middleware.ResolveOrgFromClaims(db)(
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

func TestIntegration_Auth_InactiveOrg(t *testing.T) {
	db := connectTestDB(t)
	ah := newAuthHelper(t)

	org, userJWT := ah.createTestOrg(t, db, "test-auth-inactive", "admin")

	// Deactivate the org
	if err := db.Model(&org).Update("active", false).Error; err != nil {
		t.Fatalf("failed to deactivate org: %v", err)
	}

	handler := middleware.RequireAuth(&ah.privKey.PublicKey, ah.issuer, ah.audience)(
		middleware.ResolveOrgFromClaims(db)(
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
// Auth — JWT claims validation
// --------------------------------------------------------------------------

func TestIntegration_Auth_WrongIssuerRejected(t *testing.T) {
	db := connectTestDB(t)
	ah := newAuthHelper(t)

	orgID := uuid.New()
	org := model.Org{
		ID:        orgID,
		Name:      fmt.Sprintf("test-auth-issuer-%s", uuid.New().String()[:8]),
		RateLimit: 1000,
		Active:    true,
	}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("failed to create org: %v", err)
	}
	t.Cleanup(func() { cleanupOrg(t, db, orgID) })

	// Mint JWT with wrong issuer
	wrongJWT, err := auth.IssueAccessToken(ah.privKey, "wrong-issuer", ah.audience, uuid.New().String(), orgID.String(), "admin", time.Hour)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	handler := middleware.RequireAuth(&ah.privKey.PublicKey, ah.issuer, ah.audience)(
		middleware.ResolveOrgFromClaims(db)(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Fatal("handler should not be called with wrong issuer")
			}),
		),
	)

	req := httptest.NewRequest(http.MethodGet, "/v1/credentials", nil)
	req.Header.Set("Authorization", "Bearer "+wrongJWT)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d; body: %s", rr.Code, rr.Body.String())
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
		ID:        orgID,
		Name:      "integration-token-org",
		RateLimit: 1000,
		Active:    true,
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
		ID:        orgID,
		Name:      "integration-revoked-token-org",
		RateLimit: 1000,
		Active:    true,
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

// TestIntegration_TokenAuth_XApiKey tests Anthropic-style auth (x-api-key header).
func TestIntegration_TokenAuth_XApiKey(t *testing.T) {
	db := connectTestDB(t)

	orgID := uuid.New()
	credID := uuid.New()
	db.Create(&model.Org{ID: orgID, Name: "token-xapikey-" + uuid.New().String()[:8], RateLimit: 1000, Active: true})
	db.Create(&model.Credential{ID: credID, OrgID: orgID, Label: "test", BaseURL: "https://api.anthropic.com", AuthScheme: "x-api-key", EncryptedKey: []byte("e"), WrappedDEK: []byte("w")})
	t.Cleanup(func() { cleanupOrg(t, db, orgID) })

	signingKey := []byte(testSigningKey)
	tokenStr, jti, _ := token.Mint(signingKey, orgID.String(), credID.String(), time.Hour)
	db.Create(&model.Token{ID: uuid.New(), OrgID: orgID, CredentialID: credID, JTI: jti, ExpiresAt: time.Now().Add(time.Hour)})

	var gotClaims *middleware.TokenClaims
	handler := middleware.TokenAuth(signingKey, db)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotClaims, _ = middleware.ClaimsFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/proxy/v1/messages", nil)
	req.Header.Set("x-api-key", "ptok_"+tokenStr)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("x-api-key auth: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if gotClaims.OrgID != orgID.String() {
		t.Fatalf("org_id mismatch: got %s", gotClaims.OrgID)
	}
}

// TestIntegration_TokenAuth_AzureApiKey tests Azure-style auth (api-key header).
func TestIntegration_TokenAuth_AzureApiKey(t *testing.T) {
	db := connectTestDB(t)

	orgID := uuid.New()
	credID := uuid.New()
	db.Create(&model.Org{ID: orgID, Name: "token-azure-" + uuid.New().String()[:8], RateLimit: 1000, Active: true})
	db.Create(&model.Credential{ID: credID, OrgID: orgID, Label: "test", BaseURL: "https://myinstance.openai.azure.com", AuthScheme: "api-key", EncryptedKey: []byte("e"), WrappedDEK: []byte("w")})
	t.Cleanup(func() { cleanupOrg(t, db, orgID) })

	signingKey := []byte(testSigningKey)
	tokenStr, jti, _ := token.Mint(signingKey, orgID.String(), credID.String(), time.Hour)
	db.Create(&model.Token{ID: uuid.New(), OrgID: orgID, CredentialID: credID, JTI: jti, ExpiresAt: time.Now().Add(time.Hour)})

	var gotClaims *middleware.TokenClaims
	handler := middleware.TokenAuth(signingKey, db)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotClaims, _ = middleware.ClaimsFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/proxy/v1/chat/completions", nil)
	req.Header.Set("api-key", "ptok_"+tokenStr)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("api-key auth: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if gotClaims.OrgID != orgID.String() {
		t.Fatalf("org_id mismatch: got %s", gotClaims.OrgID)
	}
}

// TestIntegration_TokenAuth_QueryParam tests Google-style auth (?key= query parameter).
func TestIntegration_TokenAuth_QueryParam(t *testing.T) {
	db := connectTestDB(t)

	orgID := uuid.New()
	credID := uuid.New()
	db.Create(&model.Org{ID: orgID, Name: "token-google-" + uuid.New().String()[:8], RateLimit: 1000, Active: true})
	db.Create(&model.Credential{ID: credID, OrgID: orgID, Label: "test", BaseURL: "https://generativelanguage.googleapis.com", AuthScheme: "query_param", EncryptedKey: []byte("e"), WrappedDEK: []byte("w")})
	t.Cleanup(func() { cleanupOrg(t, db, orgID) })

	signingKey := []byte(testSigningKey)
	tokenStr, jti, _ := token.Mint(signingKey, orgID.String(), credID.String(), time.Hour)
	db.Create(&model.Token{ID: uuid.New(), OrgID: orgID, CredentialID: credID, JTI: jti, ExpiresAt: time.Now().Add(time.Hour)})

	var gotClaims *middleware.TokenClaims
	handler := middleware.TokenAuth(signingKey, db)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotClaims, _ = middleware.ClaimsFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/proxy/v1/models/gemini:generateContent?key=ptok_"+tokenStr, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("query param auth: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if gotClaims.OrgID != orgID.String() {
		t.Fatalf("org_id mismatch: got %s", gotClaims.OrgID)
	}
}

// TestIntegration_TokenAuth_NoAuth tests that requests without any auth are rejected.
func TestIntegration_TokenAuth_NoAuth(t *testing.T) {
	db := connectTestDB(t)
	signingKey := []byte(testSigningKey)

	handler := middleware.TokenAuth(signingKey, db)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called without auth")
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/proxy/v1/messages", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("no auth: expected 401, got %d", rr.Code)
	}
}

// --------------------------------------------------------------------------
// Audit — real Postgres
// --------------------------------------------------------------------------

func TestIntegration_Audit_WritesToPostgres(t *testing.T) {
	db := connectTestDB(t)

	orgID := uuid.New()
	org := model.Org{
		ID:        orgID,
		Name:      "integration-audit-org",
		RateLimit: 1000,
		Active:    true,
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
		ID:        orgID,
		Name:      "integration-audit-multi",
		RateLimit: 1000,
		Active:    true,
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
		ID:        orgID,
		Name:      "integration-ratelimit-org",
		RateLimit: 1, // 1 per minute -> burst of 1
		Active:    true,
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
		ID:        uuid.New(),
		Name:      "integration-rl-org1",
		RateLimit: 1,
		Active:    true,
	}
	if err := db.Create(&org1).Error; err != nil {
		t.Fatalf("failed to create org1: %v", err)
	}
	t.Cleanup(func() { cleanupOrg(t, db, org1.ID) })

	org2 := model.Org{
		ID:        uuid.New(),
		Name:      "integration-rl-org2",
		RateLimit: 6000,
		Active:    true,
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
