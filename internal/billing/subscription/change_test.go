package subscription_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	sub "github.com/usehiveloop/hiveloop/internal/billing/subscription"
)

// fixtures for the change matrix.
var (
	periodStart = time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	periodEnd   = periodStart.Add(30 * 24 * time.Hour)
	midPeriod   = periodStart.Add(15 * 24 * time.Hour) // exactly halfway

	planFreeNGN = sub.PlanView{
		ID:             uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		Slug:           "free",
		PriceMinor:     0,
		Currency:       "NGN",
		MonthlyCredits: 100,
	}
	planStarterNGN = sub.PlanView{
		ID:             uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		Slug:           "starter",
		PriceMinor:     500_000, // ₦5,000
		Currency:       "NGN",
		MonthlyCredits: 1_000,
	}
	planProNGN = sub.PlanView{
		ID:             uuid.MustParse("33333333-3333-3333-3333-333333333333"),
		Slug:           "pro",
		PriceMinor:     2_000_000, // ₦20,000
		Currency:       "NGN",
		MonthlyCredits: 5_000,
	}
	planPremiumNGN = sub.PlanView{
		ID:             uuid.MustParse("44444444-4444-4444-4444-444444444444"),
		Slug:           "premium",
		PriceMinor:     5_000_000, // ₦50,000
		Currency:       "NGN",
		MonthlyCredits: 20_000,
	}
	planProUSD = sub.PlanView{
		ID:             uuid.MustParse("55555555-5555-5555-5555-555555555555"),
		Slug:           "pro-usd",
		PriceMinor:     2_500_00, // $25
		Currency:       "USD",
		MonthlyCredits: 5_000,
	}
)

func subWithPlan(p sub.PlanView) sub.SubscriptionView {
	return sub.SubscriptionView{
		ID:                 uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"),
		Plan:               p,
		CurrentPeriodStart: periodStart,
		CurrentPeriodEnd:   periodEnd,
	}
}

func TestPreviewChange_SamePlan(t *testing.T) {
	_, err := sub.PreviewChange(subWithPlan(planStarterNGN), planStarterNGN, midPeriod)
	if !errors.Is(err, sub.ErrSamePlan) {
		t.Fatalf("expected ErrSamePlan, got %v", err)
	}
}

func TestPreviewChange_CurrencyMismatch(t *testing.T) {
	_, err := sub.PreviewChange(subWithPlan(planStarterNGN), planProUSD, midPeriod)
	if !errors.Is(err, sub.ErrCurrencyMismatch) {
		t.Fatalf("expected ErrCurrencyMismatch, got %v", err)
	}
}

func TestPreviewChange_FreeUpgradeRejected(t *testing.T) {
	_, err := sub.PreviewChange(subWithPlan(planFreeNGN), planStarterNGN, midPeriod)
	if !errors.Is(err, sub.ErrFreeUpgrade) {
		t.Fatalf("expected ErrFreeUpgrade, got %v", err)
	}
}

func TestPreviewChange_PeriodEnded(t *testing.T) {
	_, err := sub.PreviewChange(subWithPlan(planStarterNGN), planProNGN, periodEnd.Add(time.Hour))
	if !errors.Is(err, sub.ErrPeriodEnded) {
		t.Fatalf("expected ErrPeriodEnded, got %v", err)
	}
}

