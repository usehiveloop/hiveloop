package subscription

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehiveloop/hiveloop/internal/billing"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// 30 days for every plan; annual not yet supported.
const RenewalPeriod = 30 * 24 * time.Hour

// Renew runs one attempt. Idempotent across replays. Errors propagate only
// for DB faults; charge declines are recorded on the row and swallowed by
// the asynq handler so the next sweep tick handles the next attempt.
func (s *Service) Renew(ctx context.Context, subID uuid.UUID) (RenewalAction, error) {
	now := s.clock()

	var sub model.Subscription
	err := s.db.WithContext(ctx).Preload("Plan").Preload("PendingPlan").
		Where("id = ?", subID).First(&sub).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ActionNoOp, nil
	}
	if err != nil {
		return "", fmt.Errorf("load subscription: %w", err)
	}

	view := RenewalSubscriptionView{
		Status:                 sub.Status,
		CurrentPeriodEnd:       sub.CurrentPeriodEnd,
		CancelAtPeriodEnd:      sub.CancelAtPeriodEnd,
		HasReusableAuthorization: sub.AuthorizationCode != "" &&
			billing.PaymentChannel(sub.PaymentChannel).IsReusable(),
		RenewalAttempts:      sub.RenewalAttempts,
		LastRenewalAttemptAt: sub.LastRenewalAttemptAt,
	}

	currentPlan := planViewOf(sub.Plan)
	var pendingPlan *PlanView
	if sub.PendingPlan != nil && sub.PendingPlan.ID != uuid.Nil {
		v := planViewOf(*sub.PendingPlan)
		pendingPlan = &v
	}

	decision := DecideRenewal(view, currentPlan, pendingPlan, now)

	switch decision.Action {
	case ActionNoOp:
		return ActionNoOp, nil
	case ActionCancelAtPeriodEnd:
		return ActionCancelAtPeriodEnd, s.finalizeCancel(ctx, &sub, now)
	case ActionTransitionToFree:
		return ActionTransitionToFree, s.transitionToFree(ctx, &sub, decision.TargetPlan, now)
	case ActionMarkPastDue:
		return ActionMarkPastDue, s.markPastDue(ctx, &sub, "no reusable authorization", now)
	case ActionCharge:
		return s.chargeAndAdvance(ctx, &sub, decision.TargetPlan, now)
	}
	return "", fmt.Errorf("unknown renewal action %q", decision.Action)
}

func (s *Service) finalizeCancel(ctx context.Context, sub *model.Subscription, now time.Time) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.Subscription{}).Where("id = ?", sub.ID).
			Updates(map[string]any{
				"status":               string(billing.StatusCanceled),
				"canceled_at":          &now,
				"cancel_at_period_end": false,
				"renewal_attempts":     0,
				"last_renewal_error":   "",
			}).Error; err != nil {
			return err
		}
		return tx.Model(&model.Org{}).Where("id = ?", sub.OrgID).
			Update("plan_slug", billing.FreePlanSlug).Error
	})
}

func (s *Service) transitionToFree(ctx context.Context, sub *model.Subscription, target PlanView, now time.Time) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.Subscription{}).Where("id = ?", sub.ID).
			Updates(map[string]any{
				"status":               string(billing.StatusCanceled),
				"plan_id":              target.ID,
				"pending_plan_id":      gorm.Expr("NULL"),
				"pending_change_at":    gorm.Expr("NULL"),
				"canceled_at":          &now,
				"cancel_at_period_end": false,
				"renewal_attempts":     0,
				"last_renewal_error":   "",
			}).Error; err != nil {
			return err
		}
		return tx.Model(&model.Org{}).Where("id = ?", sub.OrgID).
			Update("plan_slug", target.Slug).Error
	})
}

// markPastDue records the failure on the row. reason is logged onto
// last_renewal_error so the UI / support can surface it.
func (s *Service) markPastDue(ctx context.Context, sub *model.Subscription, reason string, now time.Time) error {
	return s.db.WithContext(ctx).Model(sub).Updates(map[string]any{
		"status":                  string(billing.StatusPastDue),
		"last_renewal_attempt_at": &now,
		"last_renewal_error":      truncateErr(reason),
	}).Error
}

