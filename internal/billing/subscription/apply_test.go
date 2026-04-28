package subscription_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/billing"
	subpkg "github.com/usehiveloop/hiveloop/internal/billing/subscription"
	"github.com/usehiveloop/hiveloop/internal/model"
)

func TestService_ApplyChange_UpgradeAppliesAndGrantsCredits(t *testing.T) {
	h := newHarness(t)
	h.seedSub(t, h.planStart, 15*24*3600)

	quote, _, err := h.service.PreviewChange(context.Background(), h.orgID, h.planPro.Slug)
	if err != nil {
		t.Fatalf("PreviewChange: %v", err)
	}

	h.provider.NextResolveResult = &billing.ResolveCheckoutResult{
		Status:             billing.StatusActive,
		PaidAmountMinor:    quote.AmountMinor,
		Currency:           quote.Currency,
		Reference:          "ref_apply_pro",
		ExternalCustomerID: "CUS_test",
		PaidAt:             &h.now,
		PaymentMethod: billing.PaymentMethod{
			AuthorizationCode: "AUTH_new",
			Channel:           billing.ChannelCard,
			CardLast4:         "4242",
			CardBrand:         "visa",
		},
	}

	if err := h.service.ApplyChange(context.Background(), h.orgID, quote.ID, "ref_apply_pro"); err != nil {
		t.Fatalf("ApplyChange: %v", err)
	}

	var sub model.Subscription
	if err := h.db.Where("org_id = ?", h.orgID).First(&sub).Error; err != nil {
		t.Fatalf("reload sub: %v", err)
	}
	if sub.PlanID != h.planPro.ID {
		t.Errorf("PlanID = %v, want planPro.ID %v", sub.PlanID, h.planPro.ID)
	}
	if sub.AuthorizationCode != "AUTH_new" {
		t.Errorf("AuthorizationCode = %q, want AUTH_new", sub.AuthorizationCode)
	}
	if sub.LastChargeReference != "ref_apply_pro" {
		t.Errorf("LastChargeReference = %q, want ref_apply_pro", sub.LastChargeReference)
	}

	balance, err := h.credits.Balance(h.orgID)
	if err != nil {
		t.Fatalf("balance: %v", err)
	}
	// (5_000 - 1_000) * 1/2 = 2_000.
	if balance != 2_000 {
		t.Errorf("balance = %d, want 2000 (proration credit)", balance)
	}

	var fresh model.SubscriptionChangeQuote
	if err := h.db.First(&fresh, "id = ?", quote.ID).Error; err != nil {
		t.Fatalf("reload quote: %v", err)
	}
	if fresh.ConsumedAt == nil {
		t.Error("expected quote to be consumed")
	}
}

func TestService_ApplyChange_AmountMismatchRejected(t *testing.T) {
	h := newHarness(t)
	h.seedSub(t, h.planStart, 15*24*3600)
	quote, _, _ := h.service.PreviewChange(context.Background(), h.orgID, h.planPro.Slug)

	h.provider.NextResolveResult = &billing.ResolveCheckoutResult{
		Status:          billing.StatusActive,
		PaidAmountMinor: quote.AmountMinor - 1,
		Currency:        quote.Currency,
		Reference:       "ref_short",
		PaymentMethod:   billing.PaymentMethod{Channel: billing.ChannelCard, AuthorizationCode: "AUTH"},
	}

	err := h.service.ApplyChange(context.Background(), h.orgID, quote.ID, "ref_short")
	if !errors.Is(err, subpkg.ErrAmountMismatch) {
		t.Fatalf("expected ErrAmountMismatch, got %v", err)
	}

	var sub model.Subscription
	h.db.First(&sub, "org_id = ?", h.orgID)
	if sub.PlanID != h.planStart.ID {
		t.Errorf("PlanID changed; expected starter")
	}
	var fresh model.SubscriptionChangeQuote
	h.db.First(&fresh, "id = ?", quote.ID)
	if fresh.ConsumedAt != nil {
		t.Error("quote should not be consumed on mismatch")
	}
}

