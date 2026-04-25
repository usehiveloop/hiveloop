// Package setup contains provider-agnostic primitives for idempotently
// provisioning subscription plans with a payment provider.
//
// Each provider implements PlanReconciler. CanonicalPlans (in plans.go)
// defines every plan variant the Hiveloop product sells across currencies
// and cycles; the reconciler ensures the upstream state matches — creating
// new plans and updating drifted ones as needed. Reconciliation never
// deletes: provider APIs typically don't support deletion and silent
// deprovisioning is too dangerous for billing.
//
// The decision logic (Decide, in decision.go) is a pure function: list of
// upstream plans + list of desired specs → list of actions. That's where
// almost all the tests live; the provider-specific reconcilers are thin
// HTTP shells around it.
package setup

import (
	"context"

	"github.com/usehiveloop/hiveloop/internal/billing"
)

// PlanSpec is the canonical description of one plan variant. One Hiveloop
// plan (starter/pro/business) expands into multiple specs — one per
// (currency, cycle) combination we sell.
type PlanSpec struct {
	Slug        string        // internal slug: "starter" | "pro" | "business"
	Name        string        // display name shown to users and stored upstream; also the match key on re-runs
	AmountMinor int64         // price in smallest subunit (kobo for NGN, cents for USD)
	Currency    string        // ISO-4217 uppercase: "NGN", "USD"
	Cycle       billing.Cycle // monthly | annual
	Description string        // optional, passes through to the provider
}

// Key returns the composite identifier used to match a PlanSpec against an
// existing upstream plan and to look up resolved plan_codes by callers.
func (s PlanSpec) Key() SpecKey {
	return SpecKey{Slug: s.Slug, Currency: s.Currency, Cycle: s.Cycle}
}

// SpecKey identifies one plan variant across (slug, currency, cycle). Stable
// and comparable — safe to use as a map key.
type SpecKey struct {
	Slug     string
	Currency string
	Cycle    billing.Cycle
}

// ExistingPlan is a provider-agnostic snapshot of one upstream plan. Adapters
// convert their native response shape into this before handing it to Decide.
// Fields not used by Decide (e.g. provider-specific send_invoices) stay
// hidden inside the adapter's apply step.
type ExistingPlan struct {
	PlanCode    string // upstream identifier (PLN_xxx for Paystack, prod_xxx for Polar, etc.)
	Name        string
	Currency    string
	Cycle       billing.Cycle // mapped from the provider's native interval string
	AmountMinor int64
	Description string
}

// ActionKind classifies what the reconciler will (or did) do for one spec.
type ActionKind string

const (
	ActionCreate ActionKind = "create"
	ActionUpdate ActionKind = "update"
	ActionNoOp   ActionKind = "noop"
)

// Action is the reconciliation verdict for a single PlanSpec.
type Action struct {
	Kind ActionKind
	Spec PlanSpec
	// ExistingCode is the upstream plan_code to operate on when Kind is
	// Update or NoOp. Empty when Kind is Create.
	ExistingCode string
	// DriftFields lists the fields that differ between existing and desired,
	// populated only when Kind is Update. Useful for structured logging.
	DriftFields []string
}

// ResolvedPlan is what the reconciler returns per spec after applying actions.
type ResolvedPlan struct {
	Key      SpecKey
	PlanCode string
	Action   ActionKind
}

// PlanReconciler is the interface every billing provider implements to make
// its subscription plans match CanonicalPlans. Implementations must be
// idempotent: running the same Reconcile twice with the same specs against
// the same upstream state produces the same results and performs at most
// the mutations needed to resolve drift.
type PlanReconciler interface {
	Reconcile(ctx context.Context, specs []PlanSpec) ([]ResolvedPlan, error)
}
