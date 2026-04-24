package paystack

import (
	"bytes"
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
// this level can also be a number (0 = placeholder), so we deliberately
// do not read it.
type chargeData struct {
	Reference string          `json:"reference"`
	Status    string          `json:"status"`
	Currency  string          `json:"currency"`
	Customer  customerPayload `json:"customer"`
	Plan      json.RawMessage `json:"plan"`
	PaidAt    *time.Time      `json:"paidAt"`
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

func (p *Provider) chargeState(d chargeData, planCode string) (*billing.SubscriptionState, error) {
	orgID, err := extractOrgID(d.Customer.Metadata)
	if err != nil {
		return nil, err
	}
	planSlug := p.planIndex[planCode]
	if planSlug == "" {
		// Charge for a plan_code we don't manage — someone else's plan on
		// the same Paystack account, or a dashboard-created one. Ignore.
		return nil, nil
	}
	state := &billing.SubscriptionState{
		ExternalCustomerID: d.Customer.CustomerCode,
		OrgID:              orgID,
		PlanSlug:           planSlug,
		Status:             billing.StatusActive,
	}
	if d.PaidAt != nil {
		state.CurrentPeriodStart = *d.PaidAt
	}
	return state, nil
}

// invoiceState builds state for invoice.* events, where the customer is a
// sibling of subscription at the data root and no plan object is present.
// PlanSlug is left blank — the handler resolves the plan via the existing
// subscription row keyed by subscription_code.
func (p *Provider) invoiceState(sub subscriptionData, customer customerPayload) (*billing.SubscriptionState, error) {
	orgID, err := extractOrgID(customer.Metadata)
	if err != nil {
		return nil, err
	}
	state := &billing.SubscriptionState{
		ExternalSubscriptionID: sub.SubscriptionCode,
		ExternalCustomerID:     customer.CustomerCode,
		OrgID:                  orgID,
		Status:                 mapSubscriptionStatus(sub.Status),
	}
	if sub.NextPaymentDate != nil {
		state.CurrentPeriodEnd = *sub.NextPaymentDate
	}
	return state, nil
}

// parseChargePlanCode interprets Paystack's variable shape for `data.plan`
// on charge.success events. Observed shapes in the wild (all real):
//
//	(missing) | null | {}      → empty (one-off charge, not a subscription)
//	"PLN_abc"                  → plan_code string directly (older format)
//	{"plan_code":"PLN_abc",…}  → plan_code inside an object (newer format)
//
// Returns the empty string when no usable plan_code can be extracted.
func parseChargePlanCode(raw json.RawMessage) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) || bytes.Equal(trimmed, []byte("{}")) {
		return ""
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return strings.TrimSpace(asString)
	}
	var asObj planPayload
	if err := json.Unmarshal(raw, &asObj); err == nil {
		return asObj.PlanCode
	}
	return ""
}

// extractOrgID pulls the org uuid from customer metadata. EnsureCustomer
// always sets {org_id, org_name} when we create the customer, so every
// event tied to an API-created customer will have this populated. Returns
// (uuid.Nil, nil) when absent — the webhook handler's resolveOrgID falls
// back to looking up the org by subscription_code or customer_code. An
// invalid (non-UUID) org_id is a hard error.
func extractOrgID(meta map[string]string) (uuid.UUID, error) {
	raw, ok := meta["org_id"]
	if !ok || raw == "" {
		return uuid.Nil, nil
	}
	parsed, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid org_id %q: %w", raw, err)
	}
	return parsed, nil
}

// mapSubscriptionStatus normalises Paystack's status strings onto our own
// lifecycle. Paystack uses "non-renewing" while the subscription is
// winding down, and either "complete" (billing cycles exhausted) or
// "cancelled" (user-cancelled at period end) once the subscription is
// fully terminated.
func mapSubscriptionStatus(paystackStatus string) billing.SubscriptionStatus {
	switch strings.ToLower(paystackStatus) {
	case "active", "attention":
		return billing.StatusActive
	case "non-renewing", "cancelled", "canceled":
		return billing.StatusCanceled
	case "complete", "completed":
		return billing.StatusRevoked
	}
	return billing.StatusActive
}