func TestPreviewChange_UpgradeProrated(t *testing.T) {
	tests := []struct {
		name             string
		from, to         sub.PlanView
		now              time.Time
		wantKind         sub.ChangeKind
		wantAmount       int64
		wantCreditGrant  int64
	}{
		{
			name:            "starter→pro halfway",
			from:            planStarterNGN, to: planProNGN, now: midPeriod,
			wantKind:        sub.KindUpgrade,
			wantAmount:      750_000, // (2_000_000 - 500_000) * 1/2
			wantCreditGrant: 2_000,   // (5_000 - 1_000) * 1/2
		},
		{
			name:            "starter→pro at period start (full new period)",
			from:            planStarterNGN, to: planProNGN, now: periodStart,
			wantKind:        sub.KindUpgrade,
			wantAmount:      1_500_000, // 2_000_000 - 500_000
			wantCreditGrant: 4_000,
		},
		{
			name:            "starter→premium quarter through (75% remaining)",
			from:            planStarterNGN, to: planPremiumNGN,
			now:             periodStart.Add(7*24*time.Hour + 12*time.Hour),
			wantKind:        sub.KindUpgrade,
			wantAmount:      3_375_000, // (5_000_000 - 500_000) * 0.75
			wantCreditGrant: 14_250,    // (20_000 - 1_000) * 0.75
		},
		{
			name:            "starter→premium nearly at period end",
			from:            planStarterNGN, to: planPremiumNGN,
			now:             periodEnd.Add(-time.Hour),
			wantKind:        sub.KindUpgrade,
			wantAmount:      6_250, // (4_500_000) * 1h/720h
			wantCreditGrant: 26,    // (19_000) * 1h/720h
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := sub.PreviewChange(subWithPlan(tc.from), tc.to, tc.now)
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got.Kind != tc.wantKind {
				t.Errorf("Kind = %q, want %q", got.Kind, tc.wantKind)
			}
			if got.AmountMinor != tc.wantAmount {
				t.Errorf("AmountMinor = %d, want %d", got.AmountMinor, tc.wantAmount)
			}
			if got.CreditGrantMinor != tc.wantCreditGrant {
				t.Errorf("CreditGrantMinor = %d, want %d", got.CreditGrantMinor, tc.wantCreditGrant)
			}
			if got.Currency != tc.to.Currency {
				t.Errorf("Currency = %q, want %q", got.Currency, tc.to.Currency)
			}
			if !got.EffectiveAt.Equal(tc.now) {
				t.Errorf("EffectiveAt = %v, want %v (now)", got.EffectiveAt, tc.now)
			}
			if !got.CreditExpiresAt.Equal(periodEnd) {
				t.Errorf("CreditExpiresAt = %v, want %v (periodEnd)", got.CreditExpiresAt, periodEnd)
			}
		})
	}
}

func TestPreviewChange_DowngradeDeferred(t *testing.T) {
	tests := []struct {
		name     string
		from, to sub.PlanView
		now      time.Time
	}{
		{"premium→pro halfway", planPremiumNGN, planProNGN, midPeriod},
		{"pro→starter near end", planProNGN, planStarterNGN, periodEnd.Add(-time.Hour)},
		{"pro→free", planProNGN, planFreeNGN, midPeriod},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := sub.PreviewChange(subWithPlan(tc.from), tc.to, tc.now)
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got.Kind != sub.KindDowngrade {
				t.Errorf("Kind = %q, want downgrade", got.Kind)
			}
			if got.AmountMinor != 0 {
				t.Errorf("AmountMinor = %d, want 0 for downgrade", got.AmountMinor)
			}
			if got.CreditGrantMinor != 0 {
				t.Errorf("CreditGrantMinor = %d, want 0 for downgrade", got.CreditGrantMinor)
			}
			if !got.EffectiveAt.Equal(periodEnd) {
				t.Errorf("EffectiveAt = %v, want periodEnd %v", got.EffectiveAt, periodEnd)
			}
		})
	}
}

func TestPreviewChange_FreeToFree(t *testing.T) {
	// Conceptually a no-op; ErrSamePlan since IDs match.
	_, err := sub.PreviewChange(subWithPlan(planFreeNGN), planFreeNGN, midPeriod)
	if !errors.Is(err, sub.ErrSamePlan) {
		t.Fatalf("expected ErrSamePlan, got %v", err)
	}
}

func TestPreviewChange_UpgradeWithFewerCreditsClampsGrantAtZero(t *testing.T) {
	// Edge case: target plan costs more but somehow gives fewer monthly
	// credits. We should not reach into the customer's wallet.
	weirdPlan := sub.PlanView{
		ID:             uuid.New(),
		Slug:           "weird",
		PriceMinor:     1_000_000,
		Currency:       "NGN",
		MonthlyCredits: 50, // less than starter's 1_000
	}
	got, err := sub.PreviewChange(subWithPlan(planStarterNGN), weirdPlan, midPeriod)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.CreditGrantMinor != 0 {
		t.Errorf("CreditGrantMinor = %d, want 0 (no negative credit grants)", got.CreditGrantMinor)
	}
}
