package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	zmw "github.com/zitadel/zitadel-go/v3/pkg/http/middleware"
	"github.com/zitadel/zitadel-go/v3/pkg/authorization/oauth"

	"github.com/useportal/llmvault/internal/handler"
	"github.com/useportal/llmvault/internal/middleware"
	"github.com/useportal/llmvault/internal/model"
	"github.com/useportal/llmvault/internal/zitadel"
)

const (
	testZitadelDomain = "http://localhost:8085"
	testBootstrapDir  = "../docker/zitadel/bootstrap"
)

// orgHarness extends the base testHarness with ZITADEL integration for org tests.
type orgHarness struct {
	*testHarness
	zClient    *zitadel.Client
	adminPAT   string
	projectID  string
	orgRouter  *chi.Mux
	zitadelMW  *zmw.Interceptor[*oauth.IntrospectionContext]
}

func loadAdminPAT(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(testBootstrapDir + "/admin.pat")
	if err != nil {
		t.Fatalf("cannot read admin PAT (is Docker Compose running?): %v", err)
	}
	return strings.TrimSpace(string(data))
}

type apiCreds struct {
	ProjectID    string
	ClientID     string
	ClientSecret string
}

func loadAPICreds(t *testing.T) apiCreds {
	t.Helper()
	creds := apiCreds{
		ProjectID:    os.Getenv("ZITADEL_PROJECT_ID"),
		ClientID:     os.Getenv("ZITADEL_CLIENT_ID"),
		ClientSecret: os.Getenv("ZITADEL_CLIENT_SECRET"),
	}
	if creds.ProjectID == "" || creds.ClientID == "" || creds.ClientSecret == "" {
		t.Fatal("ZITADEL_CLIENT_ID, ZITADEL_CLIENT_SECRET, and ZITADEL_PROJECT_ID must be set")
	}
	return creds
}

func newOrgHarness(t *testing.T) *orgHarness {
	t.Helper()

	h := newHarness(t)

	adminPAT := loadAdminPAT(t)
	creds := loadAPICreds(t)

	domain := envOr("ZITADEL_DOMAIN", testZitadelDomain)

	// ZITADEL admin client (uses admin PAT to manage orgs)
	zClient := zitadel.NewClient(domain, adminPAT)

	// ZITADEL middleware (introspects bearer tokens via API app credentials)
	ctx := context.Background()
	zitadelMW, err := middleware.NewZitadelAuth(ctx, domain, creds.ClientID, creds.ClientSecret)
	if err != nil {
		t.Fatalf("cannot initialize ZITADEL auth: %v", err)
	}

	// Org handler
	orgHandler := handler.NewOrgHandler(h.db, zClient, creds.ProjectID)

	// Router with real ZITADEL authentication
	r := chi.NewRouter()
	r.Route("/v1", func(r chi.Router) {
		r.Use(zitadelMW.RequireAuthorization())

		// Org management (no org context needed)
		r.Post("/orgs", orgHandler.Create)

		// Org-scoped routes (require resolved org)
		r.Group(func(r chi.Router) {
			r.Use(middleware.ResolveOrg(zitadelMW, h.db))
			r.Get("/orgs/current", orgHandler.Current)
		})
	})

	return &orgHarness{
		testHarness: h,
		zClient:     zClient,
		adminPAT:    adminPAT,
		projectID:   creds.ProjectID,
		orgRouter:   r,
		zitadelMW:   zitadelMW,
	}
}

// orgRequest makes an authenticated request to the org router using the admin PAT.
func (oh *orgHarness) orgRequest(t *testing.T, method, path string, body string) *httptest.ResponseRecorder {
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
	req.Header.Set("Authorization", "Bearer "+oh.adminPAT)
	rr := httptest.NewRecorder()
	oh.orgRouter.ServeHTTP(rr, req)
	return rr
}

