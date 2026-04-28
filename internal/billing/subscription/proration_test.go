package subscription_test

import (
	"testing"
	"time"

	sub "github.com/usehiveloop/hiveloop/internal/billing/subscription"
)

func TestProrationFraction(t *testing.T) {
	periodStart := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := periodStart.Add(30 * 24 * time.Hour)

	tests := []struct {
		name              string
		now               time.Time
		wantNum, wantDenom int64
		wantApply1000     int64 // result of applying to 1000 minor units
	}{
		{
			name:          "exact period start (full unused)",
			now:           periodStart,
			wantApply1000: 1000,
		},
		{
			name:          "before period start (clamps to full unused)",
			now:           periodStart.Add(-time.Hour),
			wantApply1000: 1000,
		},
		{
			name:          "exact period end (none unused)",
			now:           periodEnd,
			wantApply1000: 0,
		},
		{
			name:          "after period end (clamps to none unused)",
			now:           periodEnd.Add(time.Hour),
			wantApply1000: 0,
		},
		{
			name:          "halfway through period",
			now:           periodStart.Add(15 * 24 * time.Hour),
			wantApply1000: 500,
		},
		{
			name:          "one quarter through period",
			now:           periodStart.Add(7*24*time.Hour + 12*time.Hour),
			wantApply1000: 750,
		},
		{
			name:          "99% through (small unused)",
			now:           periodEnd.Add(-30 * time.Minute / 5), // 6 minutes left of 30 days
			wantApply1000: 0,                                    // rounds toward zero on tiny fractions
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := sub.ProrationFraction(tc.now, periodStart, periodEnd)
			got := f.Apply(1000)
			if got != tc.wantApply1000 {
				t.Errorf("Apply(1000) = %d, want %d (fraction %d/%d)",
					got, tc.wantApply1000, f.Numerator, f.Denominator)
			}
		})
	}
}

func TestFraction_Apply_Rounding(t *testing.T) {
	// Fraction 1/3 over int64 inputs that hit each rounding case.
	tests := []struct {
		name string
		f    sub.Fraction
		in   int64
		want int64
	}{
		{"exact division", sub.Fraction{Numerator: 1, Denominator: 2}, 1000, 500},
		{"round half down (toward even)", sub.Fraction{Numerator: 1, Denominator: 2}, 5, 2},
		{"round half up (toward even)", sub.Fraction{Numerator: 1, Denominator: 2}, 7, 4},
		{"truncate when below half", sub.Fraction{Numerator: 1, Denominator: 3}, 1, 0},
		{"round when above half", sub.Fraction{Numerator: 2, Denominator: 3}, 1, 1},
		{"zero fraction", sub.Fraction{Numerator: 0, Denominator: 30}, 1000, 0},
		{"zero amount", sub.Fraction{Numerator: 1, Denominator: 2}, 0, 0},
		{"negative amount, halfway", sub.Fraction{Numerator: 1, Denominator: 2}, -5, -2},
		{"identity 1/1", sub.Fraction{Numerator: 30, Denominator: 30}, 7777, 7777},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.f.Apply(tc.in)
			if got != tc.want {
				t.Errorf("Apply(%d) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

func TestFraction_IsZero_IsOne(t *testing.T) {
	if !(sub.Fraction{Numerator: 0, Denominator: 30}).IsZero() {
		t.Error("0/30 should be zero")
	}
	if !(sub.Fraction{Numerator: 30, Denominator: 30}).IsOne() {
		t.Error("30/30 should be one")
	}
	if (sub.Fraction{Numerator: 15, Denominator: 30}).IsZero() {
		t.Error("15/30 should not be zero")
	}
	if (sub.Fraction{Numerator: 15, Denominator: 30}).IsOne() {
		t.Error("15/30 should not be one")
	}
}

func TestProrationFraction_DegenerateRange(t *testing.T) {
	// Empty range — start == end.
	t0 := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	f := sub.ProrationFraction(t0, t0, t0)
	if !f.IsZero() {
		t.Errorf("expected zero fraction for empty range, got %d/%d", f.Numerator, f.Denominator)
	}

	// Inverted range — end before start.
	f = sub.ProrationFraction(t0, t0.Add(time.Hour), t0)
	if !f.IsZero() {
		t.Errorf("expected zero fraction for inverted range, got %d/%d", f.Numerator, f.Denominator)
	}
}
