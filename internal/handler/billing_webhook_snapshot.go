package handler

import (
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/billing"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// SnapshotPaymentForTest exposes snapshotPayment for package-external tests.
func (h *BillingWebhookHandler) SnapshotPaymentForTest(providerName string, event billing.Event) error {
	return h.snapshotPayment(providerName, event)
}

// snapshotPayment runs on charge.success. It writes the payment-method
// snapshot (card last4/brand/exp, charge ref/amount/paid_at, auth_code)
// onto the matching Subscription row, and creates that row if it doesn't
// exist yet — Paystack sometimes only fires charge.success and never
// subscription.create for popup-flow first charges, so we don't depend on
// subscription.create arriving.
//
// Match key: (provider, org_id, plan_id, status='active'). One org has
// at most one active subscription per plan; renewals UPDATE the row,
// fresh subscriptions INSERT. When subscription.create eventually arrives,
// upsertSubscription's matching-by-org fallback finds this same row and
// overlays the external_subscription_id.
func (h *BillingWebhookHandler) snapshotPayment(providerName string, event billing.Event) error {
	state := event.Subscription
	if state == nil || state.ChargeReference == "" {
		return nil
	}
	if state.OrgID == uuid.Nil || state.PlanSlug == "" {
		return nil
	}

	var plan model.Plan
	if err := h.db.Where("slug = ?", state.PlanSlug).First(&plan).Error; err != nil {
		return err
	}

	updates := map[string]any{
		"last_charge_reference": state.ChargeReference,
		"last_charge_amount":    state.ChargeAmount,
	}
	if state.ChargedAt != nil {
		updates["last_charged_at"] = state.ChargedAt
	}
	if state.CardLast4 != "" {
		updates["card_last4"] = state.CardLast4
	}
	if state.CardBrand != "" {
		updates["card_brand"] = state.CardBrand
	}
	if state.CardExpMonth != "" {
		updates["card_exp_month"] = state.CardExpMonth
	}
	if state.CardExpYear != "" {
		updates["card_exp_year"] = state.CardExpYear
	}
	if state.AuthorizationCode != "" {
		updates["authorization_code"] = state.AuthorizationCode
	}
	if state.ExternalCustomerID != "" {
		updates["external_customer_id"] = state.ExternalCustomerID
	}

	var existing model.Subscription
	err := h.db.Where("provider = ? AND org_id = ? AND plan_id = ? AND status = ?",
		providerName, state.OrgID, plan.ID, string(billing.StatusActive)).
		First(&existing).Error
	if err == nil {
		return h.db.Model(&existing).Updates(updates).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	sub := model.Subscription{
		OrgID:                  state.OrgID,
		PlanID:                 plan.ID,
		Provider:               providerName,
		ExternalSubscriptionID: state.ExternalSubscriptionID, // typically empty on charge.success
		ExternalCustomerID:     state.ExternalCustomerID,
		Status:                 string(billing.StatusActive),
		CurrentPeriodStart:     state.CurrentPeriodStart,
		CurrentPeriodEnd:       state.CurrentPeriodEnd,
		LastChargeReference:    state.ChargeReference,
		LastChargeAmount:       state.ChargeAmount,
		CardLast4:              state.CardLast4,
		CardBrand:              state.CardBrand,
		CardExpMonth:           state.CardExpMonth,
		CardExpYear:            state.CardExpYear,
		AuthorizationCode:      state.AuthorizationCode,
	}
	if state.ChargedAt != nil {
		sub.LastChargedAt = state.ChargedAt
	}
	if err := h.db.Create(&sub).Error; err != nil {
		return err
	}

	// Denormalise plan slug onto the org so runtime checks stay cheap.
	return h.db.Model(&model.Org{}).Where("id = ?", state.OrgID).
		Update("plan_slug", plan.Slug).Error
}
