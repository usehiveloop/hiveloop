package subscription_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	sub "github.com/usehiveloop/hiveloop/internal/billing/subscription"
)

var (
	rNow      = time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	rDueEnd   = rNow.Add(-time.Minute) // period ended a minute ago
	rFutureEnd = rNow.Add(24 * time.Hour)

	planFree = sub.PlanView{
		ID: uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		Slug: "free", PriceMinor: 0, Currency: "NGN", MonthlyCredits: 100,
	}
	planPro = sub.PlanView{
		ID: uuid.MustParse("33333333-3333-3333-3333-333333333333"),
		Slug: "pro", PriceMinor: 2_000_000, Currency: "NGN", MonthlyCredits: 5_000,
	}
	planStarter = sub.PlanView{
		ID: uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		Slug: "starter", PriceMinor: 500_000, Currency: "NGN", MonthlyCredits: 1_000,
	}
)

func TestDecideRenewal_Charge(t *testing.T) {
	got := sub.DecideRenewal(
		sub.RenewalSubscriptionView{
			Status:                   "active",
			CurrentPeriodEnd:         rDueEnd,
			HasReusableAuthorization: true,
		},
		planPro, nil, rNow,
	)
	if got.Action != sub.ActionCharge {
		t.Fatalf("Action = %q, want charge", got.Action)
	}
	if got.TargetPlan.ID != planPro.ID {
		t.Errorf("TargetPlan = %v, want planPro", got.TargetPlan.ID)
	}
}

func TestDecideRenewal_NoOp_NotDueYet(t *testing.T) {
	got := sub.DecideRenewal(
		sub.RenewalSubscriptionView{
			Status:                   "active",
			CurrentPeriodEnd:         rFutureEnd,
			HasReusableAuthorization: true,
		},
		planPro, nil, rNow,
	)
	if got.Action != sub.ActionNoOp {
		t.Fatalf("Action = %q, want noop", got.Action)
	}
}

func TestDecideRenewal_NoOp_StatusNotActive(t *testing.T) {
	got := sub.DecideRenewal(
		sub.RenewalSubscriptionView{
			Status:                   "canceled",
			CurrentPeriodEnd:         rDueEnd,
			HasReusableAuthorization: true,
		},
		planPro, nil, rNow,
	)
	if got.Action != sub.ActionNoOp {
		t.Fatalf("Action = %q, want noop", got.Action)
	}
}

func TestDecideRenewal_CancelAtPeriodEnd(t *testing.T) {
	got := sub.DecideRenewal(
		sub.RenewalSubscriptionView{
			Status:                   "active",
			CurrentPeriodEnd:         rDueEnd,
			CancelAtPeriodEnd:        true,
			HasReusableAuthorization: true,
		},
		planPro, nil, rNow,
	)
	if got.Action != sub.ActionCancelAtPeriodEnd {
		t.Fatalf("Action = %q, want cancel_at_period_end", got.Action)
	}
}

func TestDecideRenewal_PendingDowngradeToFree(t *testing.T) {
	got := sub.DecideRenewal(
		sub.RenewalSubscriptionView{
			Status:                   "active",
			CurrentPeriodEnd:         rDueEnd,
			HasReusableAuthorization: true,
		},
		planPro, &planFree, rNow,
	)
	if got.Action != sub.ActionTransitionToFree {
		t.Fatalf("Action = %q, want transition_to_free", got.Action)
	}
}

func TestDecideRenewal_PendingDowngradeToPaidChargesNew(t *testing.T) {
	got := sub.DecideRenewal(
		sub.RenewalSubscriptionView{
			Status:                   "active",
			CurrentPeriodEnd:         rDueEnd,
			HasReusableAuthorization: true,
		},
		planPro, &planStarter, rNow,
	)
	if got.Action != sub.ActionCharge {
		t.Fatalf("Action = %q, want charge", got.Action)
	}
	if got.TargetPlan.ID != planStarter.ID {
		t.Errorf("TargetPlan = %v, want planStarter (pending)", got.TargetPlan.ID)
	}
}

func TestDecideRenewal_NoAuth_MarksPastDue(t *testing.T) {
	got := sub.DecideRenewal(
		sub.RenewalSubscriptionView{
			Status:                   "active",
			CurrentPeriodEnd:         rDueEnd,
			HasReusableAuthorization: false,
		},
		planPro, nil, rNow,
	)
	if got.Action != sub.ActionMarkPastDue {
		t.Fatalf("Action = %q, want mark_past_due", got.Action)
	}
}

func TestDecideRenewal_CancelBeatsPendingChange(t *testing.T) {
	// If the customer queued a downgrade and then canceled, cancellation
	// wins — we don't quietly "downgrade to free" via the pending path.
	got := sub.DecideRenewal(
		sub.RenewalSubscriptionView{
			Status:                   "active",
			CurrentPeriodEnd:         rDueEnd,
			CancelAtPeriodEnd:        true,
			HasReusableAuthorization: true,
		},
		planPro, &planStarter, rNow,
	)
	if got.Action != sub.ActionCancelAtPeriodEnd {
		t.Fatalf("Action = %q, want cancel_at_period_end (cancel wins)", got.Action)
	}
}
