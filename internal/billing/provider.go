// Package billing is the provider-agnostic billing layer.
//
// Real providers (Stripe, Paddle, Lemon Squeezy, etc.) live in sub-packages
// and implement the Provider interface. A fake provider for tests lives in
// internal/billing/fake. The rest of the codebase only ever talks to the
// Provider interface and the credit ledger service — never to a concrete
// provider SDK.
package billing

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// CheckoutIntent describes a user's intent to subscribe to a plan. Providers
// translate it into their native checkout-session shape.
type CheckoutIntent struct {
	OrgID         uuid.UUID
	OrgName       string
	CustomerEmail string
	PlanSlug      string
	SuccessURL    string
	CancelURL     string
	Metadata      map[string]string
}

// CheckoutSession is a normalized provider response. Every provider hands us
// back at least a URL to redirect the browser to.
type CheckoutSession struct {
	URL        string
	ExternalID string
}

// PortalSession is a URL into the provider's self-serve billing portal.
type PortalSession struct {
	URL string
}

// EventType is the provider-agnostic webhook event type the rest of the
// codebase reacts to. Provider implementations map their native event names
// onto these.
type EventType string

const (
	EventSubscriptionActivated EventType = "subscription.activated"
	EventSubscriptionUpdated   EventType = "subscription.updated"
	EventSubscriptionCanceled  EventType = "subscription.canceled"
	EventSubscriptionRevoked   EventType = "subscription.revoked"
	EventInvoicePaid           EventType = "invoice.paid"
	EventPaymentFailed         EventType = "payment.failed"
	EventUnhandled             EventType = "unhandled"
)

// SubscriptionStatus is the normalized subscription state.
type SubscriptionStatus string

const (
	StatusActive   SubscriptionStatus = "active"
	StatusCanceled SubscriptionStatus = "canceled"
	StatusPastDue  SubscriptionStatus = "past_due"
	StatusRevoked  SubscriptionStatus = "revoked"
)

// SubscriptionState is the normalized subscription info lifted from a webhook.
type SubscriptionState struct {
	ExternalSubscriptionID string
	ExternalCustomerID     string
	OrgID                  uuid.UUID
	PlanSlug               string
	Status                 SubscriptionStatus
	CurrentPeriodStart     time.Time
	CurrentPeriodEnd       time.Time
	CanceledAt             *time.Time
}

// Event is a normalized webhook event.
type Event struct {
	Type            EventType
	Subscription    *SubscriptionState
	RawProviderType string
}

// Provider is the contract every payment provider implementation must satisfy.
// Provider methods run during HTTP request handling, so they must be safe for
// concurrent use.
type Provider interface {
	// Name is the stable slug used in webhook URLs (/internal/webhooks/:provider)
	// and stored in the subscriptions.provider column.
	Name() string

	// EnsureCustomer creates or returns the provider's external customer id
	// for the org. Implementations should be idempotent.
	EnsureCustomer(ctx context.Context, orgID uuid.UUID, email, orgName string) (string, error)

	// CreateCheckout returns a URL the browser should redirect to.
	CreateCheckout(ctx context.Context, customerID string, intent CheckoutIntent) (*CheckoutSession, error)

	// CreatePortal returns a URL into the provider's customer portal.
	CreatePortal(ctx context.Context, customerID string) (*PortalSession, error)

	// VerifyWebhook checks the signature on an incoming webhook request. The
	// request body is already read into body — implementations must not read
	// r.Body. Return a non-nil error to reject the request.
	VerifyWebhook(r *http.Request, body []byte) error

	// ParseEvent decodes an already-verified webhook body into a normalized
	// Event. Unrecognized provider event types should map to EventUnhandled.
	ParseEvent(body []byte) (Event, error)
}
