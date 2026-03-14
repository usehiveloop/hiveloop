package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/llmvault/llmvault/internal/handler"
	"github.com/llmvault/llmvault/internal/logto"
	"github.com/llmvault/llmvault/internal/middleware"
	"github.com/llmvault/llmvault/internal/model"
)

// orgHarness extends the base testHarness with Logto integration for org tests.
type orgHarness struct {
	*testHarness
	logtoClient *logto.Client
	orgRouter   *chi.Mux
	logtoAuth   *middleware.LogtoAuth
	// Test M2M app credentials (for simulating authenticated requests)
	testAppID     string
	testAppSecret string
	audience      string
	endpoint      string
}

func newOrgHarness(t *testing.T) *orgHarness {
	t.Helper()

	h := newHarness(t)

	endpoint := envOr("LOGTO_ENDPOINT", "https://auth.dev.llmvault.dev")
	audience := envOr("LOGTO_AUDIENCE", "https://api.llmvault.dev")

	m2mAppID := os.Getenv("LOGTO_M2M_APP_ID")
	m2mAppSecret := os.Getenv("LOGTO_M2M_APP_SECRET")
	if m2mAppID == "" || m2mAppSecret == "" {
		t.Fatal("LOGTO_M2M_APP_ID and LOGTO_M2M_APP_SECRET must be set")
	}

	testAppID := os.Getenv("LOGTO_TEST_APP_ID")
	testAppSecret := os.Getenv("LOGTO_TEST_APP_SECRET")
	if testAppID == "" || testAppSecret == "" {
		t.Fatal("LOGTO_TEST_APP_ID and LOGTO_TEST_APP_SECRET must be set")
	}

	logtoClient := logto.NewClient(endpoint, m2mAppID, m2mAppSecret)

	// Discover the actual OIDC issuer (may differ from endpoint due to port mapping)
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

	// Logto JWT auth middleware
	logtoAuth := middleware.NewLogtoAuth(issuer, audience)
	if issuer != endpoint+"/oidc" {
		logtoAuth.SetJWKSURL(endpoint + "/oidc")
	}

	// Org handler
	orgHandler := handler.NewOrgHandler(h.db, logtoClient)

	// Router with real Logto JWT authentication
	r := chi.NewRouter()
	r.Route("/v1", func(r chi.Router) {
		r.Use(logtoAuth.RequireAuthorization())

		// Org management (no org context needed)
		r.Post("/orgs", orgHandler.Create)

		// Org-scoped routes (require resolved org)
		r.Group(func(r chi.Router) {
			r.Use(middleware.ResolveOrg(h.db))
			r.Get("/orgs/current", orgHandler.Current)
		})
	})

	return &orgHarness{
		testHarness:   h,
		logtoClient:   logtoClient,
		orgRouter:     r,
		logtoAuth:     logtoAuth,
		testAppID:     testAppID,
		testAppSecret: testAppSecret,
		audience:      audience,
		endpoint:      endpoint,
	}
}

// getTestToken obtains an M2M token scoped to an organization.
func (oh *orgHarness) getOrgToken(t *testing.T, orgID string) string {
	t.Helper()
	tok, err := oh.logtoClient.GetM2MOrgToken(oh.testAppID, oh.testAppSecret, orgID, oh.audience, []string{"m2m:admin"})
	if err != nil {
		t.Fatalf("failed to get org-scoped token: %v", err)
	}
	return tok
}

// getNonOrgToken obtains a non-org-scoped M2M token (for creating orgs).
func (oh *orgHarness) getNonOrgToken(t *testing.T) string {
	t.Helper()
	tok, err := oh.logtoClient.GetM2MToken(oh.testAppID, oh.testAppSecret, oh.audience)
	if err != nil {
		t.Fatalf("failed to get M2M token: %v", err)
	}
	return tok
}

// orgRequest makes an authenticated request to the org router using a Logto M2M token.
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
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	oh.orgRouter.ServeHTTP(rr, req)
	return rr
}

