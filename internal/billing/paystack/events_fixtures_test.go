package paystack

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/billing"
)

// loadFixture reads a Paystack webhook sample from testdata, substituting a
// real uuid for any ORG_ID_PLACEHOLDER inside so tests can verify the
// extracted org id flows end-to-end.
func loadFixture(t *testing.T, name string, orgID uuid.UUID) []byte {
	t.Helper()
	path := filepath.Join("testdata", name)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	body = bytes.ReplaceAll(body, []byte("ORG_ID_PLACEHOLDER"), []byte(orgID.String()))
	return body
}

// fixturesProvider wires plan_code → slug for the codes used in the
// real-sample fixtures so planIndex lookups resolve.
func fixturesProvider(t *testing.T) *Provider {
	t.Helper()
	return New(Config{
		SecretKey: "sk_test",
		Plans: PlanRegistry{
			"pro": {"NGN": {Monthly: "PLN_pro_ngn_m", Annual: "PLN_pro_ngn_a"}},
		},
	})
}

// TestParseEvent_Fixture_SubscriptionCreate verifies the real Paystack
// subscription.create payload (with org_id injected into customer.metadata
// the way EnsureCustomer populates it) round-trips into a valid state.
func TestParseEvent_Fixture_SubscriptionCreate(t *testing.T) {
	orgID := uuid.New()
	body := loadFixture(t, "subscription_create.json", orgID)

	ev, err := fixturesProvider(t).ParseEvent(body)
	if err != nil {
		t.Fatalf("ParseEvent: %v", err)
	}
	if ev.Type != billing.EventSubscriptionActivated {
		t.Fatalf("Type = %s, want subscription.activated", ev.Type)
	}
	if ev.Subscription.OrgID != orgID {
		t.Errorf("OrgID = %s, want %s", ev.Subscription.OrgID, orgID)
	}
	if ev.Subscription.PlanSlug != "pro" {
		t.Errorf("PlanSlug = %q, want pro", ev.Subscription.PlanSlug)
	}
	if ev.Subscription.Status != billing.StatusActive {
		t.Errorf("Status = %s, want active", ev.Subscription.Status)
	}
	if ev.Subscription.ExternalSubscriptionID != "SUB_vsyqdmlzble3uii" {
		t.Errorf("SubscriptionID = %q", ev.Subscription.ExternalSubscriptionID)
	}
}

// TestParseEvent_Fixture_SubscriptionDisableComplete verifies status="complete"
// (Paystack's "billing cycles exhausted" signal) maps to StatusRevoked, not
// the default StatusActive that the old code returned.
func TestParseEvent_Fixture_SubscriptionDisableComplete(t *testing.T) {
	ev, err := fixturesProvider(t).ParseEvent(loadFixture(t, "subscription_disable_complete.json", uuid.Nil))
	if err != nil {
		t.Fatalf("ParseEvent: %v", err)
	}
	if ev.Type != billing.EventSubscriptionCanceled {
		t.Fatalf("Type = %s", ev.Type)
	}
	if ev.Subscription.Status != billing.StatusRevoked {
		t.Errorf("Status = %s, want revoked (complete means billing exhausted)", ev.Subscription.Status)
	}
	// Real sample has customer.metadata = {} → OrgID falls back to Nil,
	// handler's resolveOrgID will look it up by subscription_code.
	if ev.Subscription.OrgID != uuid.Nil {
		t.Errorf("OrgID = %s, want Nil when metadata is empty", ev.Subscription.OrgID)
	}
}

// TestParseEvent_Fixture_SubscriptionNotRenew verifies the real sample with
// customer.metadata=null parses without error and emits a canceled state.
func TestParseEvent_Fixture_SubscriptionNotRenew(t *testing.T) {
	ev, err := fixturesProvider(t).ParseEvent(loadFixture(t, "subscription_not_renew.json", uuid.Nil))
	if err != nil {
		t.Fatalf("ParseEvent: %v", err)
	}
	if ev.Type != billing.EventSubscriptionRevoked {
		t.Fatalf("Type = %s", ev.Type)
	}
	if ev.Subscription.Status != billing.StatusCanceled {
		t.Errorf("Status = %s, want canceled (non-renewing)", ev.Subscription.Status)
	}
	if ev.Subscription.ExternalCustomerID != "CUS_8gbmdpvn12c67ix" {
		t.Errorf("CustomerID = %q", ev.Subscription.ExternalCustomerID)
	}
}

// TestParseEvent_Fixture_InvoicePaymentFailed verifies invoice.payment_failed
// parses despite (a) no plan object anywhere in the payload, and (b) customer
// sitting at data root rather than inside the subscription sub-object.
func TestParseEvent_Fixture_InvoicePaymentFailed(t *testing.T) {
	ev, err := fixturesProvider(t).ParseEvent(loadFixture(t, "invoice_payment_failed.json", uuid.Nil))
	if err != nil {
		t.Fatalf("ParseEvent: %v", err)
	}
	if ev.Type != billing.EventPaymentFailed {
		t.Fatalf("Type = %s", ev.Type)
	}
	if ev.Subscription == nil {
		t.Fatal("expected subscription state (for handler to update status)")
	}
	if ev.Subscription.ExternalSubscriptionID != "SUB_f7ct8g01mtcjf78" {
		t.Errorf("SubscriptionID = %q", ev.Subscription.ExternalSubscriptionID)
	}
	if ev.Subscription.ExternalCustomerID != "CUS_3p3ylxyf07605kx" {
		t.Errorf("CustomerID = %q (should come from outer customer, not nested)", ev.Subscription.ExternalCustomerID)
	}
	if ev.Subscription.PlanSlug != "" {
		t.Errorf("PlanSlug = %q, want empty (no plan in invoice payload)", ev.Subscription.PlanSlug)
	}
}

