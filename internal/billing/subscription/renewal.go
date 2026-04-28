package subscription

import "time"

// 5 attempts, 1h apart, => total worst case ~5h to past_due.
const MaxRenewalAttempts = 5
const RenewalRetryInterval = time.Hour

type RenewalAction string

const (
	ActionNoOp              RenewalAction = "noop"
	ActionCancelAtPeriodEnd RenewalAction = "cancel_at_period_end"
	ActionTransitionToFree  RenewalAction = "transition_to_free"
	ActionMarkPastDue       RenewalAction = "mark_past_due"
	ActionCharge            RenewalAction = "charge"
)

type RenewalDecision struct {
	Action     RenewalAction
	TargetPlan PlanView
	Reason     string
}

type RenewalSubscriptionView struct {
	Status                   string
	CurrentPeriodEnd         time.Time
	CancelAtPeriodEnd        bool
	HasReusableAuthorization bool
	RenewalAttempts          int
	LastRenewalAttemptAt     *time.Time
}

// DecideRenewal: cancel beats pending plan, pending beats normal renewal.
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
		return RenewalDecision{Action: ActionTransitionToFree, TargetPlan: target, Reason: "target plan is free"}
	}
	if !sub.HasReusableAuthorization {
		return RenewalDecision{Action: ActionMarkPastDue, Reason: "no reusable authorization on file"}
	}
	return RenewalDecision{Action: ActionCharge, TargetPlan: target, Reason: "due renewal"}
}
