// Package billing is the provider-agnostic billing layer.
//
// We manage subscription lifecycle (period tracking, upgrades, downgrades,
// cancellations, proration) ourselves and use providers purely to charge
// a saved payment method. Provider implementations live in sub-packages
// (paystack, fake) and adhere to the Provider interface.
package billing

import (
	"context"
	"errors"
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
	ErrAuthorizationRefused = errors.New("billing: provider declined the saved authorization")
)

// PaymentChannel is one of the channel kinds we accept for subscription
// renewals. Only "card" and "bank" produce a reusable AuthorizationCode
// from Paystack; other channels (USSD, mobile money, QR) are one-shot
// and not supported on subscription rows.
type PaymentChannel string

const (
	ChannelCard PaymentChannel = "card"
	ChannelBank PaymentChannel = "bank"
)

// IsReusable reports whether c is a channel we consider eligible for
// off-session re-charging.
func (c PaymentChannel) IsReusable() bool {
	return c == ChannelCard || c == ChannelBank
}

// CheckoutIntent describes a user's intent to subscribe to a plan. Providers
// translate it into their native checkout-session shape.
type CheckoutIntent struct {
	OrgID         uuid.UUID
	OrgName       string
	CustomerEmail string
	PlanSlug      string
	AmountMinor   int64  // smallest unit of Currency (kobo for NGN, cents for USD)
	Currency      string // ISO-4217, e.g. "USD", "NGN"
	Cycle         Cycle
	SuccessURL    string
	CancelURL     string
	Metadata      map[string]string
}

// CheckoutSession is a normalized provider response. AccessCode is populated
// when the provider supports a popup/inline flow that resumes a server-
// initialised transaction (Paystack: resumeTransaction(access_code)).
type CheckoutSession struct {
	URL        string
	ExternalID string
	AccessCode string
	Reference  string
}

// PortalRequest carries everything a provider might need to build a portal
// URL. Adapters read whichever fields they care about.
type PortalRequest struct {
	OrgID                  uuid.UUID
	ExternalCustomerID     string
	ExternalSubscriptionID string
}

// PortalSession is a URL into the provider's self-serve billing portal.
type PortalSession struct {
	URL string
}

// ResolveCheckoutRequest looks up the state of a started transaction.
type ResolveCheckoutRequest struct {
	Reference     string
	ExpectedOrgID uuid.UUID
}

// PaymentMethod captures the reusable payment instrument Paystack returned
// after a successful charge. Subscription renewals charge against
// AuthorizationCode; the rest is for the UI to display "Visa ending in
// 4242" or "GTBank — Jane Doe" without re-fetching from the provider.
type PaymentMethod struct {
	AuthorizationCode string
	Channel           PaymentChannel
	CardLast4         string
	CardBrand         string
	CardExpMonth      string
	CardExpYear       string
	BankName          string
	AccountName       string
}

// ResolveCheckoutResult is the verified state of a Paystack transaction.
// Status reports the upstream payment status; PaidAmountMinor and Currency
// let callers verify the customer paid what we quoted.
//
// Metadata is the string-keyed map we passed when initialising the
// transaction, echoed back by the provider. We use it to carry plan_slug
// (fresh checkout) or quote_id (change apply) end-to-end so verify can
// look up what the customer was supposed to be paying for.
type ResolveCheckoutResult struct {
	Status             SubscriptionStatus
	ExternalCustomerID string
	PaidAt             *time.Time
	PaidAmountMinor    int64
	Currency           string
	Reference          string
	PaymentMethod      PaymentMethod
	Metadata           map[string]string
}

// ChargeAuthorizationRequest re-charges a saved payment method off-session.
type ChargeAuthorizationRequest struct {
	Email             string
	AuthorizationCode string
	AmountMinor       int64
	Currency          string
	Reference         string // optional — let the provider generate one when empty
	Metadata          map[string]string
}

// ChargeAuthorizationResult mirrors a successful re-charge.
type ChargeAuthorizationResult struct {
	Status          SubscriptionStatus
	Reference       string
	PaidAt          *time.Time
	PaidAmountMinor int64
	Currency        string
	PaymentMethod   PaymentMethod
}

// SubscriptionStatus is the normalized subscription state.
type SubscriptionStatus string

const (
	StatusActive   SubscriptionStatus = "active"
	StatusCanceled SubscriptionStatus = "canceled"
	StatusPastDue  SubscriptionStatus = "past_due"
	StatusRevoked  SubscriptionStatus = "revoked"
)

// Provider is the contract every payment provider implementation must satisfy.
// Methods run inside HTTP request handlers and must be safe for concurrent use.
type Provider interface {
	// Name is the stable slug stored in subscriptions.provider.
	Name() string

	// EnsureCustomer creates or returns the provider's external customer id
	// for the org. Implementations must be idempotent on email or org id.
	EnsureCustomer(ctx context.Context, orgID uuid.UUID, email, orgName string) (string, error)

	// CreateCheckout initialises a fresh transaction the browser can resume
	// via a popup. Adapters that can't satisfy the requested currency or
	// plan should return the matching sentinel error.
	CreateCheckout(ctx context.Context, customerID string, intent CheckoutIntent) (*CheckoutSession, error)

	// CreatePortal returns a URL into the provider's customer portal — or
	// the nearest equivalent the provider offers.
	CreatePortal(ctx context.Context, req PortalRequest) (*PortalSession, error)

	// ResolveCheckout queries the provider for the state of a transaction
	// reference, used by /v1/billing/verify and /apply-change to confirm a
	// charge completed and capture the saved authorization.
	ResolveCheckout(ctx context.Context, req ResolveCheckoutRequest) (*ResolveCheckoutResult, error)

	// ChargeAuthorization re-charges a saved authorization off-session.
	// Returns ErrAuthorizationRefused on a declined charge.
	ChargeAuthorization(ctx context.Context, req ChargeAuthorizationRequest) (*ChargeAuthorizationResult, error)
}
