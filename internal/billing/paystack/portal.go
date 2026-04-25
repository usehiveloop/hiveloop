package paystack

import (
	"context"
	"fmt"

	"github.com/usehiveloop/hiveloop/internal/billing"
)

type manageLinkResponse struct {
	Link string `json:"link"`
}

// CreatePortal returns a one-time Paystack "manage subscription" URL where
// the customer can update their card or cancel.
//
// Paystack has no Stripe-style multi-subscription portal, so the request
// MUST carry the subscription id. When a user has multiple active
// subscriptions (rare in our model — one per org), the caller should pass
// the most recent active subscription's external id.
//
// The returned link is short-lived and single-use; surface it as a redirect
// rather than a shareable URL.
func (p *Provider) CreatePortal(ctx context.Context, req billing.PortalRequest) (*billing.PortalSession, error) {
	if req.ExternalSubscriptionID == "" {
		return nil, billing.ErrNoActiveSubscription
	}
	var resp manageLinkResponse
	path := fmt.Sprintf("/subscription/%s/manage/link", req.ExternalSubscriptionID)
	if err := p.client.do(ctx, "GET", path, nil, &resp); err != nil {
		return nil, fmt.Errorf("manage link: %w", err)
	}
	if resp.Link == "" {
		return nil, fmt.Errorf("paystack returned empty manage link")
	}
	return &billing.PortalSession{URL: resp.Link}, nil
}
