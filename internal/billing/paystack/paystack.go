// Package paystack implements billing.Provider against the Paystack API.
//
// Files are split by responsibility:
//   paystack.go  — Provider type, constructor, compile-time interface check
//   config.go    — Config, PlanRegistry, plan_code lookup
//   client.go    — Low-level HTTP client for api.paystack.co
//   customer.go  — EnsureCustomer
//   checkout.go  — CreateCheckout (POST /transaction/initialize with a plan)
//   portal.go    — CreatePortal (Paystack's per-subscription manage link)
//   webhook.go   — VerifyWebhook (HMAC-SHA512 of raw body)
//   events.go    — ParseEvent + Paystack→billing event mapping
package paystack

import "github.com/usehiveloop/hiveloop/internal/billing"

// Name is the stable slug used in webhook URLs and the subscriptions.provider
// column.
const Name = "paystack"

// Provider implements billing.Provider against Paystack.
//
// It is safe for concurrent use; the underlying HTTP client is shared.
type Provider struct {
	cfg       Config
	client    *client
	planIndex map[string]string // paystack plan_code -> our plan slug
}

// New constructs a Paystack provider.
//
// The caller configures credentials and plan mappings via Config. New builds
// a reverse index (plan_code → slug) for webhook event parsing so we don't
// scan the registry on every event.
func New(cfg Config) *Provider {
	return &Provider{
		cfg:       cfg,
		client:    newClient(cfg.SecretKey),
		planIndex: cfg.Plans.reverseIndex(),
	}
}

// Name returns the stable provider slug.
func (p *Provider) Name() string { return Name }

// Compile-time guarantee the adapter satisfies billing.Provider. This trips
// the build if a method signature drifts from the interface.
var _ billing.Provider = (*Provider)(nil)