func TestOrgCreate(t *testing.T) {
	oh := newOrgHarness(t)

	tok := oh.getNonOrgToken(t)

	orgName := fmt.Sprintf("e2e-org-%s", randomSuffix())
	body := fmt.Sprintf(`{"name":%q}`, orgName)

	rr := oh.orgRequest(t, http.MethodPost, "/v1/orgs", body, tok)
	if rr.Code != http.StatusCreated {
		t.Fatalf("POST /v1/orgs: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		ID         string `json:"id"`
		Name       string `json:"name"`
		LogtoOrgID string `json:"logto_org_id"`
		RateLimit  int    `json:"rate_limit"`
		Active     bool   `json:"active"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	// Verify response fields
	if resp.Name != orgName {
		t.Errorf("name: got %q, want %q", resp.Name, orgName)
	}
	if resp.LogtoOrgID == "" {
		t.Error("logto_org_id is empty")
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

	// Verify org exists in local DB
	var dbOrg model.Org
	if err := oh.db.Where("id = ?", resp.ID).First(&dbOrg).Error; err != nil {
		t.Fatalf("org not found in DB: %v", err)
	}
	if dbOrg.LogtoOrgID != resp.LogtoOrgID {
		t.Errorf("DB logto_org_id mismatch: got %q, want %q", dbOrg.LogtoOrgID, resp.LogtoOrgID)
	}

	// Cleanup
	t.Cleanup(func() {
		oh.db.Where("id = ?", resp.ID).Delete(&model.Org{})
	})
}

func TestOrgCreateValidation(t *testing.T) {
	oh := newOrgHarness(t)
	tok := oh.getNonOrgToken(t)

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
}

func TestOrgCurrent(t *testing.T) {
	oh := newOrgHarness(t)
	tok := oh.getNonOrgToken(t)

	// First create an org so it exists in both Logto and local DB
	orgName := fmt.Sprintf("e2e-current-%s", randomSuffix())
	body := fmt.Sprintf(`{"name":%q}`, orgName)

	createRR := oh.orgRequest(t, http.MethodPost, "/v1/orgs", body, tok)
	if createRR.Code != http.StatusCreated {
		t.Fatalf("POST /v1/orgs: expected 201, got %d: %s", createRR.Code, createRR.Body.String())
	}

	var created struct {
		ID         string `json:"id"`
		LogtoOrgID string `json:"logto_org_id"`
	}
	json.NewDecoder(createRR.Body).Decode(&created)

	t.Cleanup(func() {
		oh.db.Where("id = ?", created.ID).Delete(&model.Org{})
	})

	// GET /v1/orgs/current requires an org-scoped JWT.
	// We need to add the test M2M app to the Logto org and get an org-scoped token.
	if err := oh.logtoClient.AddOrgMemberM2M(created.LogtoOrgID, oh.testAppID); err != nil {
		t.Fatalf("failed to add test app to org: %v", err)
	}
	adminRoleID, err := oh.logtoClient.GetOrgRoleByName("m2m:admin")
	if err != nil {
		t.Fatalf("failed to get admin role: %v", err)
	}
	if err := oh.logtoClient.AssignOrgRoleToM2M(created.LogtoOrgID, oh.testAppID, []string{adminRoleID}); err != nil {
		t.Fatalf("failed to assign admin role: %v", err)
	}

	orgTok := oh.getOrgToken(t, created.LogtoOrgID)

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
	tok := oh.getNonOrgToken(t)

	orgName := fmt.Sprintf("e2e-dup-%s", randomSuffix())
	body := fmt.Sprintf(`{"name":%q}`, orgName)

	rr1 := oh.orgRequest(t, http.MethodPost, "/v1/orgs", body, tok)
	if rr1.Code != http.StatusCreated {
		t.Fatalf("first create: expected 201, got %d: %s", rr1.Code, rr1.Body.String())
	}
	var org1 struct{ ID string }
	json.NewDecoder(rr1.Body).Decode(&org1)

	t.Cleanup(func() {
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
	tok := oh.getNonOrgToken(t)

	// Create two orgs with different names
	name1 := fmt.Sprintf("e2e-multi-a-%s", randomSuffix())
	name2 := fmt.Sprintf("e2e-multi-b-%s", randomSuffix())

	rr1 := oh.orgRequest(t, http.MethodPost, "/v1/orgs", fmt.Sprintf(`{"name":%q}`, name1), tok)
	if rr1.Code != http.StatusCreated {
		t.Fatalf("first create: expected 201, got %d: %s", rr1.Code, rr1.Body.String())
	}
	var org1 struct {
		ID         string `json:"id"`
		LogtoOrgID string `json:"logto_org_id"`
	}
	json.NewDecoder(rr1.Body).Decode(&org1)

	rr2 := oh.orgRequest(t, http.MethodPost, "/v1/orgs", fmt.Sprintf(`{"name":%q}`, name2), tok)
	if rr2.Code != http.StatusCreated {
		t.Fatalf("second create: expected 201, got %d: %s", rr2.Code, rr2.Body.String())
	}
	var org2 struct {
		ID         string `json:"id"`
		LogtoOrgID string `json:"logto_org_id"`
	}
	json.NewDecoder(rr2.Body).Decode(&org2)

	if org1.ID == org2.ID {
		t.Error("two orgs should have different IDs")
	}
	if org1.LogtoOrgID == org2.LogtoOrgID {
		t.Error("two orgs should have different Logto org IDs")
	}

	t.Cleanup(func() {
		oh.db.Where("id IN ?", []string{org1.ID, org2.ID}).Delete(&model.Org{})
	})
}

func randomSuffix() string {
	return uuid.New().String()[:8]
}
