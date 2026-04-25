package paystack

import (
	"context"
	"fmt"

	"github.com/usehiveloop/hiveloop/internal/billing"
)

// initializeRequest is the POST /transaction/initialize body. We omit the
// amount field — when `plan` is set, Paystack takes the amount from the plan
// itself. Pass-through metadata lands on the transaction and is echoed back
// in charge.success webhooks.
type initializeRequest struct {
	Email       string            `json:"email"`
	Plan        string            `json:"plan,omitempty"`
	Currency    string            `json:"currency,omitempty"`
	CallbackURL string            `json:"callback_url,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type initializeResponse struct {
	AuthorizationURL string `json:"authorization_url"`
	AccessCode       string `json:"access_code"`
	Reference        string `json:"reference"`
}

// CreateCheckout initialises a Paystack transaction tied to a plan.
//
// Paystack creates the Subscription automatically after the first successful
// charge (delivered as subscription.create via webhook) — so this call only
// returns the authorization URL the browser should redirect to. Our local
// Subscription row is created from the webhook handler, not from this method.
//
// The customerID argument is unused; Paystack derives the customer from the
// email on the transaction. We keep it in the signature to satisfy
// billing.Provider.
func (p *Provider) CreateCheckout(ctx context.Context, _ string, intent billing.CheckoutIntent) (*billing.CheckoutSession, error) {
	planCode, err := p.cfg.Plans.Lookup(intent.PlanSlug, intent.Currency, intent.Cycle)
	if err != nil {
		return nil, err
	}

	req := initializeRequest{
		Email:       intent.CustomerEmail,
		Plan:        planCode,
		Currency:    intent.Currency,
		CallbackURL: intent.SuccessURL,
		Metadata:    intent.Metadata,
	}
	var resp initializeResponse
	if err := p.client.do(ctx, "POST", "/transaction/initialize", req, &resp); err != nil {
		return nil, fmt.Errorf("initialize transaction: %w", err)
	}
	if resp.AuthorizationURL == "" {
		return nil, fmt.Errorf("paystack returned empty authorization_url")
	}
	return &billing.CheckoutSession{
		URL:        resp.AuthorizationURL,
		ExternalID: resp.Reference,
	}, nil
}
