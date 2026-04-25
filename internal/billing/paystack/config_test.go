package paystack

import (
	"errors"
	"testing"

	"github.com/usehiveloop/hiveloop/internal/billing"
)

func sampleRegistry() PlanRegistry {
	return PlanRegistry{
		"starter": {
			"NGN": {Monthly: "PLN_starter_ngn_m", Annual: "PLN_starter_ngn_a"},
		},
		"pro": {
			"NGN": {Monthly: "PLN_pro_ngn_m", Annual: "PLN_pro_ngn_a"},
		},
	}
}

func TestPlanRegistry_LookupHappyPath(t *testing.T) {
	r := sampleRegistry()

	code, err := r.Lookup("pro", "NGN", billing.CycleMonthly)
	if err != nil {
		t.Fatalf("monthly lookup: %v", err)
	}
	if code != "PLN_pro_ngn_m" {
		t.Fatalf("monthly code = %q, want PLN_pro_ngn_m", code)
	}

	code, err = r.Lookup("pro", "ngn", billing.CycleAnnual) // lowercase currency should normalise
	if err != nil {
		t.Fatalf("annual lookup: %v", err)
	}
	if code != "PLN_pro_ngn_a" {
		t.Fatalf("annual code = %q, want PLN_pro_ngn_a", code)
	}
}

func TestPlanRegistry_UnknownSlug(t *testing.T) {
	r := sampleRegistry()
	_, err := r.Lookup("enterprise", "NGN", billing.CycleMonthly)
	if !errors.Is(err, billing.ErrUnknownPlan) {
		t.Fatalf("expected ErrUnknownPlan, got %v", err)
	}
}

func TestPlanRegistry_UnsupportedCurrency(t *testing.T) {
	r := sampleRegistry()
	_, err := r.Lookup("pro", "USD", billing.CycleMonthly)
	if !errors.Is(err, billing.ErrUnsupportedCurrency) {
		t.Fatalf("expected ErrUnsupportedCurrency, got %v", err)
	}
}

func TestPlanRegistry_UnsupportedCycle(t *testing.T) {
	r := sampleRegistry()
	_, err := r.Lookup("pro", "NGN", billing.Cycle("forever"))
	if err == nil {
		t.Fatal("expected error for unsupported cycle")
	}
}

func TestPlanRegistry_BlankCodeCountsAsUnknown(t *testing.T) {
	r := PlanRegistry{
		"starter": {"NGN": {Monthly: "PLN_x"}}, // annual blank
	}
	_, err := r.Lookup("starter", "NGN", billing.CycleAnnual)
	if !errors.Is(err, billing.ErrUnknownPlan) {
		t.Fatalf("expected ErrUnknownPlan for blank annual code, got %v", err)
	}
}

func TestPlanRegistry_ReverseIndex(t *testing.T) {
	r := sampleRegistry()
	idx := r.reverseIndex()

	cases := map[string]string{
		"PLN_pro_ngn_m":     "pro",
		"PLN_pro_ngn_a":     "pro",
		"PLN_starter_ngn_m": "starter",
	}
	for code, wantSlug := range cases {
		if gotSlug := idx[code]; gotSlug != wantSlug {
			t.Errorf("reverseIndex[%q] = %q, want %q", code, gotSlug, wantSlug)
		}
	}
	if len(idx) != 4 {
		t.Errorf("reverseIndex has %d entries, want 4", len(idx))
	}
}
