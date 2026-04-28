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
	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

type verifyHarness struct {
	db     *gorm.DB
	router *chi.Mux
}

func newVerifyHarness(t *testing.T) *verifyHarness {
	t.Helper()
	db := connectTestDB(t)
	billingHandler := handler.NewBillingHandler(db, billing.NewRegistry(), billing.NewCreditsService(db))

	r := chi.NewRouter()
	r.Route("/v1/billing", func(r chi.Router) {
		r.Use(middleware.ResolveOrgFromHeader(db))
		r.Post("/verify", billingHandler.Verify)
	})
	return &verifyHarness{db: db, router: r}
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

func (h *verifyHarness) seedActiveSub(t *testing.T, orgID, planID uuid.UUID) model.Subscription {
	t.Helper()
	sub := model.Subscription{
		OrgID:                  orgID,
		PlanID:                 planID,
		Provider:               "paystack",
		ExternalSubscriptionID: "SUB_verify_" + uuid.NewString()[:8],
		ExternalCustomerID:     "CUS_verify_" + uuid.NewString()[:8],
		Status:                 string(billing.StatusActive),
	}
	if err := h.db.Create(&sub).Error; err != nil {
		t.Fatalf("create subscription: %v", err)
	}
	return sub
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

func TestBillingVerify_FoundImmediate(t *testing.T) {
	h := newVerifyHarness(t)
	org, user := h.seedOrgWithMember(t)
	plan := h.seedPlan(t, "verify-pro-"+uuid.NewString()[:8])
	h.seedActiveSub(t, org.ID, plan.ID)

	rr := h.post(t, user.ID, org.ID, map[string]string{"plan_slug": plan.Slug})
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
}

func TestBillingVerify_FoundDuringPoll(t *testing.T) {
	h := newVerifyHarness(t)
	org, user := h.seedOrgWithMember(t)
	plan := h.seedPlan(t, "verify-poll-"+uuid.NewString()[:8])

	go func() {
		time.Sleep(700 * time.Millisecond)
		h.seedActiveSub(t, org.ID, plan.ID)
	}()

	start := time.Now()
	rr := h.post(t, user.ID, org.ID, map[string]string{"plan_slug": plan.Slug})
	elapsed := time.Since(start)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if elapsed >= 5*time.Second {
		t.Errorf("verify took %v — should have resolved well before the 5s timeout", elapsed)
	}
}

func TestBillingVerify_TimeoutWhenAbsent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping 5s timeout test in short mode")
	}
	h := newVerifyHarness(t)
	org, user := h.seedOrgWithMember(t)
	plan := h.seedPlan(t, "verify-absent-"+uuid.NewString()[:8])

	start := time.Now()
	rr := h.post(t, user.ID, org.ID, map[string]string{"plan_slug": plan.Slug})
	elapsed := time.Since(start)

	if rr.Code != http.StatusRequestTimeout {
		t.Fatalf("expected 408, got %d: %s", rr.Code, rr.Body.String())
	}
	if elapsed < 4*time.Second {
		t.Errorf("verify returned too quickly: %v (expected ~5s)", elapsed)
	}
	var resp map[string]string
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["status"] != "timeout" {
		t.Errorf("status = %q, want timeout", resp["status"])
	}
}

func TestBillingVerify_OtherPlanIgnored(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping 5s timeout test in short mode")
	}
	h := newVerifyHarness(t)
	org, user := h.seedOrgWithMember(t)
	other := h.seedPlan(t, "verify-other-"+uuid.NewString()[:8])
	requested := h.seedPlan(t, "verify-requested-"+uuid.NewString()[:8])
	// Active subscription on a different plan than the one being verified.
	h.seedActiveSub(t, org.ID, other.ID)

	rr := h.post(t, user.ID, org.ID, map[string]string{"plan_slug": requested.Slug})
	if rr.Code != http.StatusRequestTimeout {
		t.Fatalf("expected 408, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestBillingVerify_CanceledIgnored(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping 5s timeout test in short mode")
	}
	h := newVerifyHarness(t)
	org, user := h.seedOrgWithMember(t)
	plan := h.seedPlan(t, "verify-canceled-"+uuid.NewString()[:8])
	sub := h.seedActiveSub(t, org.ID, plan.ID)
	h.db.Model(&sub).Update("status", string(billing.StatusCanceled))

	rr := h.post(t, user.ID, org.ID, map[string]string{"plan_slug": plan.Slug})
	if rr.Code != http.StatusRequestTimeout {
		t.Fatalf("expected 408 for canceled subscription, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestBillingVerify_RequiresPlanSlug(t *testing.T) {
	h := newVerifyHarness(t)
	org, user := h.seedOrgWithMember(t)

	rr := h.post(t, user.ID, org.ID, map[string]string{})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}
