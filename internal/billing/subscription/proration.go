// Package subscription is the pure billing core: proration math, change
// previews, and renewal decisions. No DB or HTTP dependencies.
package subscription

import "time"

// ProrationFraction returns the unused fraction of [periodStart, periodEnd]
// at now, clamped to [0, 1]. Stored as (num, denom) seconds so the same
// fraction applied to money and credits can't drift apart through floats.
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

type Fraction struct {
	Numerator   int64
	Denominator int64
}

func (f Fraction) IsZero() bool { return f.Numerator == 0 || f.Denominator == 0 }
func (f Fraction) IsOne() bool {
	return f.Denominator > 0 && f.Numerator == f.Denominator
}

// Apply: round-half-to-even(amount * num / denom). Banker's rounding both
// signs so long prorations don't drift up. int64 product safe for our
// domain (amount ≤ 10^12 × secs ≤ 3·10^6 ≪ 9.2·10^18).
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
	abs2r := r * 2
	if abs2r < 0 {
		abs2r = -abs2r
	}
	switch {
	case abs2r < f.Denominator:
		return q
	case abs2r > f.Denominator:
		if num >= 0 {
			return q + 1
		}
		return q - 1
	}
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
