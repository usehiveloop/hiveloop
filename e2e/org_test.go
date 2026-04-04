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

	"github.com/ziraloop/ziraloop/internal/auth"
	"github.com/ziraloop/ziraloop/internal/email"
	"github.com/ziraloop/ziraloop/internal/handler"
	"github.com/ziraloop/ziraloop/internal/middleware"
	"github.com/ziraloop/ziraloop/internal/model"
)

const (
	orgTestIssuer   = "ziraloop-e2e-org-test"
	orgTestAudience = "ziraloop-e2e"
)

// orgHarness bundles infrastructure for org E2E tests using the embedded auth system.
type orgHarness struct {
	*testHarness
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
	signingHMAC []byte
	orgRouter  *chi.Mux
}

func newOrgHarness(t *testing.T) *orgHarness {
	t.Helper()

	h := newHarness(t)

	// Generate RSA key pair for JWT signing/validation
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	pubKey := &privKey.PublicKey
	signingHMAC := []byte("e2e-org-hmac-signing-key")

	// Handlers
	authHandler := handler.NewAuthHandler(h.db, privKey, signingHMAC, orgTestIssuer, orgTestAudience, 15*time.Minute, 24*time.Hour, &email.LogSender{}, "http://localhost:3000", true)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	authHandler.StartCleanup(ctx)
	orgHandler := handler.NewOrgHandler(h.db)

	// Router with embedded auth
	r := chi.NewRouter()

	// Public auth routes (no auth middleware)
	r.Post("/auth/register", authHandler.Register)
	r.Post("/auth/login", authHandler.Login)

	// Protected routes
	r.Route("/v1", func(r chi.Router) {
		r.Use(middleware.RequireAuth(pubKey, orgTestIssuer, orgTestAudience))

		// Org management (no org context needed, user creates an org)
		r.Post("/orgs", orgHandler.Create)

		// Org-scoped routes (require resolved org from JWT claims)
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

// registerUser creates a new user via the auth handler and returns the auth response.
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

// loginUser logs in and returns the auth response.
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

// issueToken creates an access token directly using the auth package (for test convenience).
func (oh *orgHarness) issueToken(t *testing.T, userID, orgID, role string) string {
	t.Helper()
	tok, err := auth.IssueAccessToken(oh.privateKey, orgTestIssuer, orgTestAudience, userID, orgID, role, 15*time.Minute)
	if err != nil {
		t.Fatalf("issue access token: %v", err)
	}
	return tok
}

// orgRequest makes an authenticated request to the org router.
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

func TestOrgCreate(t *testing.T) {
	oh := newOrgHarness(t)

	suffix := uuid.New().String()[:8]
	email := fmt.Sprintf("orgcreate-%s@test.local", suffix)
	regResp := oh.registerUser(t, email, "password123", "OrgCreator")

	t.Cleanup(func() {
		// Clean up user, memberships, orgs
		oh.db.Where("user_id = ?", regResp.User.ID).Delete(&model.OrgMembership{})
		oh.db.Where("id = ?", regResp.User.ID).Delete(&model.User{})
		for _, o := range regResp.Orgs {
			oh.db.Where("id = ?", o.ID).Delete(&model.Org{})
		}
	})

	// Use the access token from registration (which is scoped to the auto-created org)
	// to create a second org.
	orgName := fmt.Sprintf("e2e-org-%s", uuid.New().String()[:8])
	body := fmt.Sprintf(`{"name":%q}`, orgName)

	rr := oh.orgRequest(t, http.MethodPost, "/v1/orgs", body, regResp.AccessToken)
	if rr.Code != http.StatusCreated {
		t.Fatalf("POST /v1/orgs: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		RateLimit int    `json:"rate_limit"`
		Active    bool   `json:"active"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	t.Cleanup(func() {
		oh.db.Where("org_id = ?", resp.ID).Delete(&model.OrgMembership{})
		oh.db.Where("id = ?", resp.ID).Delete(&model.Org{})
	})

	// Verify response fields
	if resp.Name != orgName {
		t.Errorf("name: got %q, want %q", resp.Name, orgName)
	}
	if resp.ID == "" {
		t.Error("id is empty")
	}
	if !resp.Active {
		t.Error("org should be active")
	}
	if resp.RateLimit != 1000 {
		t.Errorf("rate_limit: got %d, want 1000 (default)", resp.RateLimit)
	}

	// Verify org exists in DB
	var dbOrg model.Org
	if err := oh.db.Where("id = ?", resp.ID).First(&dbOrg).Error; err != nil {
		t.Fatalf("org not found in DB: %v", err)
	}
	if dbOrg.Name != orgName {
		t.Errorf("DB name mismatch: got %q, want %q", dbOrg.Name, orgName)
	}
}

func TestOrgCreateValidation(t *testing.T) {
	oh := newOrgHarness(t)

	suffix := uuid.New().String()[:8]
	email := fmt.Sprintf("orgval-%s@test.local", suffix)
	regResp := oh.registerUser(t, email, "password123", "Validator")

	t.Cleanup(func() {
		oh.db.Where("user_id = ?", regResp.User.ID).Delete(&model.OrgMembership{})
		for _, o := range regResp.Orgs {
			oh.db.Where("id = ?", o.ID).Delete(&model.Org{})
		}
		oh.db.Where("id = ?", regResp.User.ID).Delete(&model.User{})
	})

	tok := regResp.AccessToken

	// Missing name
	rr := oh.orgRequest(t, http.MethodPost, "/v1/orgs", `{"name":""}`, tok)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("empty name: expected 400, got %d", rr.Code)
	}

	// Invalid JSON
	rr = oh.orgRequest(t, http.MethodPost, "/v1/orgs", `not json`, tok)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("invalid json: expected 400, got %d", rr.Code)
	}
}

func TestOrgCreateUnauthenticated(t *testing.T) {
	oh := newOrgHarness(t)

	req := httptest.NewRequest(http.MethodPost, "/v1/orgs", strings.NewReader(`{"name":"nope"}`))
	req.Header.Set("Content-Type", "application/json")
	// No Authorization header
	rr := httptest.NewRecorder()
	oh.orgRouter.ServeHTTP(rr, req)

	if rr.Code == http.StatusCreated {
		t.Error("expected unauthenticated request to fail, got 201")
	}
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestOrgCurrent(t *testing.T) {
	oh := newOrgHarness(t)

	suffix := uuid.New().String()[:8]
	email := fmt.Sprintf("orgcur-%s@test.local", suffix)
	regResp := oh.registerUser(t, email, "password123", "CurrentUser")

	t.Cleanup(func() {
		oh.db.Where("user_id = ?", regResp.User.ID).Delete(&model.OrgMembership{})
		for _, o := range regResp.Orgs {
			oh.db.Where("id = ?", o.ID).Delete(&model.Org{})
		}
		oh.db.Where("id = ?", regResp.User.ID).Delete(&model.User{})
	})

	// Create a new org
	orgName := fmt.Sprintf("e2e-current-%s", uuid.New().String()[:8])
	body := fmt.Sprintf(`{"name":%q}`, orgName)

	createRR := oh.orgRequest(t, http.MethodPost, "/v1/orgs", body, regResp.AccessToken)
	if createRR.Code != http.StatusCreated {
		t.Fatalf("POST /v1/orgs: expected 201, got %d: %s", createRR.Code, createRR.Body.String())
	}

	var created struct {
		ID string `json:"id"`
	}
	json.NewDecoder(createRR.Body).Decode(&created)

	t.Cleanup(func() {
		oh.db.Where("org_id = ?", created.ID).Delete(&model.OrgMembership{})
		oh.db.Where("id = ?", created.ID).Delete(&model.Org{})
	})

	// Issue a token scoped to the new org
	orgTok := oh.issueToken(t, regResp.User.ID, created.ID, "admin")

	// GET /v1/orgs/current
	rr := oh.orgRequest(t, http.MethodGet, "/v1/orgs/current", "", orgTok)

	if rr.Code != http.StatusOK {
		t.Fatalf("GET /v1/orgs/current: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Active bool   `json:"active"`
	}
	json.NewDecoder(rr.Body).Decode(&resp)

	if resp.ID != created.ID {
		t.Errorf("id: got %q, want %q", resp.ID, created.ID)
	}
	if resp.Name != orgName {
		t.Errorf("name: got %q, want %q", resp.Name, orgName)
	}
	if !resp.Active {
		t.Error("org should be active")
	}
}

func TestOrgCreateDuplicateName(t *testing.T) {
	oh := newOrgHarness(t)

	suffix := uuid.New().String()[:8]
	email := fmt.Sprintf("orgdup-%s@test.local", suffix)
	regResp := oh.registerUser(t, email, "password123", "DupTester")

	t.Cleanup(func() {
		oh.db.Where("user_id = ?", regResp.User.ID).Delete(&model.OrgMembership{})
		for _, o := range regResp.Orgs {
			oh.db.Where("id = ?", o.ID).Delete(&model.Org{})
		}
		oh.db.Where("id = ?", regResp.User.ID).Delete(&model.User{})
	})

	tok := regResp.AccessToken

	orgName := fmt.Sprintf("e2e-dup-%s", uuid.New().String()[:8])
	body := fmt.Sprintf(`{"name":%q}`, orgName)

	rr1 := oh.orgRequest(t, http.MethodPost, "/v1/orgs", body, tok)
	if rr1.Code != http.StatusCreated {
		t.Fatalf("first create: expected 201, got %d: %s", rr1.Code, rr1.Body.String())
	}
	var org1 struct{ ID string }
	json.NewDecoder(rr1.Body).Decode(&org1)

	t.Cleanup(func() {
		oh.db.Where("org_id = ?", org1.ID).Delete(&model.OrgMembership{})
		oh.db.Where("id = ?", org1.ID).Delete(&model.Org{})
	})

	// Second create with same name should fail
	rr2 := oh.orgRequest(t, http.MethodPost, "/v1/orgs", body, tok)
	if rr2.Code != http.StatusInternalServerError {
		t.Errorf("duplicate name: expected 500, got %d: %s", rr2.Code, rr2.Body.String())
	}
}

func TestOrgCreateMultiple(t *testing.T) {
	oh := newOrgHarness(t)

	suffix := uuid.New().String()[:8]
	email := fmt.Sprintf("orgmulti-%s@test.local", suffix)
	regResp := oh.registerUser(t, email, "password123", "MultiTester")

	t.Cleanup(func() {
		oh.db.Where("user_id = ?", regResp.User.ID).Delete(&model.OrgMembership{})
		for _, o := range regResp.Orgs {
			oh.db.Where("id = ?", o.ID).Delete(&model.Org{})
		}
		oh.db.Where("id = ?", regResp.User.ID).Delete(&model.User{})
	})

	tok := regResp.AccessToken

	// Create two orgs with different names
	name1 := fmt.Sprintf("e2e-multi-a-%s", uuid.New().String()[:8])
	name2 := fmt.Sprintf("e2e-multi-b-%s", uuid.New().String()[:8])

	rr1 := oh.orgRequest(t, http.MethodPost, "/v1/orgs", fmt.Sprintf(`{"name":%q}`, name1), tok)
	if rr1.Code != http.StatusCreated {
		t.Fatalf("first create: expected 201, got %d: %s", rr1.Code, rr1.Body.String())
	}
	var org1 struct {
		ID string `json:"id"`
	}
	json.NewDecoder(rr1.Body).Decode(&org1)

	rr2 := oh.orgRequest(t, http.MethodPost, "/v1/orgs", fmt.Sprintf(`{"name":%q}`, name2), tok)
	if rr2.Code != http.StatusCreated {
		t.Fatalf("second create: expected 201, got %d: %s", rr2.Code, rr2.Body.String())
	}
	var org2 struct {
		ID string `json:"id"`
	}
	json.NewDecoder(rr2.Body).Decode(&org2)

	if org1.ID == org2.ID {
		t.Error("two orgs should have different IDs")
	}

	t.Cleanup(func() {
		oh.db.Where("org_id IN ?", []string{org1.ID, org2.ID}).Delete(&model.OrgMembership{})
		oh.db.Where("id IN ?", []string{org1.ID, org2.ID}).Delete(&model.Org{})
	})
}

func randomSuffix() string {
	return uuid.New().String()[:8]
}
