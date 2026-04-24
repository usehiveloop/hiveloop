package main

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/usehiveloop/hiveloop/internal/billing/setup"
)

// writeJSON emits the resolved plans as a JSON array — machine-readable
// output for CI pipelines and shell scripts (e.g. `| jq`).
func writeJSON(w io.Writer, resolved []setup.ResolvedPlan) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(resolved)
}

// formatEnvVars returns a text block of environment variables keyed by
// provider / slug / currency / cycle, with each plan_code as the value.
// Naming mirrors setup.EnvVarName, which is what the server's bootstrap
// reads when constructing a provider config.
//
// Example output:
//
//	PAYSTACK_PLAN_STARTER_NGN_MONTHLY=PLN_abc123
//	PAYSTACK_PLAN_STARTER_NGN_ANNUAL=PLN_def456
//	...
//
// Entries are sorted deterministically (slug, currency, cycle) so
// re-runs produce byte-identical output — diffable in git or CI logs.
func formatEnvVars(provider string, resolved []setup.ResolvedPlan) string {
	sorted := append([]setup.ResolvedPlan(nil), resolved...)
	sort.Slice(sorted, func(i, j int) bool {
		a, b := sorted[i].Key, sorted[j].Key
		if a.Slug != b.Slug {
			return a.Slug < b.Slug
		}
		if a.Currency != b.Currency {
			return a.Currency < b.Currency
		}
		return string(a.Cycle) < string(b.Cycle)
	})

	var sb strings.Builder
	for _, rp := range sorted {
		fmt.Fprintf(&sb, "%s=%s\n", setup.EnvVarName(provider, rp.Key), rp.PlanCode)
	}
	return sb.String()
}
