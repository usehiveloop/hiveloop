package handler

import (
	"github.com/usehiveloop/hiveloop/internal/billing"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// SnapshotPaymentForTest exposes snapshotPayment for package-external tests.
func (h *BillingWebhookHandler) SnapshotPaymentForTest(providerName string, event billing.Event) error {
	return h.snapshotPayment(providerName, event)
}

// snapshotPayment writes the payment-method snapshot from a charge event onto
// the matching Subscription row so the billing UI can display "Visa ending in
// 4242, last charged Apr 28" without round-tripping the provider. The match
// is by (provider, external_customer_id) — for renewal flows the customer
// has exactly one active subscription so this resolves cleanly. Silently
// no-ops when no Subscription row exists yet (subscription.create may arrive
// after charge.success); the next charge will fill it in.
func (h *BillingWebhookHandler) snapshotPayment(providerName string, event billing.Event) error {
	state := event.Subscription
	if state == nil || state.ChargeReference == "" {
		return nil
	}
	if state.ExternalCustomerID == "" {
		return nil
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

	res := h.db.Model(&model.Subscription{}).
		Where("provider = ? AND external_customer_id = ? AND status = ?",
			providerName, state.ExternalCustomerID, string(billing.StatusActive)).
		Updates(updates)
	return res.Error
}
