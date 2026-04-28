// Package subscription is the pure billing core: proration math, change
// previews, and state transitions. It has no DB or HTTP dependencies so
// the rules can be exercised by table-driven unit tests at the speed of
// pure-function calls.
//
// Higher layers (handler, service) project model rows into this package's
// PlanView/SubscriptionView shapes, run the pure functions, and persist
// the resulting transitions inside a DB transaction.
package subscription

import "time"

// ProrationFraction returns the unused fraction of [periodStart, periodEnd]
// at the given instant, expressed as (numerator, denominator) seconds.
// The fraction is clamped to [0, 1]:
//   - now <= periodStart  → 1/1 (full period unused)
//   - now >= periodEnd    → 0/1 (period over)
//
// We track the fraction as a pair of integers (rather than a float) so
// the same fraction can be applied to both money and credit-grant
// integers without rounding drift between them.
func ProrationFraction(now, periodStart, periodEnd time.Time) Fraction {
	totalSecs := secondsBetween(periodStart, periodEnd)
	if totalSecs <= 0 {
		return Fraction{Numerator: 0, Denominator: 1}
	}
	if !now.After(periodStart) {
		return Fraction{Numerator: totalSecs, Denominator: totalSecs}
	}
	if !now.Before(periodEnd) {
		return Fraction{Numerator: 0, Denominator: totalSecs}
	}
	return Fraction{
		Numerator:   secondsBetween(now, periodEnd),
		Denominator: totalSecs,
	}
}

// Fraction is a non-negative rational. Apply uses banker's-style rounding
// (round-half-to-even) so a long sequence of prorations doesn't drift up.
type Fraction struct {
	Numerator   int64
	Denominator int64
}

// IsZero reports whether the fraction is effectively zero.
func (f Fraction) IsZero() bool { return f.Numerator == 0 || f.Denominator == 0 }

// IsOne reports whether the fraction equals 1.
func (f Fraction) IsOne() bool {
	return f.Denominator > 0 && f.Numerator == f.Denominator
}

// Apply returns round-half-to-even(amount * f.Numerator / f.Denominator).
// Returns 0 when the fraction is zero. Negative amounts are supported and
// rounded symmetrically (banker's rounding both signs).
//
// Overflow: int64 product is safe for our domain — minor-unit amounts
// up to ~10^12 multiplied by month-of-seconds (~3·10^6) sit comfortably
// below int64's ~9.2·10^18 ceiling.
func (f Fraction) Apply(amount int64) int64 {
	if f.Denominator <= 0 || f.Numerator == 0 || amount == 0 {
		return 0
	}
	if f.Numerator == f.Denominator {
		return amount
	}
	num := amount * f.Numerator
	q, r := num/f.Denominator, num%f.Denominator
	if r == 0 {
		return q
	}
	// Compare |2r| with |denom|. Denominator is always positive.
	abs2r := r * 2
	if abs2r < 0 {
		abs2r = -abs2r
	}
	switch {
	case abs2r < f.Denominator:
		// Below half — Go's truncate-toward-zero (q) is already correct.
		return q
	case abs2r > f.Denominator:
		// Above half — round away from zero.
		if num >= 0 {
			return q + 1
		}
		return q - 1
	}
	// Exactly half — round to even.
	if q%2 == 0 {
		return q
	}
	if num >= 0 {
		return q + 1
	}
	return q - 1
}

func secondsBetween(a, b time.Time) int64 {
	d := b.Sub(a)
	return int64(d / time.Second)
}
