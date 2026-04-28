package subscription

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/billing"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// CancelInput controls Cancel's behavior. AtPeriodEnd=true is the default.
type CancelInput struct {
	AtPeriodEnd bool
}

// Cancel marks the org's active subscription as canceled. With AtPeriodEnd
// true (the Stripe default), the customer keeps access through
// CurrentPeriodEnd and the renewal worker is responsible for transitioning
// to status='canceled' afterwards. With AtPeriodEnd false the subscription
// drops to canceled immediately and the org snaps to the free plan.
func (s *Service) Cancel(ctx context.Context, orgID uuid.UUID, in CancelInput) (*model.Subscription, error) {
	now := s.clock()
	sub, err := s.activeSubscription(ctx, orgID)
	if err != nil {
		return nil, err
	}
	if sub.Status != string(billing.StatusActive) {
		return nil, ErrCannotCancel
	}

	if in.AtPeriodEnd {
		err := s.db.WithContext(ctx).Model(sub).
			Update("cancel_at_period_end", true).Error
		if err != nil {
			return nil, err
		}
		sub.CancelAtPeriodEnd = true
		return sub, nil
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(sub).Updates(map[string]any{
			"status":               string(billing.StatusCanceled),
			"canceled_at":          &now,
			"cancel_at_period_end": false,
		}).Error; err != nil {
			return err
		}
		return tx.Model(&model.Org{}).Where("id = ?", orgID).
			Update("plan_slug", billing.FreePlanSlug).Error
	})
	if err != nil {
		return nil, err
	}
	sub.Status = string(billing.StatusCanceled)
	sub.CanceledAt = &now
	return sub, nil
}

// Resume clears CancelAtPeriodEnd on an active subscription. A subscription
// that has fully transitioned to status='canceled' cannot be resumed —
// the customer must subscribe afresh.
func (s *Service) Resume(ctx context.Context, orgID uuid.UUID) (*model.Subscription, error) {
	sub, err := s.activeSubscription(ctx, orgID)
	if err != nil {
		return nil, err
	}
	if sub.Status != string(billing.StatusActive) {
		return nil, ErrCannotResume
	}
	if !sub.CancelAtPeriodEnd {
		return sub, nil
	}
	if err := s.db.WithContext(ctx).Model(sub).Update("cancel_at_period_end", false).Error; err != nil {
		return nil, err
	}
	sub.CancelAtPeriodEnd = false
	return sub, nil
}
