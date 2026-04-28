package paystack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/usehiveloop/hiveloop/internal/billing"
)

type customerPayload struct {
	CustomerCode string `json:"customer_code"`
	Email        string `json:"email"`
}

type authorizationBlock struct {
	AuthorizationCode string `json:"authorization_code"`
	Last4             string `json:"last4"`
	Brand             string `json:"brand"`
	CardType          string `json:"card_type"`
	Bank              string `json:"bank"`
	AccountName       string `json:"account_name"`
	ExpMonth          string `json:"exp_month"`
	ExpYear           string `json:"exp_year"`
	Channel           string `json:"channel"`
	Reusable          bool   `json:"reusable"`
}

type verifyTransactionResponse struct {
	Reference     string             `json:"reference"`
	Status        string             `json:"status"`
	Amount        int64              `json:"amount"`
	Currency      string             `json:"currency"`
	Channel       string             `json:"channel"`
	PaidAt        *time.Time         `json:"paid_at"`
	Customer      customerPayload    `json:"customer"`
	Authorization authorizationBlock `json:"authorization"`
	// Paystack returns metadata in one of three shapes — null, the
	// number 0 (their placeholder), or a JSON object — so we keep it
	// raw and decode by inspection.
	Metadata json.RawMessage `json:"metadata"`
}

// ResolveCheckout calls /transaction/verify/:reference and returns the
// normalized result. Callers verify the paid amount against the plan
// price themselves — this method is purely a transport.
func (p *Provider) ResolveCheckout(ctx context.Context, req billing.ResolveCheckoutRequest) (*billing.ResolveCheckoutResult, error) {
	if req.Reference == "" {
		return nil, fmt.Errorf("paystack resolve: empty reference")
	}

	var tx verifyTransactionResponse
	if err := p.client.do(ctx, http.MethodGet, "/transaction/verify/"+url.PathEscape(req.Reference), nil, &tx); err != nil {
		return nil, fmt.Errorf("verify transaction: %w", err)
	}

	return &billing.ResolveCheckoutResult{
		Status:             mapTransactionStatus(tx.Status),
		ExternalCustomerID: tx.Customer.CustomerCode,
		PaidAt:             tx.PaidAt,
		PaidAmountMinor:    tx.Amount,
		Currency:           tx.Currency,
		Reference:          tx.Reference,
		PaymentMethod:      paymentMethodFrom(tx.Authorization, tx.Channel),
		Metadata:           parseMetadata(tx.Metadata),
	}, nil
}

// parseMetadata decodes the metadata Paystack echoes back from the
// /transaction/initialize call. Their API uses three shapes here — null,
// the literal number 0 (their "no metadata" placeholder), or a JSON
// object — so we accept all of them and ignore unknown shapes.
func parseMetadata(raw json.RawMessage) map[string]string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) || bytes.Equal(trimmed, []byte("0")) {
		return nil
	}
	var m map[string]string
	if err := json.Unmarshal(trimmed, &m); err != nil {
		return nil
	}
	return m
}

func paymentMethodFrom(auth authorizationBlock, txChannel string) billing.PaymentMethod {
	channel := billing.PaymentChannel(auth.Channel)
	if channel == "" {
		channel = billing.PaymentChannel(txChannel)
	}
	return billing.PaymentMethod{
		AuthorizationCode: auth.AuthorizationCode,
		Channel:           channel,
		CardLast4:         auth.Last4,
		CardBrand:         auth.Brand,
		CardExpMonth:      auth.ExpMonth,
		CardExpYear:       auth.ExpYear,
		BankName:          auth.Bank,
		AccountName:       auth.AccountName,
	}
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