// TestParseEvent_Fixture_ChargeSuccessOneOff verifies "plan": {} (empty
// object) is correctly detected as a non-subscription charge. The old code
// treated {} as non-nil and returned bogus state with empty plan_code.
func TestParseEvent_Fixture_ChargeSuccessOneOff(t *testing.T) {
	ev, err := fixturesProvider(t).ParseEvent(loadFixture(t, "charge_success_oneoff.json", uuid.Nil))
	if err != nil {
		t.Fatalf("ParseEvent: %v", err)
	}
	if ev.Type != billing.EventInvoicePaid {
		t.Fatalf("Type = %s", ev.Type)
	}
	if ev.Subscription != nil {
		t.Fatal("expected nil subscription state for one-off charge with empty plan")
	}
}

// TestParseEvent_Fixture_ChargeSuccessPlanAsString exercises the older
// charge.success shape where `data.plan` is the plan_code as a JSON string
// (not an object). Required because Paystack's docs show both shapes and
// a single deployed system can receive either.
func TestParseEvent_Fixture_ChargeSuccessPlanAsString(t *testing.T) {
	orgID := uuid.New()
	ev, err := fixturesProvider(t).ParseEvent(loadFixture(t, "charge_success_subscription_string.json", orgID))
	if err != nil {
		t.Fatalf("ParseEvent: %v", err)
	}
	if ev.Type != billing.EventInvoicePaid {
		t.Fatalf("Type = %s", ev.Type)
	}
	if ev.Subscription == nil {
		t.Fatal("expected subscription state when plan arrives as a plan_code string")
	}
	if ev.Subscription.PlanSlug != "pro" {
		t.Errorf("PlanSlug = %q, want pro (resolved from PLN_pro_ngn_m string)", ev.Subscription.PlanSlug)
	}
	if ev.Subscription.OrgID != orgID {
		t.Errorf("OrgID = %s, want %s", ev.Subscription.OrgID, orgID)
	}
}

// TestParseEvent_Fixture_InvoiceCreateUnhandled verifies invoice.create is
// explicitly ignored (charge.success already covers "credits granted").
func TestParseEvent_Fixture_InvoiceCreateUnhandled(t *testing.T) {
	ev, err := fixturesProvider(t).ParseEvent(loadFixture(t, "invoice_create.json", uuid.Nil))
	if err != nil {
		t.Fatalf("ParseEvent: %v", err)
	}
	if ev.Type != billing.EventUnhandled {
		t.Errorf("Type = %s, want unhandled", ev.Type)
	}
	if ev.RawProviderType != "invoice.create" {
		t.Errorf("RawProviderType = %q", ev.RawProviderType)
	}
}

// TestExtractOrgID_MissingReturnsNil confirms the parser tolerates missing
// customer metadata (per the real samples) and hands the resolution job to
// the handler instead of erroring out.
func TestExtractOrgID_MissingReturnsNil(t *testing.T) {
	id, err := extractOrgID(nil)
	if err != nil {
		t.Errorf("nil meta should not error, got %v", err)
	}
	if id != uuid.Nil {
		t.Errorf("id = %s, want Nil", id)
	}
	id, err = extractOrgID(map[string]string{})
	if err != nil || id != uuid.Nil {
		t.Errorf("empty meta: id=%s err=%v", id, err)
	}
	id, err = extractOrgID(map[string]string{"org_id": ""})
	if err != nil || id != uuid.Nil {
		t.Errorf("blank org_id: id=%s err=%v", id, err)
	}
}

// TestExtractOrgID_InvalidIsHardError makes sure a malformed uuid isn't
// silently dropped — corrupt metadata is a bug we want surfaced.
func TestExtractOrgID_InvalidIsHardError(t *testing.T) {
	_, err := extractOrgID(map[string]string{"org_id": "not-a-uuid"})
	if err == nil {
		t.Fatal("expected error for non-uuid org_id")
	}
	if !strings.Contains(err.Error(), "not-a-uuid") {
		t.Errorf("error should reference the bad value: %v", err)
	}
}

// TestParseChargePlanCode_AllShapes exercises every observed wire shape for
// data.plan on charge.success in one place, so future Paystack shape
// changes have a single source of truth to update.
func TestParseChargePlanCode_AllShapes(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"missing", "", ""},
		{"null", "null", ""},
		{"empty object", "{}", ""},
		{"whitespace", "   ", ""},
		{"string", `"PLN_abc"`, "PLN_abc"},
		{"object", `{"plan_code":"PLN_xyz","interval":"monthly"}`, "PLN_xyz"},
		{"object empty plan_code", `{"plan_code":"","interval":"monthly"}`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseChargePlanCode([]byte(tc.raw))
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestMapSubscriptionStatus_Completed locks the fix for the real-world bug:
// Paystack sends "complete" (no trailing -d), not "completed".
func TestMapSubscriptionStatus_Completed(t *testing.T) {
	if got := mapSubscriptionStatus("complete"); got != billing.StatusRevoked {
		t.Errorf("complete → %s, want revoked", got)
	}
	if got := mapSubscriptionStatus("completed"); got != billing.StatusRevoked {
		t.Errorf("completed → %s, want revoked", got)
	}
	if got := mapSubscriptionStatus("non-renewing"); got != billing.StatusCanceled {
		t.Errorf("non-renewing → %s, want canceled", got)
	}
}
