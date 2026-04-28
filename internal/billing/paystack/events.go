package paystack

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/usehiveloop/hiveloop/internal/billing"
)

// ParseEvent decodes a Paystack webhook body into the normalized Event shape.
//
// Subscription / charge / invoice events fill in Event.Subscription so the
// shared webhook handler can upsert state. Events we don't care about
// (transfer.*, customer.*, invoice.create, invoice.update …) return
// EventUnhandled with no subscription state attached.
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
// events drive subscription row upsert/status changes. invoice.create /
// invoice.update are intentionally ignored — the useful signal (payment
// succeeded) already comes through charge.success.
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

// chargeData mirrors the root of `data` on charge.success. Paystack's
// `plan` field arrives in one of four shapes — see parseChargePlanCode —
// so we keep it as a raw message and decode by inspection. `metadata` at
// this level can be a number (0 = placeholder) or a JSON object — we
// keep it as a raw message and only decode when it parses as a string-map.
// This is where Paystack echoes the metadata we passed to
// /transaction/initialize, so it's the canonical place to find org_id
// for popup-flow charges (customer.metadata is often null on charge
// events even when the customer record itself carries metadata).
type chargeData struct {
	Reference     string             `json:"reference"`
	Status        string             `json:"status"`
	Currency      string             `json:"currency"`
	Amount        int64              `json:"amount"` // minor units (kobo for NGN)
	Customer      customerPayload    `json:"customer"`
	Plan          json.RawMessage    `json:"plan"`
	Metadata      json.RawMessage    `json:"metadata"`
	PaidAt        *time.Time         `json:"paidAt"`
	Authorization authorizationBlock `json:"authorization"`
}

type authorizationBlock struct {
	AuthorizationCode string `json:"authorization_code"`
	Last4             string `json:"last4"`
	Brand             string `json:"brand"`
	CardType          string `json:"card_type"`
	Bank              string `json:"bank"`
	ExpMonth          string `json:"exp_month"`
	ExpYear           string `json:"exp_year"`
	Reusable          bool   `json:"reusable"`
}

// invoiceData mirrors invoice.* events. The customer sits at the data root,
// not inside the subscription object — unlike subscription.* events.
type invoiceData struct {
	Subscription subscriptionData `json:"subscription"`
	Customer     customerPayload  `json:"customer"`
}

// buildState decodes the data blob into a SubscriptionState tailored to the
// event shape. Returns (nil, nil) for charge.success without a plan — those
// are one-off charges we don't care about here — and for charges tied to a
// plan_code we don't manage.
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
		planCode := parseChargePlanCode(d.Plan)
		if planCode == "" {
			return nil, nil
		}
		return p.chargeState(d, planCode)
	case "invoice.payment_failed":
		var d invoiceData
		if err := json.Unmarshal(data, &d); err != nil {
			return nil, err
		}
		return p.invoiceState(d.Subscription, d.Customer)
	}
	return nil, fmt.Errorf("unhandled event shape: %s", eventName)
}

