package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/auth"
	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// --------------------------------------------------------------------------
// Test infrastructure
// --------------------------------------------------------------------------

type orgUpdateHarness struct {
	db     *gorm.DB
	router *chi.Mux
}

func newOrgUpdateHarness(t *testing.T) *orgUpdateHarness {
	t.Helper()
	db := connectTestDB(t)

	orgHandler := handler.NewOrgHandler(db)

	r := chi.NewRouter()
	r.Route("/v1/orgs", func(r chi.Router) {
		r.Use(middleware.ResolveOrgFromHeader(db))
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireOrgAdmin(db))
			r.Patch("/current", orgHandler.Update)
		})
	})

	return &orgUpdateHarness{db: db, router: r}
}

func (h *orgUpdateHarness) createOrg(t *testing.T, role string) (model.Org, model.User) {
	t.Helper()
	user := model.User{Email: "org-update-" + uuid.New().String()[:8] + "@test.com", Name: "Test"}
	if err := h.db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	org := model.Org{Name: "Org-" + uuid.New().String()[:8], Active: true}
	if err := h.db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	membership := model.OrgMembership{UserID: user.ID, OrgID: org.ID, Role: role}
	if err := h.db.Create(&membership).Error; err != nil {
		t.Fatalf("create membership: %v", err)
	}
	t.Cleanup(func() {
		h.db.Where("user_id = ?", user.ID).Delete(&model.OrgMembership{})
		h.db.Where("id = ?", org.ID).Delete(&model.Org{})
		h.db.Where("id = ?", user.ID).Delete(&model.User{})
	})
	return org, user
}

