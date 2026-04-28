package subscription

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehiveloop/hiveloop/internal/billing"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// ApplyChange consumes a quote. For upgrades it requires a verified Paystack
// reference whose paid amount and currency match the quote; the quoted
// proration credits are granted to the org ledger and the subscription's
// plan is swapped immediately. For downgrades the quote is consumed without
// a charge and the change is recorded as pending until the renewal worker
// advances the period.
//
// ApplyChange is idempotent: re-applying an already-consumed quote returns
// success without performing the work twice.
func (s *Service) ApplyChange(ctx context.Context, orgID uuid.UUID, quoteID uuid.UUID, paystackReference string) error {
	quote, err := s.loadQuote(ctx, quoteID)
	if err != nil {
		return err
	}
	if quote.OrgID != orgID {
		return ErrQuoteWrongOrg
	}
	if quote.ConsumedAt != nil {
		return nil // idempotent replay
	}
	if s.clock().After(quote.ExpiresAt) {
		return ErrQuoteExpired
	}

	switch ChangeKind(quote.Kind) {
	case KindUpgrade:
		return s.applyUpgrade(ctx, quote, paystackReference)
	case KindDowngrade:
		return s.applyDowngrade(ctx, quote)
	}
	return fmt.Errorf("subscription: unknown quote kind %q", quote.Kind)
}

func (s *Service) applyUpgrade(ctx context.Context, quote *model.SubscriptionChangeQuote, reference string) error {
	if reference == "" {
		return errors.New("subscription: upgrade quote requires a paystack reference")
	}

	provider, err := s.providerForSubscription(ctx, quote.SubscriptionID)
	if err != nil {
		return err
	}

	res, err := provider.ResolveCheckout(ctx, billing.ResolveCheckoutRequest{
		Reference:     reference,
		ExpectedOrgID: quote.OrgID,
	})
	if err != nil {
		return fmt.Errorf("resolve checkout: %w", err)
	}
	if res.Status != billing.StatusActive {
		return ErrChargeRejected
	}
	if res.PaidAmountMinor != quote.AmountMinor {
		return ErrAmountMismatch
	}
	if res.Currency != quote.Currency {
		return ErrCurrencyMismatchOnVerify
	}
	if !res.PaymentMethod.Channel.IsReusable() {
		return ErrUnsupportedChannel
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var sub model.Subscription
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", quote.SubscriptionID).First(&sub).Error; err != nil {
			return err
		}

		var fresh model.SubscriptionChangeQuote
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", quote.ID).First(&fresh).Error; err != nil {
			return err
		}
		if fresh.ConsumedAt != nil {
			return nil // raced, the other writer won
		}

		var target model.Plan
		if err := tx.Where("id = ?", quote.ToPlanID).First(&target).Error; err != nil {
			return err
		}

		applyPaymentMethod(&sub, res)
		sub.PlanID = target.ID
		sub.Status = string(billing.StatusActive)
		sub.PendingPlanID = nil
		sub.PendingChangeAt = nil
		if err := tx.Save(&sub).Error; err != nil {
			return err
		}

		if quote.ProrationCreditMinor > 0 {
			expiresAt := sub.CurrentPeriodEnd
			if expiresAt.IsZero() {
				expiresAt = s.clock().Add(billing.PlanGrantGracePeriod)
			}
			if err := billing.GrantWithTx(tx, sub.OrgID, quote.ProrationCreditMinor,
				billing.ReasonAdjustment, "subscription_change_quote", quote.ID.String(),
				&expiresAt); err != nil {
				return err
			}
		}

		// Denormalize plan slug onto the org so spend checks stay cheap.
		if err := tx.Model(&model.Org{}).Where("id = ?", sub.OrgID).
			Update("plan_slug", target.Slug).Error; err != nil {
			return err
		}

		now := s.clock()
		ref := reference
		return tx.Model(&fresh).Updates(map[string]any{
			"consumed_at":        &now,
			"paystack_reference": &ref,
		}).Error
	})
}

func (s *Service) applyDowngrade(ctx context.Context, quote *model.SubscriptionChangeQuote) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var fresh model.SubscriptionChangeQuote
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", quote.ID).First(&fresh).Error; err != nil {
			return err
		}
		if fresh.ConsumedAt != nil {
			return nil
		}

		now := s.clock()
		toPlan := quote.ToPlanID
		effectiveAt := quote.EffectiveAt
		if err := tx.Model(&model.Subscription{}).
			Where("id = ?", quote.SubscriptionID).
			Updates(map[string]any{
				"pending_plan_id":   &toPlan,
				"pending_change_at": &effectiveAt,
			}).Error; err != nil {
			return err
		}

		return tx.Model(&fresh).Update("consumed_at", &now).Error
	})
}
