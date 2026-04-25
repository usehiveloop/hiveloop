package fake_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/billing"
	"github.com/usehiveloop/hiveloop/internal/billing/fake"
)

// Confirm the fake satisfies the Provider interface at compile time.
var _ billing.Provider = (*fake.Provider)(nil)

func TestFake_EnsureCustomerIsIdempotent(t *testing.T) {
	p := fake.New("fake")
	orgID := uuid.New()

	id1, err := p.EnsureCustomer(context.Background(), orgID, "a@example.com", "Acme")
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	id2, err := p.EnsureCustomer(context.Background(), orgID, "a@example.com", "Acme")
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if id1 != id2 {
		t.Fatalf("expected idempotent customer id, got %q then %q", id1, id2)
	}
}

func TestFake_CreateCheckoutRecordsIntent(t *testing.T) {
	p := fake.New("fake")
	p.NextCheckoutURL = "https://custom.example/checkout"

	session, err := p.CreateCheckout(context.Background(), "cus_1", billing.CheckoutIntent{
		OrgID:    uuid.New(),
		PlanSlug: "pro",
	})
	if err != nil {
		t.Fatalf("CreateCheckout: %v", err)
	}
	if session.URL != "https://custom.example/checkout" {
		t.Fatalf("expected custom URL, got %q", session.URL)
	}

	got := p.Checkouts()
	if len(got) != 1 {
		t.Fatalf("expected 1 recorded intent, got %d", len(got))
	}
	if got[0].PlanSlug != "pro" {
		t.Fatalf("recorded PlanSlug = %q, want %q", got[0].PlanSlug, "pro")
	}
}

func TestFake_VerifyWebhookSignature(t *testing.T) {
	p := fake.New("fake")
	p.SignatureHeader = "X-Sig"
	p.SignatureValue = "secret"

	// missing header — rejected
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	if err := p.VerifyWebhook(r, nil); err == nil {
		t.Fatal("expected rejection when signature missing")
	}

	// wrong value — rejected
	r = httptest.NewRequest(http.MethodPost, "/", nil)
	r.Header.Set("X-Sig", "wrong")
	if err := p.VerifyWebhook(r, nil); err == nil {
		t.Fatal("expected rejection when signature wrong")
	}

	// right value — accepted
	r = httptest.NewRequest(http.MethodPost, "/", nil)
	r.Header.Set("X-Sig", "secret")
	if err := p.VerifyWebhook(r, nil); err != nil {
		t.Fatalf("expected acceptance, got %v", err)
	}
}

func TestFake_ParseEvent(t *testing.T) {
	p := fake.New("fake")
	orgID := uuid.New()
	body, _ := json.Marshal(fake.Event{
		Type: billing.EventSubscriptionActivated,
		Subscription: &billing.SubscriptionState{
			ExternalSubscriptionID: "sub_1",
			ExternalCustomerID:     "cus_1",
			OrgID:                  orgID,
			PlanSlug:               "pro",
			Status:                 billing.StatusActive,
		},
	})

	ev, err := p.ParseEvent(body)
	if err != nil {
		t.Fatalf("ParseEvent: %v", err)
	}
	if ev.Type != billing.EventSubscriptionActivated {
		t.Fatalf("Type = %q, want %q", ev.Type, billing.EventSubscriptionActivated)
	}
	if ev.Subscription == nil || ev.Subscription.OrgID != orgID {
		t.Fatalf("subscription org id mismatch")
	}
}
