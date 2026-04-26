package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// --------------------------------------------------------------------------
// Test infrastructure
// --------------------------------------------------------------------------

type plansListHarness struct {
	db     *gorm.DB
	router *chi.Mux
}

func newPlansListHarness(t *testing.T) *plansListHarness {
	t.Helper()
	db := connectTestDB(t)
	plansHandler := handler.NewPlansHandler(db)
	r := chi.NewRouter()
	r.Get("/v1/plans", plansHandler.List)
	return &plansListHarness{db: db, router: r}
}

func (h *plansListHarness) seedPlan(t *testing.T, slug string, name string, priceCents int64, active bool) {
	t.Helper()
	plan := model.Plan{
		Slug:           slug,
		Name:           name,
		MonthlyCredits: priceCents * 10,
		WelcomeCredits: 100,
		PriceCents:     priceCents,
		Currency:       "USD",
		Active:         active,
	}
	if err := h.db.Where("slug = ?", slug).Delete(&model.Plan{}).Error; err != nil {
		t.Fatalf("clean prior plan: %v", err)
	}
	if err := h.db.Create(&plan).Error; err != nil {
		t.Fatalf("seed plan %s: %v", slug, err)
	}
	// GORM omits zero-value bools when the column has a SQL default, so an
	// explicit Update is the only way to land active=false on insert.
	if !active {
		if err := h.db.Model(&model.Plan{}).Where("id = ?", plan.ID).
			Update("active", false).Error; err != nil {
			t.Fatalf("force inactive on %s: %v", slug, err)
		}
	}
	t.Cleanup(func() {
		h.db.Where("slug = ?", slug).Delete(&model.Plan{})
	})
}

func (h *plansListHarness) doGet(t *testing.T) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/plans", nil)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

// --------------------------------------------------------------------------
// Tests
// --------------------------------------------------------------------------

func TestPlansList_NoAuthRequired(t *testing.T) {
	h := newPlansListHarness(t)
	// No auth middleware on the route — request goes through cleanly.
	rr := h.doGet(t)
	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 without auth", rr.Code)
	}
}

func TestPlansList_ReturnsActiveOrderedByPrice(t *testing.T) {
	h := newPlansListHarness(t)
	suffix := uuid.New().String()[:8]
	h.seedPlan(t, "test-pro-"+suffix, "Pro "+suffix, 4900, true)
	h.seedPlan(t, "test-business-"+suffix, "Business "+suffix, 19900, true)
	h.seedPlan(t, "test-free-"+suffix, "Free "+suffix, 0, true)
	h.seedPlan(t, "test-archived-"+suffix, "Archived "+suffix, 9900, false)

	rr := h.doGet(t)
	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d body=%s, want 200", rr.Code, rr.Body.String())
	}

	var got []struct {
		Slug           string `json:"slug"`
		Name           string `json:"name"`
		MonthlyCredits int64  `json:"monthly_credits"`
		WelcomeCredits int64  `json:"welcome_credits"`
		PriceCents     int64  `json:"price_cents"`
		Currency       string `json:"currency"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Filter to just our seeded slugs to ignore any pre-existing plans in the DB.
	var ours []string
	for _, plan := range got {
		switch plan.Slug {
		case "test-pro-" + suffix, "test-business-" + suffix, "test-free-" + suffix, "test-archived-" + suffix:
			ours = append(ours, plan.Slug)
			if plan.Currency != "USD" {
				t.Errorf("plan %s: currency got %q, want USD", plan.Slug, plan.Currency)
			}
			if plan.Name == "" {
				t.Errorf("plan %s: name should not be empty", plan.Slug)
			}
		}
	}

	want := []string{"test-free-" + suffix, "test-pro-" + suffix, "test-business-" + suffix}
	if len(ours) != len(want) {
		t.Fatalf("got %d of our plans (slugs=%v), want %d (excluding archived)", len(ours), ours, len(want))
	}

	// The endpoint orders by price_cents ASC — our three active plans should
	// come back in price order regardless of seed order.
	indexOf := func(slug string) int {
		for i, s := range ours {
			if s == slug {
				return i
			}
		}
		return -1
	}
	if !sort.IntsAreSorted([]int{indexOf("test-free-" + suffix), indexOf("test-pro-" + suffix), indexOf("test-business-" + suffix)}) {
		t.Errorf("plans not in ascending price order: %v", ours)
	}

	// Confirm the archived plan is NOT in the response.
	for _, plan := range got {
		if plan.Slug == "test-archived-"+suffix {
			t.Errorf("inactive plan %q should be excluded from /v1/plans", plan.Slug)
		}
	}
}

func TestPlansList_EmptyCatalog_Returns200(t *testing.T) {
	h := newPlansListHarness(t)
	// We don't actively delete other plans; just confirm the route never 5xxs
	// when there are no matching active rows in some table state.
	rr := h.doGet(t)
	if rr.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200 even when catalog might be empty", rr.Code)
	}
}
