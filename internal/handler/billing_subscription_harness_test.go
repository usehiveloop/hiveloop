package handler_test

import (
	"bytes"
	"encoding/json"
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

type subHarness struct {
	db       *gorm.DB
	router   *chi.Mux
	provider *fake.Provider
}

func newSubHarness(t *testing.T) *subHarness {
	t.Helper()
	db := connectTestDB(t)
	registry := billing.NewRegistry()
	provider := fake.New("paystack")
	registry.Register(provider)

	subHandler := handler.NewSubscriptionHandler(db, registry, billing.NewCreditsService(db))

	r := chi.NewRouter()
	r.Route("/v1/billing/subscription", func(r chi.Router) {
		r.Use(middleware.ResolveOrgFromHeader(db))
		r.Post("/preview-change", subHandler.PreviewChange)
		r.Post("/apply-change", subHandler.ApplyChange)
		r.Post("/cancel", subHandler.Cancel)
		r.Post("/resume", subHandler.Resume)
	})
	return &subHarness{db: db, router: r, provider: provider}
}

type subFixture struct {
	org      model.Org
	user     model.User
	planFrom model.Plan
	planTo   model.Plan
	sub      model.Subscription
}

// seedFixture creates an org, an owner, two plans, and an active
// subscription on planFrom. The subscription period is centered on now()
// (15 days remaining of a 30-day cycle) so tests can assert prorated
// amounts that round to predictable halves.
func (h *subHarness) seedFixture(t *testing.T, fromPriceCents, toPriceCents int64) subFixture {
	t.Helper()
	user := model.User{Email: "sub-" + uuid.NewString()[:8] + "@test.com", Name: "Sub"}
	if err := h.db.Create(&user).Error; err != nil {
		t.Fatalf("user: %v", err)
	}
	org := model.Org{ID: uuid.New(), Name: "Org-" + uuid.NewString()[:8], Active: true}
	if err := h.db.Create(&org).Error; err != nil {
		t.Fatalf("org: %v", err)
	}
	if err := h.db.Create(&model.OrgMembership{UserID: user.ID, OrgID: org.ID, Role: "owner"}).Error; err != nil {
		t.Fatalf("membership: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	suffix := uuid.NewString()[:8]
	planFrom := model.Plan{ID: uuid.New(), Slug: "from-" + suffix, Name: "From", PriceCents: fromPriceCents, Currency: "NGN", MonthlyCredits: 1_000, Active: true}
	planTo := model.Plan{ID: uuid.New(), Slug: "to-" + suffix, Name: "To", PriceCents: toPriceCents, Currency: "NGN", MonthlyCredits: 5_000, Active: true}
	if err := h.db.Create(&planFrom).Error; err != nil {
		t.Fatalf("planFrom: %v", err)
	}
	if err := h.db.Create(&planTo).Error; err != nil {
		t.Fatalf("planTo: %v", err)
	}

	sub := model.Subscription{
		OrgID:              org.ID,
		PlanID:             planFrom.ID,
		Provider:           "paystack",
		ExternalCustomerID: "CUS_test",
		Status:             string(billing.StatusActive),
		CurrentPeriodStart: now.Add(-15 * 24 * time.Hour),
		CurrentPeriodEnd:   now.Add(15 * 24 * time.Hour),
		AuthorizationCode:  "AUTH_seed",
		PaymentChannel:     "card",
	}
	if err := h.db.Create(&sub).Error; err != nil {
		t.Fatalf("sub: %v", err)
	}

	t.Cleanup(func() {
		h.db.Where("org_id = ?", org.ID).Delete(&model.SubscriptionChangeQuote{})
		h.db.Where("org_id = ?", org.ID).Delete(&model.Subscription{})
		h.db.Where("org_id = ?", org.ID).Delete(&model.OrgMembership{})
		h.db.Where("org_id = ?", org.ID).Delete(&model.CreditLedgerEntry{})
		h.db.Where("id = ?", planFrom.ID).Delete(&model.Plan{})
		h.db.Where("id = ?", planTo.ID).Delete(&model.Plan{})
		h.db.Where("id = ?", org.ID).Delete(&model.Org{})
		h.db.Where("id = ?", user.ID).Delete(&model.User{})
	})

	return subFixture{org: org, user: user, planFrom: planFrom, planTo: planTo, sub: sub}
}

func (h *subHarness) post(t *testing.T, path string, userID, orgID uuid.UUID, body any) *httptest.ResponseRecorder {
	t.Helper()
	buf := new(bytes.Buffer)
	if body != nil {
		if err := json.NewEncoder(buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest("POST", path, buf)
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
