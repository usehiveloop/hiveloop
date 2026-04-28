package paystack

import (
	"context"
	"fmt"

	"github.com/usehiveloop/hiveloop/internal/billing"
)

type initializeRequest struct {
	Email       string            `json:"email"`
	Amount      int64             `json:"amount"`
	Currency    string            `json:"currency,omitempty"`
	Channels    []string          `json:"channels,omitempty"`
	CallbackURL string            `json:"callback_url,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type initializeResponse struct {
	AuthorizationURL string `json:"authorization_url"`
	AccessCode       string `json:"access_code"`
	Reference        string `json:"reference"`
}

// CreateCheckout initialises a Paystack transaction the browser will resume
// via PaystackPop.resumeTransaction(access_code). We charge a flat amount
// — no plan_code — because we manage the recurring lifecycle ourselves.
//
// Channels are restricted to card and bank: those are the only Paystack
// channels that issue a reusable AuthorizationCode, which is required for
// subscription renewals. Other channels (USSD, mobile money) are one-shot.
//
// The customerID argument is unused; Paystack derives the customer from the
// email on the transaction. We keep it in the signature to satisfy
// billing.Provider.
func (p *Provider) CreateCheckout(ctx context.Context, _ string, intent billing.CheckoutIntent) (*billing.CheckoutSession, error) {
	req := initializeRequest{
		Email:       intent.CustomerEmail,
		Amount:      intent.AmountMinor,
		Currency:    intent.Currency,
		Channels:    []string{string(billing.ChannelCard), string(billing.ChannelBank)},
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
		AccessCode: resp.AccessCode,
		Reference:  resp.Reference,
	}, nil
}
