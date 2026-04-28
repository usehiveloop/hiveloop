package subscription_test

import (
	"context"
	"errors"
	"testing"

	subpkg "github.com/usehiveloop/hiveloop/internal/billing/subscription"
)

func TestService_PreviewChange_UpgradeIssuesQuote(t *testing.T) {
	h := newHarness(t)
	h.seedSub(t, h.planStart, 15*24*3600) // halfway through period

	quote, preview, err := h.service.PreviewChange(context.Background(), h.orgID, h.planPro.Slug)
	if err != nil {
		t.Fatalf("PreviewChange: %v", err)
	}
	if quote.Kind != string(subpkg.KindUpgrade) {
		t.Errorf("Kind = %q, want upgrade", quote.Kind)
	}
	// Halfway through, (2_000_000 - 500_000) / 2 = 750_000.
	if quote.AmountMinor != 750_000 {
		t.Errorf("AmountMinor = %d, want 750000", quote.AmountMinor)
	}
	if preview.AmountMinor != 750_000 {
		t.Errorf("preview.AmountMinor = %d, want 750000", preview.AmountMinor)
	}
	if quote.Currency != "NGN" {
		t.Errorf("Currency = %q, want NGN", quote.Currency)
	}
	if !quote.ExpiresAt.After(h.now) {
		t.Errorf("ExpiresAt %v should be after now %v", quote.ExpiresAt, h.now)
	}
}

func TestService_PreviewChange_DowngradeIssuesQuote(t *testing.T) {
	h := newHarness(t)
	h.seedSub(t, h.planPro, 5*24*3600)

	quote, preview, err := h.service.PreviewChange(context.Background(), h.orgID, h.planStart.Slug)
	if err != nil {
		t.Fatalf("PreviewChange: %v", err)
	}
	if quote.Kind != string(subpkg.KindDowngrade) {
		t.Errorf("Kind = %q, want downgrade", quote.Kind)
	}
	if quote.AmountMinor != 0 {
		t.Errorf("AmountMinor = %d, want 0 for downgrade", quote.AmountMinor)
	}
	if preview.EffectiveAt.Before(h.now) {
		t.Errorf("downgrade EffectiveAt %v should be in the future", preview.EffectiveAt)
	}
}

func TestService_PreviewChange_NoSubscription(t *testing.T) {
	h := newHarness(t)
	_, _, err := h.service.PreviewChange(context.Background(), h.orgID, h.planPro.Slug)
	if !errors.Is(err, subpkg.ErrNoActiveSubscription) {
		t.Fatalf("expected ErrNoActiveSubscription, got %v", err)
	}
}

func TestService_PreviewChange_UnknownPlan(t *testing.T) {
	h := newHarness(t)
	h.seedSub(t, h.planStart, 0)
	_, _, err := h.service.PreviewChange(context.Background(), h.orgID, "no-such-plan")
	if !errors.Is(err, subpkg.ErrUnknownPlan) {
		t.Fatalf("expected ErrUnknownPlan, got %v", err)
	}
}

func TestService_PreviewChange_SamePlanRejected(t *testing.T) {
	h := newHarness(t)
	h.seedSub(t, h.planStart, 0)
	_, _, err := h.service.PreviewChange(context.Background(), h.orgID, h.planStart.Slug)
	if !errors.Is(err, subpkg.ErrSamePlan) {
		t.Fatalf("expected ErrSamePlan, got %v", err)
	}
}
