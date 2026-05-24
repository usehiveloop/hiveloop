package middleware_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/token"
)

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
		OrgID:        &orgID,
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
		OrgID:        &orgID,
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
	db.Create(&model.Credential{ID: credID, OrgID: &orgID, Label: "test", BaseURL: "https://api.anthropic.com", AuthScheme: "x-api-key", EncryptedKey: []byte("e"), WrappedDEK: []byte("w")})
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
	db.Create(&model.Credential{ID: credID, OrgID: &orgID, Label: "test", BaseURL: "https://myinstance.openai.azure.com", AuthScheme: "api-key", EncryptedKey: []byte("e"), WrappedDEK: []byte("w")})
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
	db.Create(&model.Credential{ID: credID, OrgID: &orgID, Label: "test", BaseURL: "https://generativelanguage.googleapis.com", AuthScheme: "query_param", EncryptedKey: []byte("e"), WrappedDEK: []byte("w")})
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
