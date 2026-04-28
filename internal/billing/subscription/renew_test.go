package subscription_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/usehiveloop/hiveloop/internal/billing"
	subpkg "github.com/usehiveloop/hiveloop/internal/billing/subscription"
	"github.com/usehiveloop/hiveloop/internal/model"
)

func TestService_Renew_ChargesAndAdvancesPeriod(t *testing.T) {
	h := newHarness(t)
	h.seedOwnerEmail(t)
	sub := h.seedRenewSub(t, h.planPro)

	h.provider.NextChargeResult = &billing.ChargeAuthorizationResult{
		Status:          billing.StatusActive,
		Reference:       "ref_renew_1",
		PaidAmountMinor: h.planPro.PriceCents,
		Currency:        h.planPro.Currency,
		PaidAt:          &h.now,
		PaymentMethod: billing.PaymentMethod{
			AuthorizationCode: "AUTH_renew_new",
			Channel:           billing.ChannelCard,
			CardLast4:         "1111",
		},
	}

	action, err := h.service.Renew(context.Background(), sub.ID)
	if err != nil {
		t.Fatalf("Renew: %v", err)
	}
	if action != subpkg.ActionCharge {
		t.Fatalf("action = %q, want charge", action)
	}

	var fresh model.Subscription
	if err := h.db.First(&fresh, "id = ?", sub.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !fresh.CurrentPeriodEnd.After(h.now) {
		t.Errorf("CurrentPeriodEnd = %v, want after now %v", fresh.CurrentPeriodEnd, h.now)
	}
	if fresh.LastChargeReference != "ref_renew_1" {
		t.Errorf("LastChargeReference = %q", fresh.LastChargeReference)
	}
	if fresh.AuthorizationCode != "AUTH_renew_new" {
		t.Errorf("AuthorizationCode not refreshed: %q", fresh.AuthorizationCode)
	}
	if fresh.RenewalAttempts != 0 {
		t.Errorf("RenewalAttempts = %d, want 0 after success", fresh.RenewalAttempts)
	}

	// Plan-grant credit ledger entry must exist.
	bal, _ := h.credits.Balance(h.orgID)
	if bal != h.planPro.MonthlyCredits {
		t.Errorf("balance = %d, want %d", bal, h.planPro.MonthlyCredits)
	}

	// One charge call recorded against the provider.
	if got := h.provider.Charges(); len(got) != 1 || got[0].AmountMinor != h.planPro.PriceCents {
		t.Errorf("charges = %+v", got)
	}
}

func TestService_Renew_ChargeFailsIncrementsAttempts(t *testing.T) {
	h := newHarness(t)
	h.seedOwnerEmail(t)
	sub := h.seedRenewSub(t, h.planPro)

	h.provider.NextChargeError = errors.New("boom")

	action, err := h.service.Renew(context.Background(), sub.ID)
	if err == nil {
		t.Fatal("expected error to surface")
	}
	if action != subpkg.ActionCharge {
		t.Errorf("action = %q, want charge", action)
	}

	var fresh model.Subscription
	h.db.First(&fresh, "id = ?", sub.ID)
	if fresh.RenewalAttempts != 1 {
		t.Errorf("RenewalAttempts = %d, want 1", fresh.RenewalAttempts)
	}
	if fresh.Status != string(billing.StatusActive) {
		t.Errorf("Status = %q, want still active", fresh.Status)
	}
	if fresh.LastRenewalAttemptAt == nil {
		t.Error("LastRenewalAttemptAt should be set")
	}
	if fresh.LastRenewalError == "" {
		t.Error("LastRenewalError should be non-empty")
	}
}

func TestService_Renew_AfterMaxAttemptsMarksPastDue(t *testing.T) {
	h := newHarness(t)
	h.seedOwnerEmail(t)
	sub := h.seedRenewSub(t, h.planPro, func(s *model.Subscription) {
		s.RenewalAttempts = subpkg.MaxRenewalAttempts - 1 // 1 attempt away from cap
	})

	h.provider.NextChargeError = errors.New("declined")

	action, err := h.service.Renew(context.Background(), sub.ID)
	if err == nil {
		t.Fatal("expected error to surface")
	}
	if action != subpkg.ActionMarkPastDue {
		t.Errorf("action = %q, want mark_past_due", action)
	}

	var fresh model.Subscription
	h.db.First(&fresh, "id = ?", sub.ID)
	if fresh.Status != string(billing.StatusPastDue) {
		t.Errorf("Status = %q, want past_due", fresh.Status)
	}
	if fresh.RenewalAttempts != subpkg.MaxRenewalAttempts {
		t.Errorf("RenewalAttempts = %d, want %d", fresh.RenewalAttempts, subpkg.MaxRenewalAttempts)
	}
}

func TestService_Renew_NoOpWhenNotDue(t *testing.T) {
	h := newHarness(t)
	h.seedOwnerEmail(t)
	notDue := h.seedRenewSub(t, h.planPro, func(s *model.Subscription) {
		s.CurrentPeriodEnd = h.now.Add(24 * time.Hour) // future
	})

	action, err := h.service.Renew(context.Background(), notDue.ID)
	if err != nil {
		t.Fatalf("Renew: %v", err)
	}
	if action != subpkg.ActionNoOp {
		t.Errorf("action = %q, want noop", action)
	}
	if got := h.provider.Charges(); len(got) != 0 {
		t.Errorf("expected no charges, got %d", len(got))
	}
}

func TestService_Renew_CancelAtPeriodEndFinalizes(t *testing.T) {
	h := newHarness(t)
	h.seedOwnerEmail(t)
	sub := h.seedRenewSub(t, h.planPro, func(s *model.Subscription) {
		s.CancelAtPeriodEnd = true
	})

	action, err := h.service.Renew(context.Background(), sub.ID)
	if err != nil {
		t.Fatalf("Renew: %v", err)
	}
	if action != subpkg.ActionCancelAtPeriodEnd {
		t.Errorf("action = %q, want cancel_at_period_end", action)
	}

	var fresh model.Subscription
	h.db.First(&fresh, "id = ?", sub.ID)
	if fresh.Status != string(billing.StatusCanceled) {
		t.Errorf("Status = %q, want canceled", fresh.Status)
	}
	var org model.Org
	h.db.First(&org, "id = ?", h.orgID)
	if org.PlanSlug != billing.FreePlanSlug {
		t.Errorf("org.plan_slug = %q, want free", org.PlanSlug)
	}
	if got := h.provider.Charges(); len(got) != 0 {
		t.Errorf("expected no charges on cancel-at-end")
	}
}

func TestService_Renew_PendingDowngradeToFree(t *testing.T) {
	h := newHarness(t)
	h.seedOwnerEmail(t)
	freeID := h.planFree.ID
	sub := h.seedRenewSub(t, h.planPro, func(s *model.Subscription) {
		s.PendingPlanID = &freeID
		end := h.now.Add(-time.Minute)
		s.PendingChangeAt = &end
	})

	action, err := h.service.Renew(context.Background(), sub.ID)
	if err != nil {
		t.Fatalf("Renew: %v", err)
	}
	if action != subpkg.ActionTransitionToFree {
		t.Errorf("action = %q, want transition_to_free", action)
	}

	var fresh model.Subscription
	h.db.First(&fresh, "id = ?", sub.ID)
	if fresh.Status != string(billing.StatusCanceled) {
		t.Errorf("Status = %q, want canceled", fresh.Status)
	}
	if fresh.PlanID != freeID {
		t.Errorf("PlanID = %v, want planFree", fresh.PlanID)
	}
	if fresh.PendingPlanID != nil {
		t.Errorf("PendingPlanID should be cleared, got %v", *fresh.PendingPlanID)
	}
	if got := h.provider.Charges(); len(got) != 0 {
		t.Errorf("expected no charges on transition-to-free")
	}
}

func TestService_Renew_PendingDowngradeToPaidChargesNew(t *testing.T) {
	h := newHarness(t)
	h.seedOwnerEmail(t)
	starterID := h.planStart.ID
	sub := h.seedRenewSub(t, h.planPro, func(s *model.Subscription) {
		s.PendingPlanID = &starterID
		end := h.now.Add(-time.Minute)
		s.PendingChangeAt = &end
	})

	h.provider.NextChargeResult = &billing.ChargeAuthorizationResult{
		Status:          billing.StatusActive,
		Reference:       "ref_renew_starter",
		PaidAmountMinor: h.planStart.PriceCents,
		Currency:        h.planStart.Currency,
		PaidAt:          &h.now,
		PaymentMethod:   billing.PaymentMethod{AuthorizationCode: "AUTH", Channel: billing.ChannelCard},
	}

	action, err := h.service.Renew(context.Background(), sub.ID)
	if err != nil {
		t.Fatalf("Renew: %v", err)
	}
	if action != subpkg.ActionCharge {
		t.Errorf("action = %q, want charge", action)
	}

	var fresh model.Subscription
	h.db.First(&fresh, "id = ?", sub.ID)
	if fresh.PlanID != h.planStart.ID {
		t.Errorf("PlanID = %v, want planStart", fresh.PlanID)
	}
	if fresh.PendingPlanID != nil {
		t.Error("PendingPlanID should be cleared after applied at renewal")
	}
	// Charged the new (smaller) amount, not the old one.
	if got := h.provider.Charges(); len(got) != 1 || got[0].AmountMinor != h.planStart.PriceCents {
		t.Errorf("charge amount = %v, want %d", got, h.planStart.PriceCents)
	}
}

func TestService_Renew_NoAuthorizationMarksPastDue(t *testing.T) {
	h := newHarness(t)
	h.seedOwnerEmail(t)
	sub := h.seedRenewSub(t, h.planPro, func(s *model.Subscription) {
		s.AuthorizationCode = "" // wiped somehow
	})

	action, err := h.service.Renew(context.Background(), sub.ID)
	if err != nil {
		t.Fatalf("Renew: %v", err)
	}
	if action != subpkg.ActionMarkPastDue {
		t.Errorf("action = %q, want mark_past_due", action)
	}

	var fresh model.Subscription
	h.db.First(&fresh, "id = ?", sub.ID)
	if fresh.Status != string(billing.StatusPastDue) {
		t.Errorf("Status = %q, want past_due", fresh.Status)
	}
}

