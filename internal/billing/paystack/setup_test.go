package paystack

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/usehiveloop/hiveloop/internal/billing"
	"github.com/usehiveloop/hiveloop/internal/billing/setup"
)

// startReconciler wires a Reconciler up against a paystackStub (defined in
// setup_stub_test.go).
func startReconciler(t *testing.T, s *paystackStub, opts ...ReconcilerOption) (*Reconciler, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)

	// Disable rate limiting so tests don't take a second each.
	opts = append([]ReconcilerOption{WithRateLimit(nil)}, opts...)
	r := NewReconciler("sk_test_stub", opts...)
	r.client.baseURL = srv.URL
	return r, srv
}

func sixSpecs() []setup.PlanSpec {
	return []setup.PlanSpec{
		{Slug: "starter", Name: "Starter (Monthly)", AmountMinor: 1_350_000, Currency: "NGN", Cycle: billing.CycleMonthly, Description: "Starter"},
		{Slug: "starter", Name: "Starter (Annual)", AmountMinor: 12_900_000, Currency: "NGN", Cycle: billing.CycleAnnual, Description: "Starter"},
		{Slug: "pro", Name: "Pro (Monthly)", AmountMinor: 5_850_000, Currency: "NGN", Cycle: billing.CycleMonthly, Description: "Pro"},
		{Slug: "pro", Name: "Pro (Annual)", AmountMinor: 56_100_000, Currency: "NGN", Cycle: billing.CycleAnnual, Description: "Pro"},
		{Slug: "business", Name: "Business (Monthly)", AmountMinor: 14_850_000, Currency: "NGN", Cycle: billing.CycleMonthly, Description: "Business"},
		{Slug: "business", Name: "Business (Annual)", AmountMinor: 142_500_000, Currency: "NGN", Cycle: billing.CycleAnnual, Description: "Business"},
	}
}

func TestReconciler_FreshAccount_CreatesAllSixPlans(t *testing.T) {
	s := newStub()
	r, _ := startReconciler(t, s)

	resolved, err := r.Reconcile(t.Context(), sixSpecs())
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if len(resolved) != 6 {
		t.Fatalf("got %d resolved, want 6", len(resolved))
	}
	for _, rp := range resolved {
		if rp.Action != setup.ActionCreate {
			t.Errorf("%v: action %s, want create", rp.Key, rp.Action)
		}
		if !strings.HasPrefix(rp.PlanCode, "PLN_new") {
			t.Errorf("%v: plan_code %q missing PLN_new prefix", rp.Key, rp.PlanCode)
		}
	}
	if s.createCalls != 6 {
		t.Errorf("stub saw %d create calls, want 6", s.createCalls)
	}
	if s.updateCalls != 0 {
		t.Errorf("stub saw %d update calls on fresh account, want 0", s.updateCalls)
	}
}

func TestReconciler_RerunIsIdempotent_NoMutations(t *testing.T) {
	s := newStub()
	r, _ := startReconciler(t, s)

	if _, err := r.Reconcile(t.Context(), sixSpecs()); err != nil {
		t.Fatalf("first Reconcile: %v", err)
	}
	createsAfterFirst := s.createCalls
	updatesAfterFirst := s.updateCalls

	resolved, err := r.Reconcile(t.Context(), sixSpecs())
	if err != nil {
		t.Fatalf("second Reconcile: %v", err)
	}
	for _, rp := range resolved {
		if rp.Action != setup.ActionNoOp {
			t.Errorf("%v: second run action %s, want noop", rp.Key, rp.Action)
		}
	}
	if s.createCalls != createsAfterFirst {
		t.Errorf("second run did %d extra creates, want 0", s.createCalls-createsAfterFirst)
	}
	if s.updateCalls != updatesAfterFirst {
		t.Errorf("second run did %d extra updates, want 0", s.updateCalls-updatesAfterFirst)
	}
}

func TestReconciler_DriftTriggersUpdate(t *testing.T) {
	s := newStub()
	s.Seed(paystackPlan{
		PlanCode: "PLN_pro_m_old", Name: "Pro (Monthly)",
		Amount: 5_000_000, Interval: "monthly", Currency: "NGN", Description: "Pro",
	})
	r, _ := startReconciler(t, s)

	resolved, err := r.Reconcile(t.Context(), []setup.PlanSpec{
		{Slug: "pro", Name: "Pro (Monthly)", AmountMinor: 5_850_000, Currency: "NGN", Cycle: billing.CycleMonthly, Description: "Pro"},
	})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if resolved[0].Action != setup.ActionUpdate {
		t.Errorf("action = %s, want update", resolved[0].Action)
	}
	if resolved[0].PlanCode != "PLN_pro_m_old" {
		t.Errorf("plan_code = %q, want reuse of existing PLN_pro_m_old", resolved[0].PlanCode)
	}
	if s.updateCalls != 1 || s.createCalls != 0 {
		t.Errorf("create=%d update=%d, want create=0 update=1", s.createCalls, s.updateCalls)
	}
	if s.plans["PLN_pro_m_old"].Amount != 5_850_000 {
		t.Errorf("amount after update = %d, want 5850000", s.plans["PLN_pro_m_old"].Amount)
	}
}

