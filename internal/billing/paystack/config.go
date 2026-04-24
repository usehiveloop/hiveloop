package paystack

import (
	"fmt"
	"strings"

	"github.com/usehiveloop/hiveloop/internal/billing"
)

// Config wires the Paystack adapter with credentials and plan-code mappings.
//
// Plans resolves (plan slug, currency, cycle) → Paystack plan_code. Paystack
// requires one plan per currency, so supporting a new currency for an
// existing slug means populating another entry in Plans — no code changes.
type Config struct {
	// SecretKey is the Paystack secret key ("sk_live_…" or "sk_test_…").
	// Used both for API authorization and for webhook HMAC verification —
	// Paystack signs webhooks with the same secret.
	SecretKey string

	// Plans is the slug → currency → PlanCodes registry. At minimum it
	// should contain every slug the app sells, each with at least one
	// currency. Empty PlanCodes entries (Monthly or Annual blank) mean
	// that cycle isn't sold on Paystack for that currency.
	Plans PlanRegistry
}

// PlanRegistry is keyed first by our plan slug ("starter"/"pro"/"business"),
// then by ISO-4217 currency (uppercase: "NGN", "USD", …).
//
// Example:
//
//	PlanRegistry{
//	    "pro": {"NGN": {Monthly: "PLN_abc", Annual: "PLN_xyz"}},
//	}
type PlanRegistry map[string]map[string]PlanCodes

// PlanCodes are the Paystack plan_code values for a single (slug, currency)
// pair — one per billing cycle. An empty value means that cycle isn't sold
// on Paystack for the currency.
type PlanCodes struct {
	Monthly string
	Annual  string
}

// Lookup resolves a Paystack plan_code for a checkout.
//
// Returns billing.ErrUnknownPlan when the slug isn't configured at all,
// billing.ErrUnsupportedCurrency when the slug exists but not in the
// requested currency, and a plain error when the cycle isn't recognised or
// the plan_code for that cycle is blank.
func (r PlanRegistry) Lookup(slug, currency string, cycle billing.Cycle) (string, error) {
	byCurrency, ok := r[slug]
	if !ok {
		return "", fmt.Errorf("paystack: %w: %q", billing.ErrUnknownPlan, slug)
	}
	codes, ok := byCurrency[strings.ToUpper(currency)]
	if !ok {
		return "", fmt.Errorf("paystack: %w: %s in %s", billing.ErrUnsupportedCurrency, slug, currency)
	}
	var code string
	switch cycle {
	case billing.CycleMonthly:
		code = codes.Monthly
	case billing.CycleAnnual:
		code = codes.Annual
	default:
		return "", fmt.Errorf("paystack: unsupported cycle %q", cycle)
	}
	if code == "" {
		return "", fmt.Errorf("paystack: %w: %s/%s (%s) not configured", billing.ErrUnknownPlan, slug, currency, cycle)
	}
	return code, nil
}

// reverseIndex builds a plan_code → slug map used by the webhook parser to
// resolve which of our plans a Paystack event refers to.
func (r PlanRegistry) reverseIndex() map[string]string {
	idx := map[string]string{}
	for slug, byCurrency := range r {
		for _, codes := range byCurrency {
			if codes.Monthly != "" {
				idx[codes.Monthly] = slug
			}
			if codes.Annual != "" {
				idx[codes.Annual] = slug
			}
		}
	}
	return idx
}
