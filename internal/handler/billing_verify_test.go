package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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
		h.db.Where("org_id = ?", org.ID).Delete(&model.CreditLedgerEntry{})
		h.db.Where("id = ?", org.ID).Delete(&model.Org{})
		h.db.Where("id = ?", user.ID).Delete(&model.User{})
	})
	return org, user
}

func (h *verifyHarness) seedPlan(t *testing.T, slug string, priceCents int64, monthlyCredits int64, currency string) model.Plan {
	t.Helper()
	plan := model.Plan{
		ID:             uuid.New(),
		Slug:           slug,
		Name:           "Plan " + slug,
		PriceCents:     priceCents,
		Currency:       currency,
		MonthlyCredits: monthlyCredits,
		Active:         true,
	}
	if err := h.db.Create(&plan).Error; err != nil {
		t.Fatalf("create plan: %v", err)
	}
	t.Cleanup(func() {
		// Drop any subs that pointed at this plan first to avoid FK violations.
		h.db.Where("plan_id = ?", plan.ID).Delete(&model.Subscription{})
		h.db.Where("id = ?", plan.ID).Delete(&model.Plan{})
	})
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

func paidAt() *time.Time {
	t := time.Now().UTC().Truncate(time.Second)
	return &t
}

func TestBillingVerify_CreatesSubscriptionWithMatchingAmount(t *testing.T) {
	h := newVerifyHarness(t)
	org, user := h.seedOrgWithMember(t)
	plan := h.seedPlan(t, "verify-pro-"+uuid.NewString()[:8], 2_000_000, 5_000, "NGN")

	h.provider.NextResolveResult = &billing.ResolveCheckoutResult{
		Status:             billing.StatusActive,
		PaidAmountMinor:    plan.PriceCents,
		Currency:           plan.Currency,
		Reference:          "ref_ok",
		ExternalCustomerID: "CUS_ok",
		PaidAt:             paidAt(),
		Metadata:           map[string]string{"plan_slug": plan.Slug},
		PaymentMethod: billing.PaymentMethod{
			AuthorizationCode: "AUTH_ok",
			Channel:           billing.ChannelCard,
			CardLast4:         "4242",
		},
	}

	rr := h.post(t, user.ID, org.ID, map[string]string{"reference": "ref_ok"})
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
		t.Fatalf("expected subscription, got: %v", err)
	}
	if sub.AuthorizationCode != "AUTH_ok" {
		t.Errorf("AuthorizationCode = %q", sub.AuthorizationCode)
	}
	if sub.PaymentChannel != "card" {
		t.Errorf("PaymentChannel = %q, want card", sub.PaymentChannel)
	}
	if sub.CardLast4 != "4242" {
		t.Errorf("CardLast4 = %q", sub.CardLast4)
	}
}

func TestBillingVerify_AmountMismatchReturns402(t *testing.T) {
	h := newVerifyHarness(t)
	org, user := h.seedOrgWithMember(t)
	plan := h.seedPlan(t, "verify-mismatch-"+uuid.NewString()[:8], 2_000_000, 5_000, "NGN")

	h.provider.NextResolveResult = &billing.ResolveCheckoutResult{
		Status:          billing.StatusActive,
		PaidAmountMinor: plan.PriceCents - 1, // off by one
		Currency:        plan.Currency,
		Reference:       "ref_short",
		PaidAt:          paidAt(),
		Metadata:        map[string]string{"plan_slug": plan.Slug},
		PaymentMethod:   billing.PaymentMethod{Channel: billing.ChannelCard, AuthorizationCode: "AUTH"},
	}

	rr := h.post(t, user.ID, org.ID, map[string]string{"reference": "ref_short"})
	if rr.Code != http.StatusPaymentRequired {
		t.Fatalf("expected 402, got %d: %s", rr.Code, rr.Body.String())
	}

	// Subscription not created.
	var count int64
	h.db.Model(&model.Subscription{}).Where("org_id = ?", org.ID).Count(&count)
	if count != 0 {
		t.Errorf("expected 0 subscriptions on amount mismatch, got %d", count)
	}
}

func TestBillingVerify_UnsupportedChannelReturns400(t *testing.T) {
	h := newVerifyHarness(t)
	org, user := h.seedOrgWithMember(t)
	plan := h.seedPlan(t, "verify-channel-"+uuid.NewString()[:8], 2_000_000, 5_000, "NGN")

	h.provider.NextResolveResult = &billing.ResolveCheckoutResult{
		Status:          billing.StatusActive,
		PaidAmountMinor: plan.PriceCents,
		Currency:        plan.Currency,
		Reference:       "ref_ussd",
		PaidAt:          paidAt(),
		Metadata:        map[string]string{"plan_slug": plan.Slug},
		PaymentMethod:   billing.PaymentMethod{Channel: billing.PaymentChannel("ussd"), AuthorizationCode: "AUTH"},
	}

	rr := h.post(t, user.ID, org.ID, map[string]string{"reference": "ref_ussd"})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestBillingVerify_NoMetadataReturns400(t *testing.T) {
	h := newVerifyHarness(t)
	org, user := h.seedOrgWithMember(t)
	h.seedPlan(t, "no-meta-"+uuid.NewString()[:8], 2_000_000, 5_000, "NGN")

	h.provider.NextResolveResult = &billing.ResolveCheckoutResult{
		Status:          billing.StatusActive,
		PaidAmountMinor: 2_000_000,
		Currency:        "NGN",
		Reference:       "ref_no_meta",
		PaymentMethod:   billing.PaymentMethod{Channel: billing.ChannelCard, AuthorizationCode: "AUTH"},
	}

	rr := h.post(t, user.ID, org.ID, map[string]string{"reference": "ref_no_meta"})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
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
		t.Errorf("expected 0 subscriptions, got %d", count)
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

func TestBillingVerify_GrantsMonthlyCredits(t *testing.T) {
	h := newVerifyHarness(t)
	org, user := h.seedOrgWithMember(t)
	plan := h.seedPlan(t, "verify-credits-"+uuid.NewString()[:8], 2_000_000, 5_000, "NGN")

	h.provider.NextResolveResult = &billing.ResolveCheckoutResult{
		Status:             billing.StatusActive,
		PaidAmountMinor:    plan.PriceCents,
		Currency:           plan.Currency,
		Reference:          "ref_grant",
		ExternalCustomerID: "CUS_grant",
		PaidAt:             paidAt(),
		Metadata:           map[string]string{"plan_slug": plan.Slug},
		PaymentMethod:      billing.PaymentMethod{Channel: billing.ChannelCard, AuthorizationCode: "AUTH"},
	}

	rr := h.post(t, user.ID, org.ID, map[string]string{"reference": "ref_grant"})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	credits := billing.NewCreditsService(h.db)
	bal, err := credits.Balance(org.ID)
	if err != nil {
		t.Fatalf("balance: %v", err)
	}
	if bal != plan.MonthlyCredits {
		t.Errorf("balance = %d, want %d (monthly credits)", bal, plan.MonthlyCredits)
	}
}
