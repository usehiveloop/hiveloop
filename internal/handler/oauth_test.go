package handler_test

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/handler"
	"github.com/ziraloop/ziraloop/internal/model"
)

// ---------------------------------------------------------------------------
// Test harness
// ---------------------------------------------------------------------------

type oauthTestHarness struct {
	db      *gorm.DB
	handler *handler.OAuthHandler
	router  *chi.Mux
}

func newOAuthHarness(t *testing.T) *oauthTestHarness {
	t.Helper()

	db := connectTestDB(t)

	pk, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	signingKey := []byte("test-signing-key-for-refresh-tokens")

	h := handler.NewOAuthHandler(
		db, pk, signingKey,
		"ziraloop-test", "http://localhost:8080",
		15*time.Minute, 720*time.Hour,
		"http://localhost:3000",
		"", "", // no GitHub creds
		"", "", // no Google creds
		"", "", // no X creds
	)

	r := chi.NewRouter()
	r.Route("/oauth", func(r chi.Router) {
		r.Get("/github", h.GitHubLogin)
		r.Get("/github/callback", h.GitHubCallback)
		r.Get("/google", h.GoogleLogin)
		r.Get("/google/callback", h.GoogleCallback)
		r.Get("/x", h.XLogin)
		r.Get("/x/callback", h.XCallback)
		r.Post("/exchange", h.Exchange)
	})

	return &oauthTestHarness{db: db, handler: h, router: r}
}

// newOAuthHarnessWithProviders creates a harness with dummy provider creds so
// the login endpoints return redirects instead of 404.
func newOAuthHarnessWithProviders(t *testing.T) *oauthTestHarness {
	t.Helper()

	db := connectTestDB(t)

	pk, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	signingKey := []byte("test-signing-key-for-refresh-tokens")

	h := handler.NewOAuthHandler(
		db, pk, signingKey,
		"ziraloop-test", "http://localhost:8080",
		15*time.Minute, 720*time.Hour,
		"http://localhost:3000",
		"gh-client-id", "gh-client-secret",
		"google-client-id", "google-client-secret",
		"x-client-id", "x-client-secret",
	)

	r := chi.NewRouter()
	r.Route("/oauth", func(r chi.Router) {
		r.Get("/github", h.GitHubLogin)
		r.Get("/github/callback", h.GitHubCallback)
		r.Get("/google", h.GoogleLogin)
		r.Get("/google/callback", h.GoogleCallback)
		r.Get("/x", h.XLogin)
		r.Get("/x/callback", h.XCallback)
		r.Post("/exchange", h.Exchange)
	})

	return &oauthTestHarness{db: db, handler: h, router: r}
}

