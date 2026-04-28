package paystack

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/usehiveloop/hiveloop/internal/billing"
)

type chargeAuthorizationRequest struct {
	Email             string            `json:"email"`
	AuthorizationCode string            `json:"authorization_code"`
	Amount            int64             `json:"amount"`
	Currency          string            `json:"currency,omitempty"`
	Reference         string            `json:"reference,omitempty"`
	Metadata          map[string]string `json:"metadata,omitempty"`
}

type chargeAuthorizationResponse struct {
	Reference     string             `json:"reference"`
	Status        string             `json:"status"`
	Amount        int64              `json:"amount"`
	Currency      string             `json:"currency"`
	Channel       string             `json:"channel"`
	PaidAt        *time.Time         `json:"paid_at"`
	Authorization authorizationBlock `json:"authorization"`
	GatewayResp   string             `json:"gateway_response"`
}

// ChargeAuthorization re-charges a saved authorization off-session via
// /transaction/charge_authorization. Returns ErrAuthorizationRefused on a
// non-success gateway response.
func (p *Provider) ChargeAuthorization(ctx context.Context, req billing.ChargeAuthorizationRequest) (*billing.ChargeAuthorizationResult, error) {
	if req.AuthorizationCode == "" {
		return nil, fmt.Errorf("paystack charge: empty authorization_code")
	}
	if req.AmountMinor <= 0 {
		return nil, fmt.Errorf("paystack charge: amount must be positive")
	}

	body := chargeAuthorizationRequest{
		Email:             req.Email,
		AuthorizationCode: req.AuthorizationCode,
		Amount:            req.AmountMinor,
		Currency:          req.Currency,
		Reference:         req.Reference,
		Metadata:          req.Metadata,
	}
	var resp chargeAuthorizationResponse
	if err := p.client.do(ctx, http.MethodPost, "/transaction/charge_authorization", body, &resp); err != nil {
		return nil, fmt.Errorf("charge authorization: %w", err)
	}
	if resp.Status != "success" {
		return nil, fmt.Errorf("%w: %s", billing.ErrAuthorizationRefused, resp.GatewayResp)
	}
	return &billing.ChargeAuthorizationResult{
		Status:          billing.StatusActive,
		Reference:       resp.Reference,
		PaidAt:          resp.PaidAt,
		PaidAmountMinor: resp.Amount,
		Currency:        resp.Currency,
		PaymentMethod:   paymentMethodFrom(resp.Authorization, resp.Channel),
	}, nil
}
