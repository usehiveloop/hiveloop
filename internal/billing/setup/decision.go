package setup

import "github.com/usehiveloop/hiveloop/internal/billing"

// Decide is the pure reconciliation core. Given the current state of the
// upstream provider (existing) and the desired specs, it returns an ordered
// list of Actions — one per spec — that the caller then applies.
//
// Match semantics: an ExistingPlan matches a PlanSpec when (Name, Currency,
// Cycle) are all equal. Name is case-sensitive; Paystack and most other
// providers treat display names as literal strings.
//
// Drift is detected on the handful of fields Decide can see: AmountMinor
// and Description. Provider-specific fields (Paystack's send_invoices,
// invoice_limit, etc.) stay in the adapter's apply step and don't force
// an Update just by existing.
//
// Existing plans that don't match any spec are IGNORED — the reconciler
// never deletes. This is important: provider APIs often don't support
// deletion, and silently deprovisioning a plan with live subscribers is
// catastrophic.
func Decide(existing []ExistingPlan, desired []PlanSpec) []Action {
	byKey := make(map[matchKey]ExistingPlan, len(existing))
	for _, e := range existing {
		byKey[keyFor(e.Name, e.Currency, e.Cycle)] = e
	}

	actions := make([]Action, 0, len(desired))
	for _, spec := range desired {
		k := keyFor(spec.Name, spec.Currency, spec.Cycle)
		ex, found := byKey[k]
		if !found {
			actions = append(actions, Action{Kind: ActionCreate, Spec: spec})
			continue
		}

		drift := driftFields(ex, spec)
		if len(drift) == 0 {
			actions = append(actions, Action{
				Kind:         ActionNoOp,
				Spec:         spec,
				ExistingCode: ex.PlanCode,
			})
			continue
		}

		actions = append(actions, Action{
			Kind:         ActionUpdate,
			Spec:         spec,
			ExistingCode: ex.PlanCode,
			DriftFields:  drift,
		})
	}
	return actions
}

// matchKey is the composite lookup key. Unexported — callers use Decide.
type matchKey struct {
	Name     string
	Currency string
	Cycle    string
}

func keyFor(name, currency string, cycle billing.Cycle) matchKey {
	return matchKey{Name: name, Currency: currency, Cycle: string(cycle)}
}

// driftFields returns the list of field names that differ between an existing
// upstream plan and the desired spec. Empty slice means no update needed.
func driftFields(ex ExistingPlan, spec PlanSpec) []string {
	var fields []string
	if ex.AmountMinor != spec.AmountMinor {
		fields = append(fields, "amount")
	}
	if ex.Description != spec.Description {
		fields = append(fields, "description")
	}
	return fields
}
