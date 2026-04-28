package handler_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/auth"
	"github.com/usehiveloop/hiveloop/internal/billing"
	"github.com/usehiveloop/hiveloop/internal/billing/fake"
	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

type verifyHarness struct {
	db       *gorm.DB
	router   *chi.Mux
	provider *fake.Provider
}

func newVerifyHarness(t *testing.T) *verifyHarness {
	t.Helper()
	db := connectTestDB(t)
	registry := billing.NewRegistry()
	provider := fake.New("paystack")
	registry.Register(provider)
	billingHandler := handler.NewBillingHandler(db, registry, billing.NewCreditsService(db))

	r := chi.NewRouter()
	r.Route("/v1/billing", func(r chi.Router) {
		r.Use(middleware.ResolveOrgFromHeader(db))
		r.Post("/verify", billingHandler.Verify)
	})
	return &verifyHarness{db: db, router: r, provider: provider}
}

func (h *verifyHarness) seedOrgWithMember(t *testing.T) (model.Org, model.User) {
	t.Helper()
	user := model.User{Email: "verify-" + uuid.NewString()[:8] + "@test.com", Name: "Verify"}
	if err := h.db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	org := model.Org{Name: "Verify Org " + uuid.NewString()[:8], Active: true}
	if err := h.db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	if err := h.db.Create(&model.OrgMembership{UserID: user.ID, OrgID: org.ID, Role: "owner"}).Error; err != nil {
		t.Fatalf("create membership: %v", err)
	}
	t.Cleanup(func() {
		h.db.Where("org_id = ?", org.ID).Delete(&model.Subscription{})
		h.db.Where("org_id = ?", org.ID).Delete(&model.OrgMembership{})
		h.db.Where("id = ?", org.ID).Delete(&model.Org{})
		h.db.Where("id = ?", user.ID).Delete(&model.User{})
	})
	return org, user
}

func (h *verifyHarness) seedPlan(t *testing.T, slug string) model.Plan {
	t.Helper()
	plan := model.Plan{
		ID:             uuid.New(),
		Slug:           slug,
		Name:           "Plan " + slug,
		MonthlyCredits: 1000,
	}
	if err := h.db.Create(&plan).Error; err != nil {
		t.Fatalf("create plan: %v", err)
	}
	t.Cleanup(func() { h.db.Where("id = ?", plan.ID).Delete(&model.Plan{}) })
	return plan
}

