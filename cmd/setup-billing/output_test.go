package main

import (
	"testing"

	"github.com/usehiveloop/hiveloop/internal/billing"
	"github.com/usehiveloop/hiveloop/internal/billing/setup"
)

func TestSetupEnvVarName(t *testing.T) {
	cases := []struct {
		name     string
		provider string
		key      setup.SpecKey
		want     string
	}{
		{
			name:     "paystack pro monthly NGN",
			provider: "paystack",
			key:      setup.SpecKey{Slug: "pro", Currency: "NGN", Cycle: billing.CycleMonthly},
			want:     "PAYSTACK_PLAN_PRO_NGN_MONTHLY",
		},
		{
			name:     "paystack starter annual NGN",
			provider: "paystack",
			key:      setup.SpecKey{Slug: "starter", Currency: "NGN", Cycle: billing.CycleAnnual},
			want:     "PAYSTACK_PLAN_STARTER_NGN_ANNUAL",
		},
		{
			name:     "future polar business monthly USD",
			provider: "polar",
			key:      setup.SpecKey{Slug: "business", Currency: "USD", Cycle: billing.CycleMonthly},
			want:     "POLAR_PLAN_BUSINESS_USD_MONTHLY",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := setup.EnvVarName(tc.provider, tc.key)
			if got != tc.want {
				t.Errorf("EnvVarName = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFormatEnvVars_SortedDeterministic(t *testing.T) {
	resolved := []setup.ResolvedPlan{
		{Key: setup.SpecKey{Slug: "pro", Currency: "NGN", Cycle: billing.CycleAnnual}, PlanCode: "PLN_proA"},
		{Key: setup.SpecKey{Slug: "starter", Currency: "NGN", Cycle: billing.CycleMonthly}, PlanCode: "PLN_starM"},
		{Key: setup.SpecKey{Slug: "business", Currency: "NGN", Cycle: billing.CycleMonthly}, PlanCode: "PLN_bizM"},
		{Key: setup.SpecKey{Slug: "pro", Currency: "NGN", Cycle: billing.CycleMonthly}, PlanCode: "PLN_proM"},
	}
	got := formatEnvVars("paystack", resolved)
	// Sorted by slug, currency, cycle (annual < monthly alphabetically).
	want := "PAYSTACK_PLAN_BUSINESS_NGN_MONTHLY=PLN_bizM\n" +
		"PAYSTACK_PLAN_PRO_NGN_ANNUAL=PLN_proA\n" +
		"PAYSTACK_PLAN_PRO_NGN_MONTHLY=PLN_proM\n" +
		"PAYSTACK_PLAN_STARTER_NGN_MONTHLY=PLN_starM\n"
	if got != want {
		t.Errorf("formatEnvVars produced:\n%s\nwant:\n%s", got, want)
	}
}
