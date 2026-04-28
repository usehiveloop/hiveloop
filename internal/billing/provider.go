// Package billing is the provider-agnostic billing layer.
//
// Real providers (Stripe, Paystack, Paddle, etc.) live in sub-packages and
// implement the Provider interface. A fake provider for tests lives in
// internal/billing/fake. The rest of the codebase only ever talks to the
// Provider interface and the credit ledger service — never to a concrete
// provider SDK.
package billing

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// Cycle is the billing cadence the user chose at checkout.
type Cycle string

const (
	CycleMonthly Cycle = "monthly"
	CycleAnnual  Cycle = "annual"
)

// IsValid reports whether c is one of the recognised cycles.
func (c Cycle) IsValid() bool { return c == CycleMonthly || c == CycleAnnual }

// Well-known currency codes. Providers should treat currency as an opaque
// ISO-4217 string — these constants are purely a convenience.
const (
	CurrencyUSD = "USD"
	CurrencyNGN = "NGN"
)

// Provider-agnostic errors. Adapters should wrap these with %w so callers can
// errors.Is them regardless of which provider raised the issue.
var (
	ErrUnsupportedCurrency  = errors.New("billing: currency not supported by this provider")
	ErrUnknownPlan          = errors.New("billing: plan slug not configured for this provider")
	ErrNoActiveSubscription = errors.New("billing: no active subscription for this org")
)

// CheckoutIntent describes a user's intent to subscribe to a plan. Providers
// translate it into their native checkout-session shape.
type CheckoutIntent struct {
	OrgID         uuid.UUID
	OrgName       string
	CustomerEmail string
	PlanSlug      string
	AmountMinor   int64 // smallest unit of Currency (kobo for NGN, cents for USD); pinned to the plan's price at checkout time
	Currency      string // ISO-4217, e.g. "USD", "NGN"
	Cycle         Cycle
	SuccessURL    string
	CancelURL     string
	Metadata      map[string]string
}

// CheckoutSession is a normalized provider response. Every provider hands us
// back at least a URL to redirect the browser to. AccessCode is populated
// when the provider supports a popup/inline flow that resumes a server-
// initialised transaction (Paystack: resumeTransaction(access_code)).
type CheckoutSession struct {
	URL        string
	ExternalID string
	AccessCode string
}

// PortalRequest carries everything a provider might need to build a portal
// URL. Adapters read whichever fields they care about; Stripe uses the
// customer id, Paystack uses the subscription id, etc.
type PortalRequest struct {
	OrgID                  uuid.UUID
	ExternalCustomerID     string
	ExternalSubscriptionID string
}

// PortalSession is a URL into the provider's self-serve billing portal, or an
// equivalent hosted page when the provider lacks a full portal.
type PortalSession struct {
	URL string
}

type ResolveCheckoutRequest struct {
	Reference     string
	ExpectedOrgID uuid.UUID
}

type ResolveCheckoutResult struct {
	Status                 SubscriptionStatus
	PlanSlug               string
	ExternalSubscriptionID string
	ExternalCustomerID     string
	CurrentPeriodStart     time.Time
	CurrentPeriodEnd       time.Time

	ChargeReference   string
	ChargeAmount      int64
	ChargedAt         *time.Time
	CardLast4         string
	CardBrand         string
	CardExpMonth      string
	CardExpYear       string
	AuthorizationCode string
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

	// Payment-method snapshot — only populated by charge events. Empty on
	// subscription.* events. The webhook handler writes these onto the
	// Subscription row so the UI doesn't have to round-trip the provider.
	ChargeReference   string
	ChargeAmount      int64 // minor units
	ChargedAt         *time.Time
	CardLast4         string
	CardBrand         string
	CardExpMonth      string
	CardExpYear       string
	AuthorizationCode string
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
	// for the org. Implementations must be idempotent on email or org id.
	EnsureCustomer(ctx context.Context, orgID uuid.UUID, email, orgName string) (string, error)

	// CreateCheckout returns a URL the browser should redirect to. Adapters
	// that can't satisfy the requested currency or plan should return the
	// matching ErrUnsupportedCurrency / ErrUnknownPlan sentinel.
	CreateCheckout(ctx context.Context, customerID string, intent CheckoutIntent) (*CheckoutSession, error)

	// CreatePortal returns a URL into the provider's customer portal — or the
	// nearest equivalent the provider offers (e.g. Paystack's per-subscription
	// manage-link). Adapters that require an active subscription should
	// return ErrNoActiveSubscription when req.ExternalSubscriptionID is empty.
	CreatePortal(ctx context.Context, req PortalRequest) (*PortalSession, error)

	// ResolveCheckout queries the provider synchronously for the state of a
	// checkout. Used by /v1/billing/verify to confirm a popup-flow charge
	// completed without waiting on a webhook.
	ResolveCheckout(ctx context.Context, req ResolveCheckoutRequest) (*ResolveCheckoutResult, error)

	// VerifyWebhook checks the signature on an incoming webhook request. The
	// request body is already read into body — implementations must not read
	// r.Body. Return a non-nil error to reject the request.
	VerifyWebhook(r *http.Request, body []byte) error

	// ParseEvent decodes an already-verified webhook body into a normalized
	// Event. Unrecognized provider event types should map to EventUnhandled.
	ParseEvent(body []byte) (Event, error)
}