func (s *Service) chargeAndAdvance(ctx context.Context, sub *model.Subscription, target PlanView, now time.Time) (RenewalAction, error) {
	provider, err := s.registry.Get(sub.Provider)
	if err != nil {
		return "", err
	}

	email, err := s.lookupOrgOwnerEmail(ctx, sub.OrgID)
	if err != nil {
		return "", fmt.Errorf("lookup org owner email: %w", err)
	}

	res, chargeErr := provider.ChargeAuthorization(ctx, billing.ChargeAuthorizationRequest{
		Email:             email,
		AuthorizationCode: sub.AuthorizationCode,
		AmountMinor:       target.PriceMinor,
		Currency:          target.Currency,
		Metadata: map[string]string{
			"org_id":          sub.OrgID.String(),
			"subscription_id": sub.ID.String(),
		},
	})
	if chargeErr != nil {
		return s.recordChargeFailure(ctx, sub, chargeErr, now)
	}

	return ActionCharge, s.applySuccessfulRenewal(ctx, sub, target, res, now)
}

func (s *Service) recordChargeFailure(ctx context.Context, sub *model.Subscription, chargeErr error, now time.Time) (RenewalAction, error) {
	attempts := sub.RenewalAttempts + 1
	updates := map[string]any{
		"renewal_attempts":        attempts,
		"last_renewal_attempt_at": &now,
		"last_renewal_error":      truncateErr(chargeErr.Error()),
	}
	action := ActionCharge
	if attempts >= MaxRenewalAttempts {
		updates["status"] = string(billing.StatusPastDue)
		action = ActionMarkPastDue
	}
	if err := s.db.WithContext(ctx).Model(sub).Updates(updates).Error; err != nil {
		return "", fmt.Errorf("record charge failure: %w (charge: %v)", err, chargeErr)
	}
	return action, chargeErr
}

func (s *Service) applySuccessfulRenewal(
	ctx context.Context,
	sub *model.Subscription,
	target PlanView,
	res *billing.ChargeAuthorizationResult,
	now time.Time,
) error {
	periodStart := sub.CurrentPeriodEnd
	if periodStart.IsZero() || periodStart.After(now) {
		periodStart = now
	}
	periodEnd := periodStart.Add(RenewalPeriod)

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var fresh model.Subscription
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", sub.ID).First(&fresh).Error; err != nil {
			return err
		}
		// Race: a parallel worker may have already advanced this row.
		if !fresh.CurrentPeriodEnd.IsZero() && fresh.CurrentPeriodEnd.After(now) {
			return nil
		}

		fresh.PlanID = target.ID
		fresh.Status = string(billing.StatusActive)
		fresh.CurrentPeriodStart = periodStart
		fresh.CurrentPeriodEnd = periodEnd
		fresh.PendingPlanID = nil
		fresh.PendingChangeAt = nil
		fresh.RenewalAttempts = 0
		fresh.LastRenewalError = ""
		t := now
		fresh.LastRenewalAttemptAt = &t
		fresh.LastChargeReference = res.Reference
		fresh.LastChargeAmount = res.PaidAmountMinor
		fresh.LastChargedAt = res.PaidAt
		if pm := res.PaymentMethod; pm.AuthorizationCode != "" {
			fresh.AuthorizationCode = pm.AuthorizationCode
			fresh.PaymentChannel = string(pm.Channel)
			fresh.CardLast4 = pm.CardLast4
			fresh.CardBrand = pm.CardBrand
			fresh.CardExpMonth = pm.CardExpMonth
			fresh.CardExpYear = pm.CardExpYear
			fresh.PaymentBankName = pm.BankName
			fresh.PaymentAccountName = pm.AccountName
		}
		if err := tx.Save(&fresh).Error; err != nil {
			return err
		}

		if err := tx.Model(&model.Org{}).Where("id = ?", sub.OrgID).
			Update("plan_slug", target.Slug).Error; err != nil {
			return err
		}

		if target.MonthlyCredits > 0 {
			expires := periodEnd.Add(billing.PlanGrantGracePeriod)
			if err := billing.GrantWithTx(tx, sub.OrgID, target.MonthlyCredits,
				billing.ReasonPlanGrant, "subscription_renewal", res.Reference,
				&expires); err != nil && !errors.Is(err, billing.ErrAlreadyRecorded) {
				return err
			}
		}
		return nil
	})
}

func (s *Service) lookupOrgOwnerEmail(ctx context.Context, orgID uuid.UUID) (string, error) {
	var membership model.OrgMembership
	if err := s.db.WithContext(ctx).Where("org_id = ?", orgID).
		Order("created_at ASC").First(&membership).Error; err != nil {
		return "", err
	}
	var user model.User
	if err := s.db.WithContext(ctx).Where("id = ?", membership.UserID).
		First(&user).Error; err != nil {
		return "", err
	}
	return user.Email, nil
}

func truncateErr(s string) string {
	const max = 500
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
