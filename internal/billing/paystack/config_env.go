package paystack

import (
	"os"
	"strings"

	"github.com/usehiveloop/hiveloop/internal/billing"
	"github.com/usehiveloop/hiveloop/internal/billing/setup"
)

// PlanRegistryFromEnv walks setup.CanonicalPlans and pulls each plan_code out
// of the environment under the canonical name (see setup.EnvVarName, e.g.
// PAYSTACK_PLAN_PRO_NGN_MONTHLY). Missing or blank env vars are skipped —
// that slug/currency/cycle simply isn't sold on Paystack.
//
// currencies restricts the walk to the given ISO-4217 codes; empty pulls all
// canonical currencies. The returned registry is safe to hand straight to
// paystack.Config{Plans: …}.
func PlanRegistryFromEnv(currencies ...string) PlanRegistry {
	specs := setup.FilterByCurrency(currencies...)
	reg := PlanRegistry{}
	for _, spec := range specs {
		code := strings.TrimSpace(os.Getenv(setup.EnvVarName(Name, spec.Key())))
		if code == "" {
			continue
		}
		byCurrency, ok := reg[spec.Slug]
		if !ok {
			byCurrency = map[string]PlanCodes{}
			reg[spec.Slug] = byCurrency
		}
		codes := byCurrency[spec.Currency]
		switch spec.Cycle {
		case billing.CycleMonthly:
			codes.Monthly = code
		case billing.CycleAnnual:
			codes.Annual = code
		}
		byCurrency[spec.Currency] = codes
	}
	return reg
}
