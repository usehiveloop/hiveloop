// Package fake provides an in-memory billing.Provider for tests. It adheres
// to the billing.Provider contract — production code never imports it.
package fake

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/billing"
)

// Provider is an in-memory billing.Provider. Tests configure the canned
// responses via the public fields and inspect the observed calls via the
// accessor methods.
type Provider struct {
	mu        sync.Mutex
	name      string
	customers map[uuid.UUID]string
	checkouts []billing.CheckoutIntent

	// SignatureHeader, when set, forces VerifyWebhook to require the header's
	// value equal SignatureValue. Empty means "accept everything".
	SignatureHeader string
	SignatureValue  string

	// NextCheckoutURL overrides the URL returned by CreateCheckout. Empty uses
	// a generated placeholder.
	NextCheckoutURL string

	// NextPortalURL overrides the URL returned by CreatePortal.
	NextPortalURL string
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

// EnsureCustomer implements billing.Provider. Returns a stable id derived from
// the org id.
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
	return &billing.CheckoutSession{URL: url, ExternalID: "ch_" + uuid.NewString()}, nil
}

// CreatePortal implements billing.Provider.
func (p *Provider) CreatePortal(_ context.Context, customerID string) (*billing.PortalSession, error) {
	url := p.NextPortalURL
	if url == "" {
		url = "https://fake-portal.example/" + customerID
	}
	return &billing.PortalSession{URL: url}, nil
}

// VerifyWebhook implements billing.Provider. Returns nil (accept) when no
// signature check is configured.
func (p *Provider) VerifyWebhook(r *http.Request, _ []byte) error {
	if p.SignatureHeader == "" {
		return nil
	}
	if r.Header.Get(p.SignatureHeader) != p.SignatureValue {
		return errors.New("fake: invalid signature")
	}
	return nil
}

// Event is the JSON body the fake provider decodes. Tests emit this shape to
// exercise the webhook handler.
type Event struct {
	Type         billing.EventType          `json:"type"`
	Subscription *billing.SubscriptionState `json:"subscription,omitempty"`
}

// ParseEvent implements billing.Provider.
func (p *Provider) ParseEvent(body []byte) (billing.Event, error) {
	var ev Event
	if err := json.Unmarshal(body, &ev); err != nil {
		return billing.Event{}, err
	}
	return billing.Event{
		Type:            ev.Type,
		Subscription:    ev.Subscription,
		RawProviderType: string(ev.Type),
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
