package fake_test

import (
	"context"
	"errors"
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
	if session.Reference == "" {
		t.Fatal("expected a non-empty reference")
	}

	got := p.Checkouts()
	if len(got) != 1 {
		t.Fatalf("expected 1 recorded intent, got %d", len(got))
	}
	if got[0].PlanSlug != "pro" {
		t.Fatalf("recorded PlanSlug = %q, want %q", got[0].PlanSlug, "pro")
	}
}

func TestFake_ChargeAuthorizationDefaultActive(t *testing.T) {
	p := fake.New("fake")
	res, err := p.ChargeAuthorization(context.Background(), billing.ChargeAuthorizationRequest{
		Email:             "x@example.com",
		AuthorizationCode: "AUTH_xxx",
		AmountMinor:       1000,
		Currency:          "NGN",
	})
	if err != nil {
		t.Fatalf("ChargeAuthorization: %v", err)
	}
	if res.Status != billing.StatusActive {
		t.Errorf("Status = %q, want active", res.Status)
	}
	if res.PaidAmountMinor != 1000 {
		t.Errorf("PaidAmountMinor = %d, want 1000", res.PaidAmountMinor)
	}

	if got := p.Charges(); len(got) != 1 {
		t.Fatalf("expected 1 recorded charge, got %d", len(got))
	}
}

func TestFake_ChargeAuthorizationError(t *testing.T) {
	p := fake.New("fake")
	p.NextChargeError = errors.New("boom")

	if _, err := p.ChargeAuthorization(context.Background(), billing.ChargeAuthorizationRequest{
		AuthorizationCode: "AUTH_xxx",
		AmountMinor:       100,
	}); err == nil {
		t.Fatal("expected error")
	}
}