func (h *verifyHarness) post(t *testing.T, userID, orgID uuid.UUID, body any) *httptest.ResponseRecorder {
	t.Helper()
	buf := new(bytes.Buffer)
	json.NewEncoder(buf).Encode(body)
	req := httptest.NewRequest("POST", "/v1/billing/verify", buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-ID", orgID.String())
	req = middleware.WithAuthClaims(req, &auth.AuthClaims{
		UserID: userID.String(),
		OrgID:  orgID.String(),
		Role:   "owner",
	})
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

func TestBillingVerify_CreatesSubscription(t *testing.T) {
	h := newVerifyHarness(t)
	org, user := h.seedOrgWithMember(t)
	plan := h.seedPlan(t, "verify-create-"+uuid.NewString()[:8])

	h.provider.NextResolveResult = &billing.ResolveCheckoutResult{
		Status:                 billing.StatusActive,
		PlanSlug:               plan.Slug,
		ExternalSubscriptionID: "SUB_create",
		ExternalCustomerID:     "CUS_create",
		ChargeReference:        "ref_create",
		ChargeAmount:           500000,
		CardLast4:              "4242",
		CardBrand:              "visa",
		AuthorizationCode:      "AUTH_create",
	}

	rr := h.post(t, user.ID, org.ID, map[string]string{"reference": "ref_create"})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]string
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["status"] != "active" {
		t.Errorf("status = %q, want active", resp["status"])
	}
	if resp["plan_slug"] != plan.Slug {
		t.Errorf("plan_slug = %q, want %q", resp["plan_slug"], plan.Slug)
	}

	var sub model.Subscription
	if err := h.db.Where("org_id = ? AND plan_id = ?", org.ID, plan.ID).First(&sub).Error; err != nil {
		t.Fatalf("expected subscription row, got: %v", err)
	}
	if sub.LastChargeReference != "ref_create" {
		t.Errorf("LastChargeReference = %q, want %q", sub.LastChargeReference, "ref_create")
	}
	if sub.AuthorizationCode != "AUTH_create" {
		t.Errorf("AuthorizationCode = %q, want %q", sub.AuthorizationCode, "AUTH_create")
	}

	var orgRow model.Org
	h.db.First(&orgRow, "id = ?", org.ID)
	if orgRow.PlanSlug != plan.Slug {
		t.Errorf("org.plan_slug = %q, want %q", orgRow.PlanSlug, plan.Slug)
	}
}

func TestBillingVerify_UpdatesExistingSubscription(t *testing.T) {
	h := newVerifyHarness(t)
	org, user := h.seedOrgWithMember(t)
	plan := h.seedPlan(t, "verify-update-"+uuid.NewString()[:8])

	existing := model.Subscription{
		OrgID:                  org.ID,
		PlanID:                 plan.ID,
		Provider:               "paystack",
		ExternalSubscriptionID: "SUB_old",
		ExternalCustomerID:     "CUS_old",
		Status:                 string(billing.StatusActive),
		LastChargeReference:    "ref_old",
		CardLast4:              "0000",
	}
	if err := h.db.Create(&existing).Error; err != nil {
		t.Fatalf("seed sub: %v", err)
	}

	h.provider.NextResolveResult = &billing.ResolveCheckoutResult{
		Status:                 billing.StatusActive,
		PlanSlug:               plan.Slug,
		ExternalSubscriptionID: "SUB_new",
		ExternalCustomerID:     "CUS_new",
		ChargeReference:        "ref_new",
		ChargeAmount:           700000,
		CardLast4:              "4242",
	}

	rr := h.post(t, user.ID, org.ID, map[string]string{"reference": "ref_new"})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var got model.Subscription
	if err := h.db.First(&got, "id = ?", existing.ID).Error; err != nil {
		t.Fatalf("reload sub: %v", err)
	}
	if got.LastChargeReference != "ref_new" {
		t.Errorf("LastChargeReference = %q, want %q", got.LastChargeReference, "ref_new")
	}
	if got.ExternalSubscriptionID != "SUB_new" {
		t.Errorf("ExternalSubscriptionID = %q, want %q", got.ExternalSubscriptionID, "SUB_new")
	}
	if got.CardLast4 != "4242" {
		t.Errorf("CardLast4 = %q, want %q", got.CardLast4, "4242")
	}
}

func TestBillingVerify_NonSuccessReturnsStatus(t *testing.T) {
	h := newVerifyHarness(t)
	org, user := h.seedOrgWithMember(t)

	h.provider.NextResolveResult = &billing.ResolveCheckoutResult{Status: billing.StatusPastDue}

	rr := h.post(t, user.ID, org.ID, map[string]string{"reference": "ref_pending"})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]string
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["status"] != "past_due" {
		t.Errorf("status = %q, want past_due", resp["status"])
	}

	var count int64
	h.db.Model(&model.Subscription{}).Where("org_id = ?", org.ID).Count(&count)
	if count != 0 {
		t.Errorf("expected 0 subscription rows, got %d", count)
	}
}

func TestBillingVerify_ProviderErrorReturns502(t *testing.T) {
	h := newVerifyHarness(t)
	org, user := h.seedOrgWithMember(t)

	h.provider.NextResolveError = errors.New("paystack: boom")

	rr := h.post(t, user.ID, org.ID, map[string]string{"reference": "ref_boom"})
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestBillingVerify_RequiresReference(t *testing.T) {
	h := newVerifyHarness(t)
	org, user := h.seedOrgWithMember(t)

	rr := h.post(t, user.ID, org.ID, map[string]string{})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}
