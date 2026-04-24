package paystack

import (
	"testing"

	"github.com/usehiveloop/hiveloop/internal/billing"
)

func TestPlanRegistryFromEnv_BuildsFromCanonicalNames(t *testing.T) {
	t.Setenv("PAYSTACK_PLAN_STARTER_NGN_MONTHLY", "PLN_starM")
	t.Setenv("PAYSTACK_PLAN_STARTER_NGN_ANNUAL", "PLN_starA")
	t.Setenv("PAYSTACK_PLAN_PRO_NGN_MONTHLY", "PLN_proM")
	// Pro annual deliberately missing — registry should treat it as "not sold".

	reg := PlanRegistryFromEnv("NGN")

	got, err := reg.Lookup("starter", "NGN", billing.CycleMonthly)
	if err != nil || got != "PLN_starM" {
		t.Errorf("starter/NGN/monthly = %q err=%v, want PLN_starM", got, err)
	}
	got, err = reg.Lookup("starter", "NGN", billing.CycleAnnual)
	if err != nil || got != "PLN_starA" {
		t.Errorf("starter/NGN/annual = %q err=%v, want PLN_starA", got, err)
	}
	got, err = reg.Lookup("pro", "NGN", billing.CycleMonthly)
	if err != nil || got != "PLN_proM" {
		t.Errorf("pro/NGN/monthly = %q err=%v, want PLN_proM", got, err)
	}
	// Missing env → Lookup must report unknown, not return "".
	if _, err := reg.Lookup("pro", "NGN", billing.CycleAnnual); err == nil {
		t.Error("pro/NGN/annual: expected ErrUnknownPlan for missing env, got nil")
	}
}

func TestPlanRegistryFromEnv_IgnoresUnsetVars(t *testing.T) {
	// No PAYSTACK_PLAN_* env vars set — registry must be empty, not panic.
	reg := PlanRegistryFromEnv("NGN")
	if len(reg) != 0 {
		t.Errorf("registry with no env vars = %v, want empty", reg)
	}
}

func TestPlanRegistryFromEnv_TrimsWhitespace(t *testing.T) {
	t.Setenv("PAYSTACK_PLAN_BUSINESS_NGN_MONTHLY", "  PLN_biz  ")
	reg := PlanRegistryFromEnv("NGN")
	got, _ := reg.Lookup("business", "NGN", billing.CycleMonthly)
	if got != "PLN_biz" {
		t.Errorf("got %q, want trimmed PLN_biz", got)
	}
}