func (h *orgUpdateHarness) doPatch(t *testing.T, userID, orgID uuid.UUID, role string, body any) *httptest.ResponseRecorder {
	t.Helper()
	buf := new(bytes.Buffer)
	if body != nil {
		json.NewEncoder(buf).Encode(body)
	}
	req := httptest.NewRequest(http.MethodPatch, "/v1/orgs/current", buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-ID", orgID.String())
	req = middleware.WithAuthClaims(req, &auth.AuthClaims{
		UserID: userID.String(),
		OrgID:  orgID.String(),
		Role:   role,
	})

	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

// --------------------------------------------------------------------------
// Tests
// --------------------------------------------------------------------------

func TestOrgUpdate_NameAndLogoSucceed(t *testing.T) {
	h := newOrgUpdateHarness(t)
	org, user := h.createOrg(t, "admin")

	rr := h.doPatch(t, user.ID, org.ID, "admin", map[string]any{
		"name":     "Renamed Inc",
		"logo_url": "https://assets.usehiveloop.com/pub/o/" + org.ID.String() + "/logo.png",
	})

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d body=%s, want 200", rr.Code, rr.Body.String())
	}

	var got struct {
		Name    string `json:"name"`
		LogoURL string `json:"logo_url"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Name != "Renamed Inc" {
		t.Errorf("name: got %q, want %q", got.Name, "Renamed Inc")
	}
	if got.LogoURL == "" {
		t.Error("logo_url should be populated in response")
	}

	var reloaded model.Org
	if err := h.db.First(&reloaded, "id = ?", org.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Name != "Renamed Inc" {
		t.Errorf("db name: got %q", reloaded.Name)
	}
	if reloaded.LogoURL == "" {
		t.Error("db logo_url should be set")
	}
}

func TestOrgUpdate_LogoOnly(t *testing.T) {
	h := newOrgUpdateHarness(t)
	org, user := h.createOrg(t, "owner")

	rr := h.doPatch(t, user.ID, org.ID, "owner", map[string]any{
		"logo_url": "https://assets.usehiveloop.com/pub/o/abc/logo.png",
	})

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d body=%s, want 200", rr.Code, rr.Body.String())
	}

	var reloaded model.Org
	h.db.First(&reloaded, "id = ?", org.ID)
	if reloaded.Name != org.Name {
		t.Errorf("name shouldn't change: got %q, want %q", reloaded.Name, org.Name)
	}
	if reloaded.LogoURL == "" {
		t.Error("logo_url should be set")
	}
}

func TestOrgUpdate_EmptyLogoClears(t *testing.T) {
	h := newOrgUpdateHarness(t)
	org, user := h.createOrg(t, "admin")

	// Seed an existing logo.
	h.db.Model(&model.Org{}).Where("id = ?", org.ID).
		Update("logo_url", "https://assets.usehiveloop.com/pub/o/abc/old.png")

	emptyLogo := ""
	rr := h.doPatch(t, user.ID, org.ID, "admin", map[string]any{
		"logo_url": emptyLogo,
	})

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d body=%s, want 200", rr.Code, rr.Body.String())
	}

	var reloaded model.Org
	h.db.First(&reloaded, "id = ?", org.ID)
	if reloaded.LogoURL != "" {
		t.Errorf("logo_url should be cleared, got %q", reloaded.LogoURL)
	}
}

func TestOrgUpdate_RequiresOrgAdmin(t *testing.T) {
	h := newOrgUpdateHarness(t)
	org, user := h.createOrg(t, "member")

	rr := h.doPatch(t, user.ID, org.ID, "member", map[string]any{
		"name": "Hijacked",
	})

	if rr.Code != http.StatusForbidden {
		t.Errorf("status: got %d body=%s, want 403 for member", rr.Code, rr.Body.String())
	}

	var reloaded model.Org
	h.db.First(&reloaded, "id = ?", org.ID)
	if reloaded.Name != org.Name {
		t.Errorf("name should not change on a forbidden request")
	}
}

func TestOrgUpdate_RejectsEmptyName(t *testing.T) {
	h := newOrgUpdateHarness(t)
	org, user := h.createOrg(t, "admin")

	emptyName := "   "
	rr := h.doPatch(t, user.ID, org.ID, "admin", map[string]any{
		"name": emptyName,
	})

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want 400 for empty name", rr.Code)
	}
}

func TestOrgUpdate_NoFieldsReturns400(t *testing.T) {
	h := newOrgUpdateHarness(t)
	org, user := h.createOrg(t, "admin")

	rr := h.doPatch(t, user.ID, org.ID, "admin", map[string]any{})

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want 400 for empty body", rr.Code)
	}
}

func TestOrgUpdate_ResponseIncludesPlanInfo(t *testing.T) {
	h := newOrgUpdateHarness(t)
	org, user := h.createOrg(t, "admin")

	// Seed a plan row that matches the org's default plan slug ("free") and
	// confirm the patch response carries both plan_slug and plan_name.
	plan := model.Plan{Slug: "free", Name: "Free", PriceCents: 0, Currency: "USD", Active: true}
	if err := h.db.Where("slug = ?", plan.Slug).FirstOrCreate(&plan).Error; err != nil {
		t.Fatalf("seed plan: %v", err)
	}
	t.Cleanup(func() {
		// Only clean up if we created it — leave a pre-existing "free" plan alone.
	})

	rr := h.doPatch(t, user.ID, org.ID, "admin", map[string]any{
		"name": "With Plan Info",
	})

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d body=%s, want 200", rr.Code, rr.Body.String())
	}

	var got struct {
		Plan *struct {
			Slug           string `json:"slug"`
			Name           string `json:"name"`
			MonthlyCredits int64  `json:"monthly_credits"`
			PriceCents     int64  `json:"price_cents"`
			Currency       string `json:"currency"`
		} `json:"plan"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Plan == nil {
		t.Fatalf("plan should be populated, got nil")
	}
	if got.Plan.Slug != "free" {
		t.Errorf("plan.slug: got %q, want %q", got.Plan.Slug, "free")
	}
	if got.Plan.Name != "Free" {
		t.Errorf("plan.name: got %q, want %q", got.Plan.Name, "Free")
	}
	if got.Plan.Currency != "USD" {
		t.Errorf("plan.currency: got %q, want %q", got.Plan.Currency, "USD")
	}
}
