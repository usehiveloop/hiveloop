package paystack

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/billing"
)

type verifyTransactionResponse struct {
	Reference     string             `json:"reference"`
	Status        string             `json:"status"`
	Amount        int64              `json:"amount"`
	Currency      string             `json:"currency"`
	PaidAt        *time.Time         `json:"paid_at"`
	Customer      customerPayload    `json:"customer"`
	Plan          json.RawMessage    `json:"plan"`
	Metadata      json.RawMessage    `json:"metadata"`
	Authorization authorizationBlock `json:"authorization"`
}

type listSubscriptionsItem struct {
	SubscriptionCode string     `json:"subscription_code"`
	Status           string     `json:"status"`
	NextPaymentDate  *time.Time `json:"next_payment_date"`
	CreatedAt        *time.Time `json:"createdAt"`
}

func (p *Provider) ResolveCheckout(ctx context.Context, req billing.ResolveCheckoutRequest) (*billing.ResolveCheckoutResult, error) {
	if req.Reference == "" {
		return nil, fmt.Errorf("paystack resolve: empty reference")
	}

	var tx verifyTransactionResponse
	if err := p.client.do(ctx, http.MethodGet, "/transaction/verify/"+url.PathEscape(req.Reference), nil, &tx); err != nil {
		return nil, fmt.Errorf("verify transaction: %w", err)
	}

	if tx.Status != "success" {
		return &billing.ResolveCheckoutResult{Status: mapTransactionStatus(tx.Status)}, nil
	}

	if req.ExpectedOrgID != uuid.Nil {
		orgID, err := extractOrgIDFromRaw(tx.Metadata)
		if err == nil && orgID == uuid.Nil {
			orgID, _ = extractOrgID(tx.Customer.Metadata)
		}
		if orgID != uuid.Nil && orgID != req.ExpectedOrgID {
			return nil, fmt.Errorf("paystack resolve: reference belongs to a different org")
		}
	}

	planCode := parseChargePlanCode(tx.Plan)
	if planCode == "" {
		return nil, fmt.Errorf("paystack resolve: transaction has no plan")
	}
	planSlug := p.cfg.Plans.SlugForPlanCode(planCode)
	if planSlug == "" {
		return nil, fmt.Errorf("paystack resolve: plan %q not managed", planCode)
	}

	res := &billing.ResolveCheckoutResult{
		Status:             billing.StatusActive,
		PlanSlug:           planSlug,
		ExternalCustomerID: tx.Customer.CustomerCode,
		ChargeReference:    tx.Reference,
		ChargeAmount:       tx.Amount,
		ChargedAt:          tx.PaidAt,
		CardLast4:          tx.Authorization.Last4,
		CardBrand:          tx.Authorization.Brand,
		CardExpMonth:       tx.Authorization.ExpMonth,
		CardExpYear:        tx.Authorization.ExpYear,
		AuthorizationCode:  tx.Authorization.AuthorizationCode,
	}
	if tx.PaidAt != nil {
		res.CurrentPeriodStart = *tx.PaidAt
	}

	if sub, err := p.findActiveSubscription(ctx, tx.Customer.CustomerCode, planCode); err == nil && sub != nil {
		res.ExternalSubscriptionID = sub.SubscriptionCode
		if sub.NextPaymentDate != nil {
			res.CurrentPeriodEnd = *sub.NextPaymentDate
		}
		if sub.CreatedAt != nil && res.CurrentPeriodStart.IsZero() {
			res.CurrentPeriodStart = *sub.CreatedAt
		}
	}

	return res, nil
}

// findActiveSubscription queries /subscription?customer=&plan= for the
// subscription Paystack creates after the first successful charge. Returns
// (nil, nil) when no row exists yet — callers treat that as "charge succeeded
// but the subscription isn't visible yet" and fall back to charge data.
func (p *Provider) findActiveSubscription(ctx context.Context, customerCode, planCode string) (*listSubscriptionsItem, error) {
	if customerCode == "" || planCode == "" {
		return nil, nil
	}
	q := url.Values{}
	q.Set("customer", customerCode)
	q.Set("plan", planCode)

	var items []listSubscriptionsItem
	if err := p.client.do(ctx, http.MethodGet, "/subscription?"+q.Encode(), nil, &items); err != nil {
		return nil, err
	}
	for i := range items {
		return &items[i], nil
	}
	return nil, nil
}

func mapTransactionStatus(s string) billing.SubscriptionStatus {
	switch s {
	case "success":
		return billing.StatusActive
	case "failed", "abandoned", "reversed":
		return billing.StatusRevoked
	}
	return billing.StatusPastDue
}
