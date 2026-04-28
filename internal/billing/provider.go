// Package billing is the provider-agnostic billing layer. We manage
// subscription lifecycle ourselves and use providers purely to charge a
// saved payment method.
package billing

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

type Cycle string

const (
	CycleMonthly Cycle = "monthly"
	CycleAnnual  Cycle = "annual"
)

func (c Cycle) IsValid() bool { return c == CycleMonthly || c == CycleAnnual }

const (
	CurrencyUSD = "USD"
	CurrencyNGN = "NGN"
)

// Adapters wrap these with %w so callers can errors.Is regardless of provider.
var (
	ErrUnsupportedCurrency  = errors.New("billing: currency not supported by this provider")
	ErrUnknownPlan          = errors.New("billing: plan slug not configured for this provider")
	ErrNoActiveSubscription = errors.New("billing: no active subscription for this org")
	ErrAuthorizationRefused = errors.New("billing: provider declined the saved authorization")
)

// PaymentChannel restricts subscriptions to channels that issue a reusable
// AuthorizationCode. USSD, mobile money, QR are one-shot and rejected.
type PaymentChannel string

const (
	ChannelCard PaymentChannel = "card"
	ChannelBank PaymentChannel = "bank"
)

func (c PaymentChannel) IsReusable() bool {
	return c == ChannelCard || c == ChannelBank
}

type CheckoutIntent struct {
	OrgID         uuid.UUID
	OrgName       string
	CustomerEmail string
	PlanSlug      string
	AmountMinor   int64
	Currency      string
	Cycle         Cycle
	SuccessURL    string
	CancelURL     string
	Metadata      map[string]string
}

// CheckoutSession.AccessCode is set when the provider supports a popup
// flow (Paystack: resumeTransaction(access_code)).
type CheckoutSession struct {
	URL        string
	ExternalID string
	AccessCode string
	Reference  string
}

type PortalRequest struct {
	OrgID                  uuid.UUID
	ExternalCustomerID     string
	ExternalSubscriptionID string
}

type PortalSession struct {
	URL string
}

type ResolveCheckoutRequest struct {
	Reference     string
	ExpectedOrgID uuid.UUID
}

// PaymentMethod is the reusable payment instrument; AuthorizationCode is
// what we re-charge against, the rest is for "Visa ending 4242" UI.
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

// Metadata is the string-keyed map we passed when initialising the
// transaction, echoed back. Carries plan_slug for fresh-checkout verify.
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

type ChargeAuthorizationRequest struct {
	Email             string
	AuthorizationCode string
	AmountMinor       int64
	Currency          string
	Reference         string
	Metadata          map[string]string
}

type ChargeAuthorizationResult struct {
	Status          SubscriptionStatus
	Reference       string
	PaidAt          *time.Time
	PaidAmountMinor int64
	Currency        string
	PaymentMethod   PaymentMethod
}

type SubscriptionStatus string

const (
	StatusActive   SubscriptionStatus = "active"
	StatusCanceled SubscriptionStatus = "canceled"
	StatusPastDue  SubscriptionStatus = "past_due"
	StatusRevoked  SubscriptionStatus = "revoked"
)

// Provider methods run inside HTTP request handlers; must be safe for
// concurrent use. EnsureCustomer must be idempotent on email/org id.
// ChargeAuthorization returns ErrAuthorizationRefused on a declined charge.
type Provider interface {
	Name() string
	EnsureCustomer(ctx context.Context, orgID uuid.UUID, email, orgName string) (string, error)
	CreateCheckout(ctx context.Context, customerID string, intent CheckoutIntent) (*CheckoutSession, error)
	CreatePortal(ctx context.Context, req PortalRequest) (*PortalSession, error)
	ResolveCheckout(ctx context.Context, req ResolveCheckoutRequest) (*ResolveCheckoutResult, error)
	ChargeAuthorization(ctx context.Context, req ChargeAuthorizationRequest) (*ChargeAuthorizationResult, error)
}