func (h *oauthTestHarness) doRequest(t *testing.T, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

// createOAuthTestUser creates a user, org, membership, and OAuth account for testing.
func createOAuthTestUser(t *testing.T, db *gorm.DB, email, name, provider, providerUserID string) model.User {
	t.Helper()

	user := model.User{
		Email: email,
		Name:  name,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	org := model.Org{
		Name: fmt.Sprintf("%s's Workspace-%s", name, uuid.New().String()[:8]),
	}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}

	membership := model.OrgMembership{
		UserID: user.ID,
		OrgID:  org.ID,
		Role:   "admin",
	}
	if err := db.Create(&membership).Error; err != nil {
		t.Fatalf("create membership: %v", err)
	}

	oauthAcct := model.OAuthAccount{
		UserID:         user.ID,
		Provider:       provider,
		ProviderUserID: providerUserID,
	}
	if err := db.Create(&oauthAcct).Error; err != nil {
		t.Fatalf("create oauth account: %v", err)
	}

	t.Cleanup(func() {
		db.Where("user_id = ?", user.ID).Delete(&model.OAuthAccount{})
		db.Where("user_id = ?", user.ID).Delete(&model.OAuthExchangeToken{})
		db.Where("user_id = ?", user.ID).Delete(&model.RefreshToken{})
		db.Where("user_id = ?", user.ID).Delete(&model.OrgMembership{})
		db.Where("id = ?", org.ID).Delete(&model.Org{})
		db.Where("id = ?", user.ID).Delete(&model.User{})
	})

	return user
}

// ---------------------------------------------------------------------------
// Login endpoint tests
// ---------------------------------------------------------------------------

func TestOAuth_Login_ProviderNotConfigured(t *testing.T) {
	h := newOAuthHarness(t)

	for _, path := range []string{"/oauth/github", "/oauth/google", "/oauth/x"} {
		rr := h.doRequest(t, http.MethodGet, path, nil)
		if rr.Code != http.StatusNotFound {
			t.Errorf("%s: expected 404, got %d", path, rr.Code)
		}
	}
}

func TestOAuth_Login_RedirectsToProvider(t *testing.T) {
	h := newOAuthHarnessWithProviders(t)

	tests := []struct {
		path     string
		wantHost string
	}{
		{"/oauth/github", "github.com"},
		{"/oauth/google", "accounts.google.com"},
		{"/oauth/x", "x.com"},
	}

	for _, tc := range tests {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		rr := httptest.NewRecorder()
		h.router.ServeHTTP(rr, req)

		if rr.Code != http.StatusTemporaryRedirect {
			t.Errorf("%s: expected 307, got %d", tc.path, rr.Code)
			continue
		}

		loc := rr.Header().Get("Location")
		if !strings.Contains(loc, tc.wantHost) {
			t.Errorf("%s: expected redirect to %s, got %s", tc.path, tc.wantHost, loc)
		}
	}
}

func TestOAuth_Login_SetsCookies(t *testing.T) {
	h := newOAuthHarnessWithProviders(t)

	req := httptest.NewRequest(http.MethodGet, "/oauth/x", nil)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	cookies := rr.Result().Cookies()
	var hasState, hasVerifier bool
	for _, c := range cookies {
		switch c.Name {
		case "oauth_state":
			hasState = true
			if c.Value == "" {
				t.Error("oauth_state cookie is empty")
			}
			if !c.HttpOnly {
				t.Error("oauth_state cookie should be HttpOnly")
			}
		case "oauth_verifier":
			hasVerifier = true
			if c.Value == "" {
				t.Error("oauth_verifier cookie is empty")
			}
			if !c.HttpOnly {
				t.Error("oauth_verifier cookie should be HttpOnly")
			}
		}
	}
	if !hasState {
		t.Error("missing oauth_state cookie")
	}
	if !hasVerifier {
		t.Error("missing oauth_verifier cookie")
	}
}

func TestOAuth_Login_PKCEInRedirectURL(t *testing.T) {
	h := newOAuthHarnessWithProviders(t)

	req := httptest.NewRequest(http.MethodGet, "/oauth/x", nil)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	loc := rr.Header().Get("Location")
	if !strings.Contains(loc, "code_challenge=") {
		t.Error("redirect URL missing code_challenge parameter")
	}
	if !strings.Contains(loc, "code_challenge_method=S256") {
		t.Error("redirect URL missing code_challenge_method=S256")
	}
}

// ---------------------------------------------------------------------------
// Callback endpoint tests
// ---------------------------------------------------------------------------

func TestOAuth_Callback_ProviderNotConfigured(t *testing.T) {
	h := newOAuthHarness(t) // no providers configured

	req := httptest.NewRequest(http.MethodGet, "/oauth/x/callback?code=abc&state=xyz", nil)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected 307 redirect, got %d", rr.Code)
	}
	loc := rr.Header().Get("Location")
	if !strings.Contains(loc, "error=provider_not_configured") {
		t.Errorf("expected provider_not_configured error, got redirect to %s", loc)
	}
}

func TestOAuth_Callback_InvalidState(t *testing.T) {
	h := newOAuthHarnessWithProviders(t)

	req := httptest.NewRequest(http.MethodGet, "/oauth/x/callback?code=abc&state=wrong", nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: "correct"})
	req.AddCookie(&http.Cookie{Name: "oauth_verifier", Value: "some-verifier"})
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	loc := rr.Header().Get("Location")
	if !strings.Contains(loc, "error=invalid_state") {
		t.Errorf("expected invalid_state error, got redirect to %s", loc)
	}
}

