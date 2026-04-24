package setup

import "github.com/usehiveloop/hiveloop/internal/billing"

// CanonicalPlans is the single source of truth for every plan variant
// Hiveloop sells. Pricing mirrors business/pricing.md exactly.
//
// USD prices are in cents, NGN prices are in kobo. Annual is 20% off the
// 12× monthly list price (see pricing.md § Annual discount). NGN conversions
// use the pegged rate of 1 USD = 1,500 NGN documented in pricing.md.
//
// To add a currency (e.g. enable USD on Paystack), append the corresponding
// specs here and rerun the setup script. Runs are idempotent: existing plans
// are left alone, new ones get created.
//
// To add a plan tier, append specs and update pricing.md in the same PR so
// the two stay in lockstep.
var CanonicalPlans = []PlanSpec{
	// ── NGN ───────────────────────────────────────────────────────────────
	{
		Slug:        "starter",
		Name:        "Starter (Monthly)",
		AmountMinor: 13_500 * 100, // ₦13,500
		Currency:    "NGN",
		Cycle:       billing.CycleMonthly,
		Description: "Starter plan — 9,000 credits per month.",
	},
	{
		Slug:        "starter",
		Name:        "Starter (Annual)",
		AmountMinor: 129_000 * 100, // ₦129,000 (20% off 12× monthly)
		Currency:    "NGN",
		Cycle:       billing.CycleAnnual,
		Description: "Starter plan — 9,000 credits per month, billed yearly.",
	},
	{
		Slug:        "pro",
		Name:        "Pro (Monthly)",
		AmountMinor: 58_500 * 100, // ₦58,500
		Currency:    "NGN",
		Cycle:       billing.CycleMonthly,
		Description: "Pro plan — 39,000 credits per month.",
	},
	{
		Slug:        "pro",
		Name:        "Pro (Annual)",
		AmountMinor: 561_000 * 100, // ₦561,000 (20% off 12× monthly)
		Currency:    "NGN",
		Cycle:       billing.CycleAnnual,
		Description: "Pro plan — 39,000 credits per month, billed yearly.",
	},
	{
		Slug:        "business",
		Name:        "Business (Monthly)",
		AmountMinor: 148_500 * 100, // ₦148,500
		Currency:    "NGN",
		Cycle:       billing.CycleMonthly,
		Description: "Business plan — 99,000 credits per month.",
	},
	{
		Slug:        "business",
		Name:        "Business (Annual)",
		AmountMinor: 1_425_000 * 100, // ₦1,425,000 (20% off 12× monthly)
		Currency:    "NGN",
		Cycle:       billing.CycleAnnual,
		Description: "Business plan — 99,000 credits per month, billed yearly.",
	},

	// ── USD (not yet enabled on Paystack — add to the reconciler's
	// currency filter when USD settlement is provisioned) ─────────────────
	// {Slug: "starter",  Name: "Starter (Monthly)",  AmountMinor:    9_00, Currency: "USD", Cycle: billing.CycleMonthly, Description: "..."},
	// {Slug: "starter",  Name: "Starter (Annual)",   AmountMinor:   86_00, Currency: "USD", Cycle: billing.CycleAnnual,  Description: "..."},
	// {Slug: "pro",      Name: "Pro (Monthly)",      AmountMinor:   39_00, Currency: "USD", Cycle: billing.CycleMonthly, Description: "..."},
	// {Slug: "pro",      Name: "Pro (Annual)",       AmountMinor:  374_00, Currency: "USD", Cycle: billing.CycleAnnual,  Description: "..."},
	// {Slug: "business", Name: "Business (Monthly)", AmountMinor:   99_00, Currency: "USD", Cycle: billing.CycleMonthly, Description: "..."},
	// {Slug: "business", Name: "Business (Annual)",  AmountMinor:  950_00, Currency: "USD", Cycle: billing.CycleAnnual,  Description: "..."},
}

// FilterByCurrency returns the subset of CanonicalPlans matching any of the
// given ISO-4217 currency codes. Empty currencies returns the full slice.
func FilterByCurrency(currencies ...string) []PlanSpec {
	if len(currencies) == 0 {
		return append([]PlanSpec(nil), CanonicalPlans...)
	}
	allowed := make(map[string]bool, len(currencies))
	for _, c := range currencies {
		allowed[c] = true
	}
	out := make([]PlanSpec, 0, len(CanonicalPlans))
	for _, p := range CanonicalPlans {
		if allowed[p.Currency] {
			out = append(out, p)
		}
	}
	return out
}