func TestReconciler_DryRun_NoMutationsIssued(t *testing.T) {
	s := newStub()
	r, _ := startReconciler(t, s, WithDryRun())

	resolved, err := r.Reconcile(t.Context(), sixSpecs())
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if len(resolved) != 6 {
		t.Fatalf("got %d resolved, want 6", len(resolved))
	}
	for _, rp := range resolved {
		if rp.PlanCode != "(dry-run)" {
			t.Errorf("dry-run plan_code = %q, want (dry-run)", rp.PlanCode)
		}
	}
	if s.createCalls != 0 || s.updateCalls != 0 {
		t.Errorf("dry-run issued %d creates + %d updates, want 0 each", s.createCalls, s.updateCalls)
	}
}

func TestReconciler_Paginates(t *testing.T) {
	s := newStub()
	s.forcePage = 2
	for i := 1; i <= 5; i++ {
		s.Seed(paystackPlan{
			Name: "Filler " + strconv.Itoa(i), Amount: int64(i * 1000),
			Interval: "monthly", Currency: "NGN",
		})
	}
	r, _ := startReconciler(t, s)

	resolved, err := r.Reconcile(t.Context(), []setup.PlanSpec{
		{Slug: "x", Name: "Filler 5", AmountMinor: 5000, Currency: "NGN", Cycle: billing.CycleMonthly},
	})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if resolved[0].Action != setup.ActionNoOp {
		t.Errorf("after paginate, expected noop, got %s — pagination might be dropping plans on later pages", resolved[0].Action)
	}
}

func TestReconciler_IgnoresUnrelatedExistingPlans(t *testing.T) {
	s := newStub()
	s.Seed(paystackPlan{Name: "Some Legacy Thing", Amount: 99_00, Interval: "weekly", Currency: "NGN"})
	s.Seed(paystackPlan{Name: "Another Legacy", Amount: 42, Interval: "quarterly", Currency: "NGN"})
	r, _ := startReconciler(t, s)

	resolved, err := r.Reconcile(t.Context(), sixSpecs())
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if s.createCalls != 6 {
		t.Errorf("createCalls = %d, want 6", s.createCalls)
	}
	if s.updateCalls != 0 {
		t.Errorf("updateCalls = %d, want 0 (legacies stay put)", s.updateCalls)
	}
	if len(s.plans) != 8 {
		t.Errorf("stub ended with %d plans, want 8 (2 legacy + 6 new)", len(s.plans))
	}
	for _, rp := range resolved {
		if rp.Action != setup.ActionCreate {
			t.Errorf("%v: action %s, want create", rp.Key, rp.Action)
		}
	}
}

// TestReconciler_UpdateAlwaysSetsNoExistingSubUpdate intercepts the PUT body
// to assert update_existing_subscriptions=false. This is the single most
// important safety invariant — if this flag ever slips to true, a pricing
// change would silently reprice every active subscriber on their next cycle.
func TestReconciler_UpdateAlwaysSetsNoExistingSubUpdate(t *testing.T) {
	var gotBody stubUpdateBody
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/plan/") {
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &gotBody)
			writeJSON(w, http.StatusOK, map[string]any{
				"status": true,
				"data":   paystackPlan{PlanCode: strings.TrimPrefix(r.URL.Path, "/plan/")},
			})
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/plan" {
			writeJSON(w, http.StatusOK, map[string]any{
				"status": true,
				"data": []paystackPlan{{
					PlanCode: "PLN_pro_m", Name: "Pro (Monthly)",
					Amount: 1, Interval: "monthly", Currency: "NGN",
				}},
				"meta": map[string]int{"page": 1, "pageCount": 1, "total": 1},
			})
			return
		}
		http.NotFound(w, r)
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	r := NewReconciler("sk_test", WithRateLimit(nil))
	r.client.baseURL = srv.URL

	_, err := r.Reconcile(t.Context(), []setup.PlanSpec{
		{Slug: "pro", Name: "Pro (Monthly)", AmountMinor: 5_850_000, Currency: "NGN", Cycle: billing.CycleMonthly, Description: "Pro"},
	})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if gotBody.UpdateExistingSubscriptions == nil {
		t.Fatal("update_existing_subscriptions missing from request body")
	}
	if *gotBody.UpdateExistingSubscriptions {
		t.Fatal("update_existing_subscriptions = true — MUST be false, existing subscribers must never silently reprice")
	}
}
