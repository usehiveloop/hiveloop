// Package fake provides an in-memory billing.Provider for tests. It adheres
// to the billing.Provider contract — production code never imports it.
package fake

import (
	"context"
	"sync"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/billing"
)

// Provider is an in-memory billing.Provider. Tests configure canned responses
// via the public fields and inspect observed calls via the accessor methods.
type Provider struct {
	mu        sync.Mutex
	name      string
	customers map[uuid.UUID]string
	checkouts []billing.CheckoutIntent
	charges   []billing.ChargeAuthorizationRequest

	NextCheckoutURL string
	NextResolveResult *billing.ResolveCheckoutResult
	NextResolveError  error

	NextChargeResult *billing.ChargeAuthorizationResult
	NextChargeError  error
}

// New returns a fake provider registered under the given name.
func New(name string) *Provider {
	return &Provider{
		name:      name,
		customers: map[uuid.UUID]string{},
	}
}

// Name implements billing.Provider.
func (p *Provider) Name() string { return p.name }

// EnsureCustomer implements billing.Provider with a stable id.
func (p *Provider) EnsureCustomer(_ context.Context, orgID uuid.UUID, _, _ string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if id, ok := p.customers[orgID]; ok {
		return id, nil
	}
	id := "cus_fake_" + orgID.String()
	p.customers[orgID] = id
	return id, nil
}

// CreateCheckout implements billing.Provider and records the intent.
func (p *Provider) CreateCheckout(_ context.Context, customerID string, intent billing.CheckoutIntent) (*billing.CheckoutSession, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.checkouts = append(p.checkouts, intent)
	url := p.NextCheckoutURL
	if url == "" {
		url = "https://fake-checkout.example/" + customerID
	}
	ref := "ref_fake_" + uuid.NewString()
	return &billing.CheckoutSession{URL: url, ExternalID: ref, Reference: ref}, nil
}

// ResolveCheckout implements billing.Provider.
func (p *Provider) ResolveCheckout(_ context.Context, _ billing.ResolveCheckoutRequest) (*billing.ResolveCheckoutResult, error) {
	if p.NextResolveError != nil {
		return nil, p.NextResolveError
	}
	if p.NextResolveResult != nil {
		return p.NextResolveResult, nil
	}
	return &billing.ResolveCheckoutResult{Status: billing.StatusActive}, nil
}

// ChargeAuthorization implements billing.Provider and records the call.
func (p *Provider) ChargeAuthorization(_ context.Context, req billing.ChargeAuthorizationRequest) (*billing.ChargeAuthorizationResult, error) {
	p.mu.Lock()
	p.charges = append(p.charges, req)
	p.mu.Unlock()
	if p.NextChargeError != nil {
		return nil, p.NextChargeError
	}
	if p.NextChargeResult != nil {
		return p.NextChargeResult, nil
	}
	return &billing.ChargeAuthorizationResult{
		Status:          billing.StatusActive,
		Reference:       "ref_charge_" + uuid.NewString(),
		PaidAmountMinor: req.AmountMinor,
		Currency:        req.Currency,
	}, nil
}

// Checkouts returns a snapshot of intents CreateCheckout has been called with.
func (p *Provider) Checkouts() []billing.CheckoutIntent {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]billing.CheckoutIntent, len(p.checkouts))
	copy(out, p.checkouts)
	return out
}

// Charges returns a snapshot of charge-authorization requests.
func (p *Provider) Charges() []billing.ChargeAuthorizationRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]billing.ChargeAuthorizationRequest, len(p.charges))
	copy(out, p.charges)
	return out
}
