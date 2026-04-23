package handler_test

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/model"
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
		"hiveloop-test", "http://localhost:8080",
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
		"hiveloop-test", "http://localhost:8080",
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

