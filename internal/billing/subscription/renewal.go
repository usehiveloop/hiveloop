package subscription

import "time"

// MaxRenewalAttempts is the cap on charge attempts per period before a
// subscription is moved to past_due. Each attempt is rate-limited via
// RenewalRetryInterval, so 5 attempts span at minimum 5 * 1h = 5h.
const MaxRenewalAttempts = 5

// RenewalRetryInterval is the minimum gap between renewal attempts on the
// same subscription. The sweep query filters out rows that were attempted
// more recently than this so a fast-firing cron doesn't burn the budget
// in seconds.
const RenewalRetryInterval = time.Hour

// RenewalAction is the verdict the pure decision function hands back to
// the service layer.
type RenewalAction string

const (
	// ActionNoOp means no work to do this tick — the subscription has
	// already been advanced, isn't due yet, or is in a non-renewable state.
	ActionNoOp RenewalAction = "noop"

	// ActionCancelAtPeriodEnd finalizes a deferred cancel: marks the row
	// canceled, drops the org to free, no charge.
	ActionCancelAtPeriodEnd RenewalAction = "cancel_at_period_end"

	// ActionTransitionToFree applies a deferred downgrade-to-free: swaps
	// the plan, ends the recurring billing relationship, no charge.
	ActionTransitionToFree RenewalAction = "transition_to_free"

	// ActionMarkPastDue moves the row to past_due because we have no
	// reusable authorization to charge against. Customer must add a
	// payment method via fresh checkout.
	ActionMarkPastDue RenewalAction = "mark_past_due"

	// ActionCharge tells the service to charge TargetPlan via the
	// provider and advance the period on success.
	ActionCharge RenewalAction = "charge"
)

// RenewalDecision describes what the renewal worker should do.
type RenewalDecision struct {
	Action     RenewalAction
	TargetPlan PlanView // populated for ActionCharge / ActionTransitionToFree
	Reason     string
}

// RenewalSubscriptionView is the projection of a Subscription row that
// the pure decision needs. Kept separate from SubscriptionView (used by
// PreviewChange) so each function gets exactly the fields it cares about.
type RenewalSubscriptionView struct {
	Status                 string
	CurrentPeriodEnd       time.Time
	CancelAtPeriodEnd      bool
	HasReusableAuthorization bool
	RenewalAttempts        int
	LastRenewalAttemptAt   *time.Time
}

// DecideRenewal is the pure renewal-state machine.
//
// Inputs:
//   - sub: the row's renewal-relevant state
//   - currentPlan: the plan the row is currently on
//   - pendingPlan: the deferred plan change scheduled for this period end,
//     or nil if no pending change
//   - now: the wall-clock instant of the decision
//
// Decision order matches the user-visible precedence: cancellation wins
// over plan changes, plan changes win over normal renewal. ActionNoOp is
// returned for any subscription not yet due.
func DecideRenewal(
	sub RenewalSubscriptionView,
	currentPlan PlanView,
	pendingPlan *PlanView,
	now time.Time,
) RenewalDecision {
	if sub.Status != "active" {
		return RenewalDecision{Action: ActionNoOp, Reason: "status not active"}
	}
	if !sub.CurrentPeriodEnd.IsZero() && now.Before(sub.CurrentPeriodEnd) {
		return RenewalDecision{Action: ActionNoOp, Reason: "period not yet ended"}
	}

	if sub.CancelAtPeriodEnd {
		return RenewalDecision{Action: ActionCancelAtPeriodEnd, Reason: "cancel_at_period_end set"}
	}

	target := currentPlan
	if pendingPlan != nil {
		target = *pendingPlan
	}

	if target.IsFree() {
		return RenewalDecision{
			Action:     ActionTransitionToFree,
			TargetPlan: target,
			Reason:     "target plan is free",
		}
	}

	if !sub.HasReusableAuthorization {
		return RenewalDecision{Action: ActionMarkPastDue, Reason: "no reusable authorization on file"}
	}

	return RenewalDecision{
		Action:     ActionCharge,
		TargetPlan: target,
		Reason:     "due renewal",
	}
}