func TestOAuth_Callback_MissingState(t *testing.T) {
	h := newOAuthHarnessWithProviders(t)

	req := httptest.NewRequest(http.MethodGet, "/oauth/x/callback?code=abc&state=xyz", nil)
	// no oauth_state cookie
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	loc := rr.Header().Get("Location")
	if !strings.Contains(loc, "error=invalid_state") {
		t.Errorf("expected invalid_state error, got redirect to %s", loc)
	}
}

func TestOAuth_Callback_MissingVerifier(t *testing.T) {
	h := newOAuthHarnessWithProviders(t)

	req := httptest.NewRequest(http.MethodGet, "/oauth/x/callback?code=abc&state=mystate", nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: "mystate"})
	// no oauth_verifier cookie
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	loc := rr.Header().Get("Location")
	if !strings.Contains(loc, "error=missing_verifier") {
		t.Errorf("expected missing_verifier error, got redirect to %s", loc)
	}
}

func TestOAuth_Callback_MissingCode(t *testing.T) {
	h := newOAuthHarnessWithProviders(t)

	req := httptest.NewRequest(http.MethodGet, "/oauth/x/callback?state=mystate", nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: "mystate"})
	req.AddCookie(&http.Cookie{Name: "oauth_verifier", Value: "some-verifier"})
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	loc := rr.Header().Get("Location")
	if !strings.Contains(loc, "error=missing_code") {
		t.Errorf("expected missing_code error, got redirect to %s", loc)
	}
}

func TestOAuth_Callback_ProviderError(t *testing.T) {
	h := newOAuthHarnessWithProviders(t)

	req := httptest.NewRequest(http.MethodGet, "/oauth/x/callback?state=mystate&error=access_denied", nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: "mystate"})
	req.AddCookie(&http.Cookie{Name: "oauth_verifier", Value: "some-verifier"})
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	loc := rr.Header().Get("Location")
	if !strings.Contains(loc, "error=access_denied") {
		t.Errorf("expected access_denied error forwarded, got redirect to %s", loc)
	}
}

// ---------------------------------------------------------------------------
// Exchange endpoint tests
// ---------------------------------------------------------------------------

func TestOAuth_Exchange_Success(t *testing.T) {
	h := newOAuthHarness(t)

	user := createOAuthTestUser(t, h.db, fmt.Sprintf("exchange-ok-%s@test.com", uuid.New().String()[:8]),
		"Exchange OK", "github", fmt.Sprintf("gh-%s", uuid.New().String()[:8]))

	plaintext, hash, err := model.GenerateExchangeToken()
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	et := model.OAuthExchangeToken{
		UserID:    user.ID,
		TokenHash: hash,
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}
	if err := h.db.Create(&et).Error; err != nil {
		t.Fatalf("create exchange token: %v", err)
	}
	t.Cleanup(func() { h.db.Where("id = ?", et.ID).Delete(&model.OAuthExchangeToken{}) })

	rr := h.doRequest(t, http.MethodPost, "/oauth/exchange", map[string]string{"token": plaintext})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)

	if resp["access_token"] == nil || resp["access_token"] == "" {
		t.Error("missing access_token")
	}
	if resp["refresh_token"] == nil || resp["refresh_token"] == "" {
		t.Error("missing refresh_token")
	}
	userResp := resp["user"].(map[string]any)
	if userResp["email"] != user.Email {
		t.Errorf("expected email %s, got %v", user.Email, userResp["email"])
	}
}

