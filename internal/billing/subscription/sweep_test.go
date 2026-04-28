package subscription_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/billing"
	subpkg "github.com/usehiveloop/hiveloop/internal/billing/subscription"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// seedRenewSub creates an active subscription whose current_period_end has
// just passed so it's due for renewal. Lives in this file (rather than
// renew_test.go) so the per-test file stays under the 300-line cap.
func (h *harness) seedRenewSub(t *testing.T, plan model.Plan, opts ...func(*model.Subscription)) model.Subscription {
	t.Helper()
	end := h.now.Add(-time.Minute)
	start := end.Add(-30 * 24 * time.Hour)
	sub := model.Subscription{
		OrgID:              h.orgID,
		PlanID:             plan.ID,
		Provider:           "paystack",
		ExternalCustomerID: "CUS_renew",
		Status:             string(billing.StatusActive),
		CurrentPeriodStart: start,
		CurrentPeriodEnd:   end,
		AuthorizationCode:  "AUTH_renew",
		PaymentChannel:     "card",
		CardLast4:          "0000",
	}
	for _, opt := range opts {
		opt(&sub)
	}
	if err := h.db.Create(&sub).Error; err != nil {
		t.Fatalf("seed sub: %v", err)
	}
	return sub
}

// seedOwnerEmail attaches an owner user+membership so the renewal worker
// can resolve sub.OrgID → email when calling ChargeAuthorization.
func (h *harness) seedOwnerEmail(t *testing.T) {
	t.Helper()
	user := model.User{ID: uuid.New(), Email: "owner-" + uuid.NewString()[:8] + "@test.com", Name: "Owner"}
	if err := h.db.Create(&user).Error; err != nil {
		t.Fatalf("create owner: %v", err)
	}
	if err := h.db.Create(&model.OrgMembership{UserID: user.ID, OrgID: h.orgID, Role: "owner"}).Error; err != nil {
		t.Fatalf("create membership: %v", err)
	}
	t.Cleanup(func() {
		h.db.Where("org_id = ?", h.orgID).Delete(&model.OrgMembership{})
		h.db.Where("id = ?", user.ID).Delete(&model.User{})
	})
}

func TestService_Renew_IdempotentReplay(t *testing.T) {
	h := newHarness(t)
	h.seedOwnerEmail(t)
	sub := h.seedRenewSub(t, h.planPro)

	h.provider.NextChargeResult = &billing.ChargeAuthorizationResult{
		Status:          billing.StatusActive,
		Reference:       "ref_idempotent",
		PaidAmountMinor: h.planPro.PriceCents,
		Currency:        h.planPro.Currency,
		PaidAt:          &h.now,
		PaymentMethod:   billing.PaymentMethod{AuthorizationCode: "AUTH", Channel: billing.ChannelCard},
	}

	if _, err := h.service.Renew(context.Background(), sub.ID); err != nil {
		t.Fatalf("first renew: %v", err)
	}
	// Re-running should be a noop: the period is already in the future
	// and the credit ledger's idempotency index blocks a duplicate grant.
	action2, err := h.service.Renew(context.Background(), sub.ID)
	if err != nil {
		t.Fatalf("second renew: %v", err)
	}
	if action2 != subpkg.ActionNoOp {
		t.Errorf("second action = %q, want noop", action2)
	}

	bal, _ := h.credits.Balance(h.orgID)
	if bal != h.planPro.MonthlyCredits {
		t.Errorf("balance = %d, want %d (no double grant)", bal, h.planPro.MonthlyCredits)
	}
}

func TestSweep_DueSubscriptionIDs_FiltersCorrectly(t *testing.T) {
	h := newHarness(t)
	h.seedOwnerEmail(t)

	due := h.seedRenewSub(t, h.planPro)
	notDue := h.seedRenewSub(t, h.planStart, func(s *model.Subscription) {
		s.CurrentPeriodEnd = h.now.Add(24 * time.Hour)
	})
	atCap := h.seedRenewSub(t, h.planPrem, func(s *model.Subscription) {
		s.RenewalAttempts = subpkg.MaxRenewalAttempts
	})
	rateLimited := h.seedRenewSub(t, h.planFree, func(s *model.Subscription) {
		recent := h.now.Add(-time.Minute)
		s.LastRenewalAttemptAt = &recent
	})

	ids, err := subpkg.DueSubscriptionIDs(context.Background(), h.db, h.now, 100)
	if err != nil {
		t.Fatalf("DueSubscriptionIDs: %v", err)
	}

	contains := func(target uuid.UUID) bool {
		for _, id := range ids {
			if id == target {
				return true
			}
		}
		return false
	}

	if !contains(due.ID) {
		t.Errorf("due sub %v should be selected", due.ID)
	}
	for _, sub := range []model.Subscription{notDue, atCap, rateLimited} {
		if contains(sub.ID) {
			t.Errorf("sub %v should NOT be selected", sub.ID)
		}
	}
}
