package paystack

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

type customerCreateRequest struct {
	Email     string            `json:"email"`
	FirstName string            `json:"first_name,omitempty"`
	LastName  string            `json:"last_name,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

type customerCreateResponse struct {
	ID           int64  `json:"id"`
	CustomerCode string `json:"customer_code"`
	Email        string `json:"email"`
}

// EnsureCustomer creates or fetches a Paystack customer for the org.
//
// POST /customer is idempotent on email — when the email already exists
// Paystack returns the existing record (same customer_code) rather than
// erroring. That's exactly the upsert semantics billing.Provider requires.
//
// We attach the org id to the customer's metadata so subscription webhooks
// (which carry the customer object) can resolve back to our org without a
// round-trip to our DB.
func (p *Provider) EnsureCustomer(ctx context.Context, orgID uuid.UUID, email, orgName string) (string, error) {
	req := customerCreateRequest{
		Email: email,
		Metadata: map[string]string{
			"org_id":   orgID.String(),
			"org_name": orgName,
		},
	}
	var resp customerCreateResponse
	if err := p.client.do(ctx, "POST", "/customer", req, &resp); err != nil {
		return "", fmt.Errorf("ensure customer: %w", err)
	}
	if resp.CustomerCode == "" {
		return "", fmt.Errorf("paystack returned empty customer_code")
	}
	return resp.CustomerCode, nil
}
