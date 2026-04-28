package paystack

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/billing"
)

func (p *Provider) subscriptionState(d subscriptionData) (*billing.SubscriptionState, error) {
	orgID, err := extractOrgID(d.Customer.Metadata)
	if err != nil {
		return nil, err
	}
	state := &billing.SubscriptionState{
		ExternalSubscriptionID: d.SubscriptionCode,
		ExternalCustomerID:     d.Customer.CustomerCode,
		OrgID:                  orgID,
		PlanSlug:               p.cfg.Plans.SlugForPlanCode(d.Plan.PlanCode),
		Status:                 mapSubscriptionStatus(d.Status),
	}
	if d.CreatedAt != nil {
		state.CurrentPeriodStart = *d.CreatedAt
	}
	if d.NextPaymentDate != nil {
		state.CurrentPeriodEnd = *d.NextPaymentDate
	}
	return state, nil
}

func (p *Provider) chargeState(d chargeData, planCode string) (*billing.SubscriptionState, error) {
	orgID, err := extractOrgID(d.Customer.Metadata)
	if err != nil {
		return nil, err
	}
	if orgID == uuid.Nil {
		// Customer.metadata is often null on charge events; fall back to
		// the transaction-level metadata set on /transaction/initialize.
		orgID, err = extractOrgIDFromRaw(d.Metadata)
		if err != nil {
			return nil, err
		}
	}
	planSlug := p.cfg.Plans.SlugForPlanCode(planCode)
	if planSlug == "" {
		return nil, nil
	}
	state := &billing.SubscriptionState{
		ExternalCustomerID: d.Customer.CustomerCode,
		OrgID:              orgID,
		PlanSlug:           planSlug,
		Status:             billing.StatusActive,
		ChargeReference:    d.Reference,
		ChargeAmount:       d.Amount,
		ChargedAt:          d.PaidAt,
		CardLast4:          d.Authorization.Last4,
		CardBrand:          d.Authorization.Brand,
		CardExpMonth:       d.Authorization.ExpMonth,
		CardExpYear:        d.Authorization.ExpYear,
		AuthorizationCode:  d.Authorization.AuthorizationCode,
	}
	if d.PaidAt != nil {
		state.CurrentPeriodStart = *d.PaidAt
	}
	return state, nil
}

func (p *Provider) invoiceState(sub subscriptionData, customer customerPayload) (*billing.SubscriptionState, error) {
	orgID, err := extractOrgID(customer.Metadata)
	if err != nil {
		return nil, err
	}
	state := &billing.SubscriptionState{
		ExternalSubscriptionID: sub.SubscriptionCode,
		ExternalCustomerID:     customer.CustomerCode,
		OrgID:                  orgID,
		Status:                 mapSubscriptionStatus(sub.Status),
	}
	if sub.NextPaymentDate != nil {
		state.CurrentPeriodEnd = *sub.NextPaymentDate
	}
	return state, nil
}

// parseChargePlanCode handles the four shapes Paystack uses for data.plan
// on charge.success: missing/null/{}, "PLN_abc", or {"plan_code":...}.
func parseChargePlanCode(raw json.RawMessage) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) || bytes.Equal(trimmed, []byte("{}")) {
		return ""
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return strings.TrimSpace(asString)
	}
	var asObj planPayload
	if err := json.Unmarshal(raw, &asObj); err == nil {
		return asObj.PlanCode
	}
	return ""
}

func extractOrgID(meta map[string]string) (uuid.UUID, error) {
	raw, ok := meta["org_id"]
	if !ok || raw == "" {
		return uuid.Nil, nil
	}
	parsed, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid org_id %q: %w", raw, err)
	}
	return parsed, nil
}

func extractOrgIDFromRaw(raw json.RawMessage) (uuid.UUID, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return uuid.Nil, nil
	}
	var meta map[string]string
	if err := json.Unmarshal(raw, &meta); err != nil {
		return uuid.Nil, nil
	}
	return extractOrgID(meta)
}

// mapSubscriptionStatus: Paystack uses "non-renewing" while a sub winds
// down, "complete" once billing cycles exhaust, "cancelled" if cancelled.
func mapSubscriptionStatus(paystackStatus string) billing.SubscriptionStatus {
	switch strings.ToLower(paystackStatus) {
	case "active", "attention":
		return billing.StatusActive
	case "non-renewing", "cancelled", "canceled":
		return billing.StatusCanceled
	case "complete", "completed":
		return billing.StatusRevoked
	}
	return billing.StatusActive
}
