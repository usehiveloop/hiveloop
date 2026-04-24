package paystack

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/billing"
)

// ParseEvent decodes a Paystack webhook body into the normalized Event shape.
//
// Subscription / charge / invoice events fill in Event.Subscription so the
// shared webhook handler can upsert state. Events we don't care about
// (transfer.*, customer.*, etc.) return EventUnhandled with no subscription
// state attached — the handler ignores them.
func (p *Provider) ParseEvent(body []byte) (billing.Event, error) {
	var env envelopeEvent
	if err := json.Unmarshal(body, &env); err != nil {
		return billing.Event{}, fmt.Errorf("paystack: decode envelope: %w", err)
	}

	evType := mapEventType(env.Event)
	if evType == billing.EventUnhandled {
		return billing.Event{Type: evType, RawProviderType: env.Event}, nil
	}

	state, err := p.buildState(env.Event, env.Data)
	if err != nil {
		return billing.Event{}, fmt.Errorf("paystack event %q: %w", env.Event, err)
	}

	return billing.Event{
		Type:            evType,
		Subscription:    state,
		RawProviderType: env.Event,
	}, nil
}

// envelopeEvent is the outer webhook payload: {"event": "...", "data": {...}}.
type envelopeEvent struct {
	Event string          `json:"event"`
	Data  json.RawMessage `json:"data"`
}

// mapEventType maps Paystack event names to our normalized EventType.
//
// Rule-of-thumb: charge.success is our single source of truth for "credits
// should be granted" so we avoid double-counting on initial signup (where
// both subscription.create and charge.success fire). The subscription.*
// events drive subscription row upsert/status changes.
func mapEventType(paystackEvent string) billing.EventType {
	switch paystackEvent {
	case "subscription.create":
		return billing.EventSubscriptionActivated
	case "subscription.disable":
		return billing.EventSubscriptionCanceled
	case "subscription.not_renew":
		return billing.EventSubscriptionRevoked
	case "charge.success":
		return billing.EventInvoicePaid
	case "invoice.payment_failed":
		return billing.EventPaymentFailed
	}
	return billing.EventUnhandled
}

// --- JSON payload shapes ---

type customerPayload struct {
	CustomerCode string            `json:"customer_code"`
	Email        string            `json:"email"`
	Metadata     map[string]string `json:"metadata"`
}

type planPayload struct {
	PlanCode string `json:"plan_code"`
	Interval string `json:"interval"` // "monthly", "annually"
}

type subscriptionData struct {
	SubscriptionCode string          `json:"subscription_code"`
	Status           string          `json:"status"`
	NextPaymentDate  *time.Time      `json:"next_payment_date"`
	CreatedAt        *time.Time      `json:"createdAt"`
	Customer         customerPayload `json:"customer"`
	Plan             planPayload     `json:"plan"`
}

type chargeData struct {
	Reference string            `json:"reference"`
	Status    string            `json:"status"`
	Currency  string            `json:"currency"`
	Customer  customerPayload   `json:"customer"`
	Plan      *planPayload      `json:"plan"` // null for non-subscription charges
	Metadata  map[string]string `json:"metadata"`
	PaidAt    *time.Time        `json:"paidAt"`
}

type invoiceData struct {
	Subscription subscriptionData `json:"subscription"`
	Customer     customerPayload  `json:"customer"`
}

// buildState decodes the data blob into a SubscriptionState tailored to the
// event shape. Returns (nil, nil) for charge.success without a plan — those
// are one-off charges we don't care about here.
func (p *Provider) buildState(eventName string, data json.RawMessage) (*billing.SubscriptionState, error) {
	switch eventName {
	case "subscription.create", "subscription.disable", "subscription.not_renew":
		var d subscriptionData
		if err := json.Unmarshal(data, &d); err != nil {
			return nil, err
		}
		return p.subscriptionState(d)
	case "charge.success":
		var d chargeData
		if err := json.Unmarshal(data, &d); err != nil {
			return nil, err
		}
		if d.Plan == nil {
			return nil, nil // one-off charge, not a subscription renewal
		}
		return p.chargeState(d)
	case "invoice.payment_failed":
		var d invoiceData
		if err := json.Unmarshal(data, &d); err != nil {
			return nil, err
		}
		return p.subscriptionState(d.Subscription)
	}
	return nil, fmt.Errorf("unhandled event shape: %s", eventName)
}

func (p *Provider) subscriptionState(d subscriptionData) (*billing.SubscriptionState, error) {
	orgID, err := extractOrgID(d.Customer.Metadata)
	if err != nil {
		return nil, err
	}
	state := &billing.SubscriptionState{
		ExternalSubscriptionID: d.SubscriptionCode,
		ExternalCustomerID:     d.Customer.CustomerCode,
		OrgID:                  orgID,
		PlanSlug:               p.planIndex[d.Plan.PlanCode],
		Status:                 mapSubscriptionStatus(d.Status),
	}
	if d.CreatedAt != nil {
		state.CurrentPeriodStart = *d.CreatedAt
	}
	if d.NextPaymentDate != nil {
		state.CurrentPeriodEnd = *d.NextPaymentDate
	}
	return state, nil
}

func (p *Provider) chargeState(d chargeData) (*billing.SubscriptionState, error) {
	orgID, err := extractOrgID(d.Customer.Metadata)
	if err != nil {
		return nil, err
	}
	state := &billing.SubscriptionState{
		ExternalCustomerID: d.Customer.CustomerCode,
		OrgID:              orgID,
		PlanSlug:           p.planIndex[d.Plan.PlanCode],
		Status:             billing.StatusActive,
	}
	if d.PaidAt != nil {
		state.CurrentPeriodStart = *d.PaidAt
	}
	return state, nil
}

// extractOrgID pulls the org uuid from a customer's metadata. Paystack echoes
// customer metadata onto every event that includes the customer object.
func extractOrgID(meta map[string]string) (uuid.UUID, error) {
	raw, ok := meta["org_id"]
	if !ok {
		return uuid.Nil, fmt.Errorf("missing org_id in customer metadata")
	}
	parsed, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid org_id %q: %w", raw, err)
	}
	return parsed, nil
}

// mapSubscriptionStatus normalises Paystack's status strings onto our own
// lifecycle. Paystack uses "non-renewing" and "completed" where we use
// canceled/revoked respectively.
func mapSubscriptionStatus(paystackStatus string) billing.SubscriptionStatus {
	switch strings.ToLower(paystackStatus) {
	case "active", "attention":
		return billing.StatusActive
	case "non-renewing", "cancelled", "canceled":
		return billing.StatusCanceled
	case "completed":
		return billing.StatusRevoked
	}
	return billing.StatusActive
}
