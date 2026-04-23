package e2e

import (
	"context"
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

	"github.com/usehiveloop/hiveloop/internal/auth"
	"github.com/usehiveloop/hiveloop/internal/email"
	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

const (
	orgTestIssuer   = "hiveloop-e2e-org-test"
	orgTestAudience = "hiveloop-e2e"
)

type orgHarness struct {
	*testHarness
	privateKey  *rsa.PrivateKey
	publicKey   *rsa.PublicKey
	signingHMAC []byte
	orgRouter   *chi.Mux
}

func newOrgHarness(t *testing.T) *orgHarness {
	t.Helper()

	h := newHarness(t)

	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	pubKey := &privKey.PublicKey
	signingHMAC := []byte("e2e-org-hmac-signing-key")

	authHandler := handler.NewAuthHandler(h.db, privKey, signingHMAC, orgTestIssuer, orgTestAudience, 15*time.Minute, 24*time.Hour, &email.LogSender{}, "http://localhost:3000", true)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	authHandler.StartCleanup(ctx)
	orgHandler := handler.NewOrgHandler(h.db)

	r := chi.NewRouter()

	r.Post("/auth/register", authHandler.Register)
	r.Post("/auth/login", authHandler.Login)

	r.Route("/v1", func(r chi.Router) {
		r.Use(middleware.RequireAuth(pubKey, orgTestIssuer, orgTestAudience))

		r.Post("/orgs", orgHandler.Create)

		r.Group(func(r chi.Router) {
			r.Use(middleware.ResolveOrgFromClaims(h.db))
			r.Get("/orgs/current", orgHandler.Current)
		})
	})

	return &orgHarness{
		testHarness: h,
		privateKey:  privKey,
		publicKey:   pubKey,
		signingHMAC: signingHMAC,
		orgRouter:   r,
	}
}

func (oh *orgHarness) registerUser(t *testing.T, email, password, name string) authResponseDTO {
	t.Helper()

	body := fmt.Sprintf(`{"email":%q,"password":%q,"name":%q}`, email, password, name)
	req := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	oh.orgRouter.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("POST /auth/register: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp authResponseDTO
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode register response: %v", err)
	}
	return resp
}

func (oh *orgHarness) loginUser(t *testing.T, email, password string, orgID string) authResponseDTO {
	t.Helper()

	var body string
	if orgID != "" {
		body = fmt.Sprintf(`{"email":%q,"password":%q,"org_id":%q}`, email, password, orgID)
	} else {
		body = fmt.Sprintf(`{"email":%q,"password":%q}`, email, password)
	}
	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	oh.orgRouter.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("POST /auth/login: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp authResponseDTO
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	return resp
}

func (oh *orgHarness) issueToken(t *testing.T, userID, orgID, role string) string {
	t.Helper()
	tok, err := auth.IssueAccessToken(oh.privateKey, orgTestIssuer, orgTestAudience, userID, orgID, role, 15*time.Minute)
	if err != nil {
		t.Fatalf("issue access token: %v", err)
	}
	return tok
}

func (oh *orgHarness) orgRequest(t *testing.T, method, path string, body string, token string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *strings.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	var req *http.Request
	if reader != nil {
		req = httptest.NewRequest(method, path, reader)
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rr := httptest.NewRecorder()
	oh.orgRouter.ServeHTTP(rr, req)
	return rr
}

func randomSuffix() string {
	return uuid.New().String()[:8]
}

type authResponseDTO struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	User         struct {
		ID    string `json:"id"`
		Email string `json:"email"`
		Name  string `json:"name"`
	} `json:"user"`
	Orgs []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Role string `json:"role"`
	} `json:"orgs"`
}
