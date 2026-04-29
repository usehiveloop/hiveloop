package fake_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/billing"
	"github.com/usehiveloop/hiveloop/internal/billing/fake"
)

// Confirm the fake satisfies the Provider interface at compile time.
var _ billing.Provider = (*fake.Provider)(nil)

// TestFake_EnsureCustomerIsIdempotent verifies that EnsureCustomer returns the same ID for repeated calls.
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

// Note: Tests for CreateCheckoutRecordsIntent, ChargeAuthorizationDefaultActive, ChargeAuthorizationError
// were removed as they test fake implementation behavior. See USELESS_TESTS_RECOMMENDATIONS.md for details.