func TestService_ApplyChange_UnsupportedChannelRejected(t *testing.T) {
	h := newHarness(t)
	h.seedSub(t, h.planStart, 15*24*3600)
	quote, _, _ := h.service.PreviewChange(context.Background(), h.orgID, h.planPro.Slug)

	h.provider.NextResolveResult = &billing.ResolveCheckoutResult{
		Status:          billing.StatusActive,
		PaidAmountMinor: quote.AmountMinor,
		Currency:        quote.Currency,
		Reference:       "ref_ussd",
		PaymentMethod:   billing.PaymentMethod{Channel: billing.PaymentChannel("ussd"), AuthorizationCode: "AUTH"},
	}

	err := h.service.ApplyChange(context.Background(), h.orgID, quote.ID, "ref_ussd")
	if !errors.Is(err, subpkg.ErrUnsupportedChannel) {
		t.Fatalf("expected ErrUnsupportedChannel, got %v", err)
	}
}

func TestService_ApplyChange_QuoteWrongOrg(t *testing.T) {
	h := newHarness(t)
	h.seedSub(t, h.planStart, 15*24*3600)
	quote, _, _ := h.service.PreviewChange(context.Background(), h.orgID, h.planPro.Slug)

	otherOrg := uuid.New()
	err := h.service.ApplyChange(context.Background(), otherOrg, quote.ID, "ref_x")
	if !errors.Is(err, subpkg.ErrQuoteWrongOrg) {
		t.Fatalf("expected ErrQuoteWrongOrg, got %v", err)
	}
}

func TestService_ApplyChange_IdempotentReplay(t *testing.T) {
	h := newHarness(t)
	h.seedSub(t, h.planStart, 15*24*3600)
	quote, _, _ := h.service.PreviewChange(context.Background(), h.orgID, h.planPro.Slug)

	h.provider.NextResolveResult = &billing.ResolveCheckoutResult{
		Status:          billing.StatusActive,
		PaidAmountMinor: quote.AmountMinor,
		Currency:        quote.Currency,
		Reference:       "ref_idempotent",
		PaymentMethod:   billing.PaymentMethod{Channel: billing.ChannelCard, AuthorizationCode: "AUTH"},
	}

	if err := h.service.ApplyChange(context.Background(), h.orgID, quote.ID, "ref_idempotent"); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	if err := h.service.ApplyChange(context.Background(), h.orgID, quote.ID, "ref_idempotent"); err != nil {
		t.Fatalf("second apply: %v", err)
	}

	balance, _ := h.credits.Balance(h.orgID)
	if balance != 2_000 {
		t.Errorf("balance = %d after replay, want still 2000", balance)
	}
}

func TestService_ApplyChange_ExpiredQuote(t *testing.T) {
	h := newHarness(t)
	h.seedSub(t, h.planStart, 15*24*3600)
	h.service.SetQuoteTTL(0)

	quote, _, _ := h.service.PreviewChange(context.Background(), h.orgID, h.planPro.Slug)
	h.service.SetClock(func() time.Time { return h.now.Add(time.Second) })

	err := h.service.ApplyChange(context.Background(), h.orgID, quote.ID, "ref_late")
	if !errors.Is(err, subpkg.ErrQuoteExpired) {
		t.Fatalf("expected ErrQuoteExpired, got %v", err)
	}
}

func TestService_ApplyChange_DowngradeMarksPending(t *testing.T) {
	h := newHarness(t)
	sub := h.seedSub(t, h.planPro, 5*24*3600)

	quote, _, _ := h.service.PreviewChange(context.Background(), h.orgID, h.planStart.Slug)

	if err := h.service.ApplyChange(context.Background(), h.orgID, quote.ID, ""); err != nil {
		t.Fatalf("ApplyChange downgrade: %v", err)
	}

	var fresh model.Subscription
	if err := h.db.First(&fresh, "id = ?", sub.ID).Error; err != nil {
		t.Fatalf("reload sub: %v", err)
	}
	if fresh.PendingPlanID == nil || *fresh.PendingPlanID != h.planStart.ID {
		t.Errorf("PendingPlanID = %v, want planStart.ID", fresh.PendingPlanID)
	}
	if fresh.PendingChangeAt == nil {
		t.Error("PendingChangeAt should be set")
	}
}
