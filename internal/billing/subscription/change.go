package subscription

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrSamePlan         = errors.New("subscription: target plan is the same as current plan")
	ErrCurrencyMismatch = errors.New("subscription: cannot change between plans in different currencies")
	ErrPeriodEnded      = errors.New("subscription: current period has already ended")
	// ErrFreeUpgrade: caller must use fresh-checkout flow — no saved auth to charge.
	ErrFreeUpgrade = errors.New("subscription: upgrade from free requires a fresh checkout")
)

type ChangeKind string

const (
	KindUpgrade   ChangeKind = "upgrade"
	KindDowngrade ChangeKind = "downgrade"
)

type PlanView struct {
	ID             uuid.UUID
	Slug           string
	PriceMinor     int64
	Currency       string
	MonthlyCredits int64
}

func (p PlanView) IsFree() bool { return p.PriceMinor == 0 }

type SubscriptionView struct {
	ID                 uuid.UUID
	Plan               PlanView
	CurrentPeriodStart time.Time
	CurrentPeriodEnd   time.Time
	CancelAtPeriodEnd  bool
}

// Upgrades: charged AmountMinor immediately, granted CreditGrantMinor
// (expiring at CurrentPeriodEnd). Downgrades: both fields are 0 and
// EffectiveAt is the current period end — the renewal worker swaps the
// plan when it advances the period.
type ChangePreview struct {
	Kind             ChangeKind
	AmountMinor      int64
	Currency         string
	CreditGrantMinor int64
	CreditExpiresAt  time.Time
	EffectiveAt      time.Time
	UnusedFraction   Fraction
}

// PreviewChange validates in user-visible failure-mode order: same-plan,
// currency, expired period, free-upgrade. Then prorates.
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

	return &ChangePreview{
		Kind:           KindDowngrade,
		AmountMinor:    0,
		Currency:       target.Currency,
		EffectiveAt:    sub.CurrentPeriodEnd,
		UnusedFraction: frac,
	}, nil
}
