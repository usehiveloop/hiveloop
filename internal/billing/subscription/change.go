package subscription

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// Domain errors. Handlers should map these to appropriate HTTP statuses.
var (
	// ErrSamePlan is returned by PreviewChange when target == current.
	ErrSamePlan = errors.New("subscription: target plan is the same as current plan")

	// ErrCurrencyMismatch is returned by PreviewChange when the target plan
	// is denominated in a different currency than the current subscription.
	// We don't auto-convert; the caller must cancel + start a fresh checkout.
	ErrCurrencyMismatch = errors.New("subscription: cannot change between plans in different currencies")

	// ErrPeriodEnded is returned when now > sub.CurrentPeriodEnd. The
	// renewal worker should advance the period before any change is made.
	ErrPeriodEnded = errors.New("subscription: current period has already ended")

	// ErrFreeUpgrade is returned when the customer is on the free plan and
	// wants a paid plan. The handler should redirect them to the fresh-
	// checkout flow because there's no saved authorization to charge.
	ErrFreeUpgrade = errors.New("subscription: upgrade from free requires a fresh checkout")
)

// ChangeKind classifies a subscription transition.
type ChangeKind string

const (
	KindUpgrade   ChangeKind = "upgrade"
	KindDowngrade ChangeKind = "downgrade"
)

// PlanView is the minimal projection of model.Plan that the billing core
// needs. The handler builds this from a model.Plan row.
type PlanView struct {
	ID             uuid.UUID
	Slug           string
	PriceMinor     int64
	Currency       string
	MonthlyCredits int64
}

// IsFree reports whether the plan has no recurring price.
func (p PlanView) IsFree() bool { return p.PriceMinor == 0 }

// SubscriptionView is the minimal projection of model.Subscription.
type SubscriptionView struct {
	ID                 uuid.UUID
	Plan               PlanView
	CurrentPeriodStart time.Time
	CurrentPeriodEnd   time.Time
	CancelAtPeriodEnd  bool
}

// ChangePreview describes the financial and credit-ledger consequences of
// switching a subscription's plan.
//
// For upgrades, the customer is charged AmountMinor immediately and granted
// CreditGrantMinor AI-usage credits expiring at the current period end —
// they're paying today for the unused remainder of the cycle on the new
// plan, and they get the new plan's credit allowance prorated to match.
//
// For downgrades, the change is deferred to the period end: AmountMinor and
// CreditGrantMinor are both 0, EffectiveAt is the current period end, and
// the renewal worker is responsible for swapping the plan when it advances
// the period.
type ChangePreview struct {
	Kind                 ChangeKind
	AmountMinor          int64
	Currency             string
	CreditGrantMinor     int64
	CreditExpiresAt      time.Time
	EffectiveAt          time.Time
	UnusedFraction       Fraction
}

// PreviewChange produces the ChangePreview for switching sub.Plan to target
// at the given instant. Validation order matches the user-visible failure
// modes: same-plan first (cheapest check), then currency, then expired
// period, then free-upgrade.
func PreviewChange(sub SubscriptionView, target PlanView, now time.Time) (*ChangePreview, error) {
	if sub.Plan.ID == target.ID {
		return nil, ErrSamePlan
	}
	if sub.Plan.Currency != target.Currency {
		return nil, ErrCurrencyMismatch
	}
	if !sub.CurrentPeriodEnd.IsZero() && now.After(sub.CurrentPeriodEnd) {
		return nil, ErrPeriodEnded
	}
	if sub.Plan.IsFree() && !target.IsFree() {
		return nil, ErrFreeUpgrade
	}

	frac := ProrationFraction(now, sub.CurrentPeriodStart, sub.CurrentPeriodEnd)

	if target.PriceMinor > sub.Plan.PriceMinor {
		// Upgrade. Charge the prorated price difference and grant the
		// prorated additional monthly credits, expiring at period end so
		// they don't outlive the cycle the customer paid for.
		priceDelta := target.PriceMinor - sub.Plan.PriceMinor
		creditDelta := target.MonthlyCredits - sub.Plan.MonthlyCredits

		amount := frac.Apply(priceDelta)
		creditGrant := int64(0)
		if creditDelta > 0 {
			creditGrant = frac.Apply(creditDelta)
		}

		return &ChangePreview{
			Kind:             KindUpgrade,
			AmountMinor:      amount,
			Currency:         target.Currency,
			CreditGrantMinor: creditGrant,
			CreditExpiresAt:  sub.CurrentPeriodEnd,
			EffectiveAt:      now,
			UnusedFraction:   frac,
		}, nil
	}

	// Downgrade. No charge today, no credit change today; the renewal
	// worker will swap to target at sub.CurrentPeriodEnd.
	return &ChangePreview{
		Kind:           KindDowngrade,
		AmountMinor:    0,
		Currency:       target.Currency,
		EffectiveAt:    sub.CurrentPeriodEnd,
		UnusedFraction: frac,
	}, nil
}
