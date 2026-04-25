package setup

import (
	"fmt"
	"strings"
)

// EnvVarName is the canonical naming convention for plan-code environment
// variables across every provider adapter.
//
//	provider=paystack, slug=pro, currency=NGN, cycle=monthly
//	→ PAYSTACK_PLAN_PRO_NGN_MONTHLY
//
// The setup-billing CLI emits env blocks in this shape on stderr, and the
// bootstrap code reads the same shape when constructing a provider config —
// keeping the CLI's output and the server's input in lockstep.
func EnvVarName(provider string, k SpecKey) string {
	return strings.ToUpper(fmt.Sprintf("%s_PLAN_%s_%s_%s",
		provider, k.Slug, k.Currency, string(k.Cycle)))
}