func TestOrgCreate(t *testing.T) {
	oh := newOrgHarness(t)

	orgName := fmt.Sprintf("e2e-org-%s", randomSuffix())
	body := fmt.Sprintf(`{"name":%q}`, orgName)

	rr := oh.orgRequest(t, http.MethodPost, "/v1/orgs", body)
	if rr.Code != http.StatusCreated {
		t.Fatalf("POST /v1/orgs: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		ID           string `json:"id"`
		Name         string `json:"name"`
		ZitadelOrgID string `json:"zitadel_org_id"`
		RateLimit    int    `json:"rate_limit"`
		Active       bool   `json:"active"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	// Verify response fields
	if resp.Name != orgName {
		t.Errorf("name: got %q, want %q", resp.Name, orgName)
	}
	if resp.ZitadelOrgID == "" {
		t.Error("zitadel_org_id is empty")
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
	if dbOrg.ZitadelOrgID != resp.ZitadelOrgID {
		t.Errorf("DB zitadel_org_id mismatch: got %q, want %q", dbOrg.ZitadelOrgID, resp.ZitadelOrgID)
	}

	// Cleanup
	t.Cleanup(func() {
		oh.db.Where("id = ?", resp.ID).Delete(&model.Org{})
	})
}

func TestOrgCreateValidation(t *testing.T) {
	oh := newOrgHarness(t)

	// Missing name
	rr := oh.orgRequest(t, http.MethodPost, "/v1/orgs", `{"name":""}`)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("empty name: expected 400, got %d", rr.Code)
	}

	// Invalid JSON
	rr = oh.orgRequest(t, http.MethodPost, "/v1/orgs", `not json`)
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

	// First create an org so it exists in both ZITADEL and local DB
	orgName := fmt.Sprintf("e2e-current-%s", randomSuffix())
	body := fmt.Sprintf(`{"name":%q}`, orgName)

	createRR := oh.orgRequest(t, http.MethodPost, "/v1/orgs", body)
	if createRR.Code != http.StatusCreated {
		t.Fatalf("POST /v1/orgs: expected 201, got %d: %s", createRR.Code, createRR.Body.String())
	}

	var created struct {
		ID           string `json:"id"`
		ZitadelOrgID string `json:"zitadel_org_id"`
	}
	json.NewDecoder(createRR.Body).Decode(&created)

	t.Cleanup(func() {
		oh.db.Where("id = ?", created.ID).Delete(&model.Org{})
	})

	// GET /v1/orgs/current requires the ZITADEL token to resolve to this org.
	// The admin PAT resolves to the admin's default org, not the newly created one.
	// So we test this by directly injecting the org context (matching the existing e2e pattern).
	var dbOrg model.Org
	if err := oh.db.Where("id = ?", created.ID).First(&dbOrg).Error; err != nil {
		t.Fatalf("org not in DB: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/orgs/current", nil)
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &dbOrg)
	rr := httptest.NewRecorder()

	// Use a minimal router that just needs org context (no ZITADEL auth)
	directRouter := chi.NewRouter()
	orgHandler := handler.NewOrgHandler(oh.db, oh.zClient, oh.projectID)
	directRouter.Get("/v1/orgs/current", orgHandler.Current)
	directRouter.ServeHTTP(rr, req)

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

	// ZITADEL rejects duplicate org names within the same parent org.
	orgName := fmt.Sprintf("e2e-dup-%s", randomSuffix())
	body := fmt.Sprintf(`{"name":%q}`, orgName)

	rr1 := oh.orgRequest(t, http.MethodPost, "/v1/orgs", body)
	if rr1.Code != http.StatusCreated {
		t.Fatalf("first create: expected 201, got %d: %s", rr1.Code, rr1.Body.String())
	}
	var org1 struct{ ID string }
	json.NewDecoder(rr1.Body).Decode(&org1)

	t.Cleanup(func() {
		oh.db.Where("id = ?", org1.ID).Delete(&model.Org{})
	})

	// Second create with same name should fail
	rr2 := oh.orgRequest(t, http.MethodPost, "/v1/orgs", body)
	if rr2.Code != http.StatusInternalServerError {
		t.Errorf("duplicate name: expected 500, got %d: %s", rr2.Code, rr2.Body.String())
	}
}

func TestOrgCreateMultiple(t *testing.T) {
	oh := newOrgHarness(t)

	// Create two orgs with different names
	name1 := fmt.Sprintf("e2e-multi-a-%s", randomSuffix())
	name2 := fmt.Sprintf("e2e-multi-b-%s", randomSuffix())

	rr1 := oh.orgRequest(t, http.MethodPost, "/v1/orgs", fmt.Sprintf(`{"name":%q}`, name1))
	if rr1.Code != http.StatusCreated {
		t.Fatalf("first create: expected 201, got %d: %s", rr1.Code, rr1.Body.String())
	}
	var org1 struct {
		ID           string `json:"id"`
		ZitadelOrgID string `json:"zitadel_org_id"`
	}
	json.NewDecoder(rr1.Body).Decode(&org1)

	rr2 := oh.orgRequest(t, http.MethodPost, "/v1/orgs", fmt.Sprintf(`{"name":%q}`, name2))
	if rr2.Code != http.StatusCreated {
		t.Fatalf("second create: expected 201, got %d: %s", rr2.Code, rr2.Body.String())
	}
	var org2 struct {
		ID           string `json:"id"`
		ZitadelOrgID string `json:"zitadel_org_id"`
	}
	json.NewDecoder(rr2.Body).Decode(&org2)

	if org1.ID == org2.ID {
		t.Error("two orgs should have different IDs")
	}
	if org1.ZitadelOrgID == org2.ZitadelOrgID {
		t.Error("two orgs should have different ZITADEL org IDs")
	}

	t.Cleanup(func() {
		oh.db.Where("id IN ?", []string{org1.ID, org2.ID}).Delete(&model.Org{})
	})
}

func randomSuffix() string {
	return uuid.New().String()[:8]
}
