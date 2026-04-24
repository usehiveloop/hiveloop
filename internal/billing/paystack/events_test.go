package paystack

import (
	"fmt"
	"testing"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/billing"
)

func providerForEvents(t *testing.T) *Provider {
	t.Helper()
	return New(Config{
		SecretKey: "sk_test",
		Plans:     sampleRegistry(),
	})
}

func TestParseEvent_SubscriptionCreate(t *testing.T) {
	p := providerForEvents(t)
	orgID := uuid.New()
	body := []byte(fmt.Sprintf(`{
		"event": "subscription.create",
		"data": {
			"subscription_code": "SUB_123",
			"status": "active",
			"next_payment_date": "2026-05-24T00:00:00.000Z",
			"createdAt": "2026-04-24T00:00:00.000Z",
			"customer": {
				"customer_code": "CUS_abc",
				"metadata": {"org_id": "%s"}
			},
			"plan": {"plan_code": "PLN_pro_ngn_m", "interval": "monthly"}
		}
	}`, orgID))

	ev, err := p.ParseEvent(body)
	if err != nil {
		t.Fatalf("ParseEvent: %v", err)
	}
	if ev.Type != billing.EventSubscriptionActivated {
		t.Fatalf("Type = %s, want subscription.activated", ev.Type)
	}
	if ev.Subscription == nil {
		t.Fatal("expected Subscription state")
	}
	if ev.Subscription.OrgID != orgID {
		t.Errorf("OrgID mismatch: %s vs %s", ev.Subscription.OrgID, orgID)
	}
	if ev.Subscription.ExternalSubscriptionID != "SUB_123" {
		t.Errorf("sub id = %s", ev.Subscription.ExternalSubscriptionID)
	}
	if ev.Subscription.PlanSlug != "pro" {
		t.Errorf("plan slug = %q, want pro", ev.Subscription.PlanSlug)
	}
	if ev.Subscription.Status != billing.StatusActive {
		t.Errorf("status = %s", ev.Subscription.Status)
	}
}

func TestParseEvent_SubscriptionDisable(t *testing.T) {
	p := providerForEvents(t)
	orgID := uuid.New()
	body := []byte(fmt.Sprintf(`{
		"event": "subscription.disable",
		"data": {
			"subscription_code": "SUB_321",
			"status": "cancelled",
			"customer": {"customer_code": "CUS_q", "metadata": {"org_id": "%s"}},
			"plan": {"plan_code": "PLN_pro_ngn_a"}
		}
	}`, orgID))

	ev, err := p.ParseEvent(body)
	if err != nil {
		t.Fatalf("ParseEvent: %v", err)
	}
	if ev.Type != billing.EventSubscriptionCanceled {
		t.Fatalf("Type = %s, want subscription.canceled", ev.Type)
	}
	if ev.Subscription.Status != billing.StatusCanceled {
		t.Errorf("Status = %s, want canceled", ev.Subscription.Status)
	}
}

func TestParseEvent_ChargeSuccessWithPlan(t *testing.T) {
	p := providerForEvents(t)
	orgID := uuid.New()
	body := []byte(fmt.Sprintf(`{
		"event": "charge.success",
		"data": {
			"reference": "ref_1",
			"status": "success",
			"currency": "NGN",
			"customer": {"customer_code": "CUS_zz", "metadata": {"org_id": "%s"}},
			"plan": {"plan_code": "PLN_pro_ngn_m"},
			"paidAt": "2026-05-01T00:00:00.000Z"
		}
	}`, orgID))

	ev, err := p.ParseEvent(body)
	if err != nil {
		t.Fatalf("ParseEvent: %v", err)
	}
	if ev.Type != billing.EventInvoicePaid {
		t.Fatalf("Type = %s, want invoice.paid", ev.Type)
	}
	if ev.Subscription == nil {
		t.Fatal("expected subscription state from charge.success with plan")
	}
	if ev.Subscription.PlanSlug != "pro" {
		t.Errorf("plan slug = %q", ev.Subscription.PlanSlug)
	}
}

func TestParseEvent_ChargeSuccessWithoutPlan(t *testing.T) {
	// One-off charge (no plan) — we emit EventInvoicePaid but with no
	// subscription state. The handler ignores it for credit grants.
	p := providerForEvents(t)
	body := []byte(`{
		"event": "charge.success",
		"data": {"reference": "ref_x", "status": "success", "currency": "NGN"}
	}`)
	ev, err := p.ParseEvent(body)
	if err != nil {
		t.Fatalf("ParseEvent: %v", err)
	}
	if ev.Type != billing.EventInvoicePaid {
		t.Fatalf("Type = %s", ev.Type)
	}
	if ev.Subscription != nil {
		t.Fatal("expected nil subscription state for one-off charge")
	}
}

func TestParseEvent_UnknownEvent(t *testing.T) {
	p := providerForEvents(t)
	ev, err := p.ParseEvent([]byte(`{"event":"transfer.success","data":{}}`))
	if err != nil {
		t.Fatalf("unknown event should not error, got %v", err)
	}
	if ev.Type != billing.EventUnhandled {
		t.Fatalf("Type = %s, want unhandled", ev.Type)
	}
	if ev.RawProviderType != "transfer.success" {
		t.Errorf("raw type = %q", ev.RawProviderType)
	}
}

// TestParseEvent_MissingOrgIDReturnsNilOrg ensures the parser doesn't reject
// events whose customer.metadata is empty or null. Paystack's real samples
// often lack metadata (only populated for API-created customers), so the
// parser hands the problem to the handler via OrgID=uuid.Nil. The handler's
// resolveOrgID then falls back to subscription_code / customer_code lookup.
func TestParseEvent_MissingOrgIDReturnsNilOrg(t *testing.T) {
	p := providerForEvents(t)
	body := []byte(`{
		"event": "subscription.create",
		"data": {
			"subscription_code": "SUB_1",
			"status": "active",
			"customer": {"customer_code": "CUS_1", "metadata": {}},
			"plan": {"plan_code": "PLN_pro_ngn_m"}
		}
	}`)
	ev, err := p.ParseEvent(body)
	if err != nil {
		t.Fatalf("ParseEvent with missing org_id should not error: %v", err)
	}
	if ev.Subscription == nil {
		t.Fatal("expected subscription state")
	}
	if ev.Subscription.OrgID != uuid.Nil {
		t.Errorf("OrgID = %s, want Nil (handler must resolve)", ev.Subscription.OrgID)
	}
	// Correlation identifiers must still be populated so the handler can do
	// the fallback lookup.
	if ev.Subscription.ExternalSubscriptionID != "SUB_1" {
		t.Errorf("SubscriptionID missing: %q", ev.Subscription.ExternalSubscriptionID)
	}
	if ev.Subscription.ExternalCustomerID != "CUS_1" {
		t.Errorf("CustomerID missing: %q", ev.Subscription.ExternalCustomerID)
	}
}