func TestOAuth_Exchange_MissingToken(t *testing.T) {
	h := newOAuthHarness(t)

	rr := h.doRequest(t, http.MethodPost, "/oauth/exchange", map[string]string{"token": ""})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestOAuth_Exchange_InvalidToken(t *testing.T) {
	h := newOAuthHarness(t)

	rr := h.doRequest(t, http.MethodPost, "/oauth/exchange", map[string]string{"token": "bogus-token"})
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestOAuth_Exchange_ExpiredToken(t *testing.T) {
	h := newOAuthHarness(t)

	user := createOAuthTestUser(t, h.db, fmt.Sprintf("exchange-exp-%s@test.com", uuid.New().String()[:8]),
		"Exchange Expired", "github", fmt.Sprintf("gh-%s", uuid.New().String()[:8]))

	plaintext, hash, err := model.GenerateExchangeToken()
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	et := model.OAuthExchangeToken{
		UserID:    user.ID,
		TokenHash: hash,
		ExpiresAt: time.Now().Add(-1 * time.Minute), // already expired
	}
	if err := h.db.Create(&et).Error; err != nil {
		t.Fatalf("create exchange token: %v", err)
	}
	t.Cleanup(func() { h.db.Where("id = ?", et.ID).Delete(&model.OAuthExchangeToken{}) })

	rr := h.doRequest(t, http.MethodPost, "/oauth/exchange", map[string]string{"token": plaintext})
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

func TestOAuth_Exchange_UsedToken(t *testing.T) {
	h := newOAuthHarness(t)

	user := createOAuthTestUser(t, h.db, fmt.Sprintf("exchange-used-%s@test.com", uuid.New().String()[:8]),
		"Exchange Used", "github", fmt.Sprintf("gh-%s", uuid.New().String()[:8]))

	plaintext, hash, err := model.GenerateExchangeToken()
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	now := time.Now()
	et := model.OAuthExchangeToken{
		UserID:    user.ID,
		TokenHash: hash,
		ExpiresAt: time.Now().Add(5 * time.Minute),
		UsedAt:    &now, // already used
	}
	if err := h.db.Create(&et).Error; err != nil {
		t.Fatalf("create exchange token: %v", err)
	}
	t.Cleanup(func() { h.db.Where("id = ?", et.ID).Delete(&model.OAuthExchangeToken{}) })

	rr := h.doRequest(t, http.MethodPost, "/oauth/exchange", map[string]string{"token": plaintext})
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

func TestOAuth_Exchange_InvalidBody(t *testing.T) {
	h := newOAuthHarness(t)

	req := httptest.NewRequest(http.MethodPost, "/oauth/exchange", strings.NewReader("not-json"))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestOAuth_Exchange_EmailConfirmedField(t *testing.T) {
	h := newOAuthHarness(t)

	// User with confirmed email
	confirmedUser := createOAuthTestUser(t, h.db,
		fmt.Sprintf("confirmed-%s@test.com", uuid.New().String()[:8]),
		"Confirmed", "google", fmt.Sprintf("goog-%s", uuid.New().String()[:8]))
	now := time.Now()
	h.db.Model(&confirmedUser).Update("email_confirmed_at", &now)

	plaintext1, hash1, _ := model.GenerateExchangeToken()
	h.db.Create(&model.OAuthExchangeToken{UserID: confirmedUser.ID, TokenHash: hash1, ExpiresAt: time.Now().Add(5 * time.Minute)})
	t.Cleanup(func() { h.db.Where("token_hash = ?", hash1).Delete(&model.OAuthExchangeToken{}) })

	rr1 := h.doRequest(t, http.MethodPost, "/oauth/exchange", map[string]string{"token": plaintext1})
	var resp1 map[string]any
	json.NewDecoder(rr1.Body).Decode(&resp1)
	user1 := resp1["user"].(map[string]any)
	if user1["email_confirmed"] != true {
		t.Errorf("expected email_confirmed=true, got %v", user1["email_confirmed"])
	}

	// User with placeholder email (unconfirmed)
	placeholderUser := createOAuthTestUser(t, h.db,
		fmt.Sprintf("xuser-%s@placeholder-email.com", uuid.New().String()[:8]),
		"X User", "x", fmt.Sprintf("x-%s", uuid.New().String()[:8]))

	plaintext2, hash2, _ := model.GenerateExchangeToken()
	h.db.Create(&model.OAuthExchangeToken{UserID: placeholderUser.ID, TokenHash: hash2, ExpiresAt: time.Now().Add(5 * time.Minute)})
	t.Cleanup(func() { h.db.Where("token_hash = ?", hash2).Delete(&model.OAuthExchangeToken{}) })

	rr2 := h.doRequest(t, http.MethodPost, "/oauth/exchange", map[string]string{"token": plaintext2})
	var resp2 map[string]any
	json.NewDecoder(rr2.Body).Decode(&resp2)
	user2 := resp2["user"].(map[string]any)
	if user2["email_confirmed"] != false {
		t.Errorf("expected email_confirmed=false for placeholder email, got %v", user2["email_confirmed"])
	}
}
