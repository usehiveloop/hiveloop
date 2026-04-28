package handler_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/billing"
	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// seededSub bundles everything a webhook test typically needs about a
// pre-seeded Subscription row: the org id, the plan, and the row itself.
type seededSub struct {
	OrgID uuid.UUID
	Plan  model.Plan
	Sub   model.Subscription
}

// seedSubscriptionFull inserts a Subscription row (and its prerequisite org +
// plan). Used by snapshot/upsert tests that need org_id + plan_slug to match
// the (provider, org_id, plan_id, status='active') key.
func seedSubscriptionFull(t *testing.T, db *gorm.DB, externalSubID, externalCustomerID string) seededSub {
	t.Helper()
	org := createTestOrg(t, db)

	plan := model.Plan{
		ID:             uuid.New(),
		Slug:           "pro-" + uuid.NewString()[:8],
		Name:           "Pro test",
		MonthlyCredits: 39_000,
	}
	if err := db.Create(&plan).Error; err != nil {
		t.Fatalf("create plan: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", plan.ID).Delete(&model.Plan{}) })

	sub := model.Subscription{
		OrgID:                  org.ID,
		PlanID:                 plan.ID,
		Provider:               "paystack",
		ExternalSubscriptionID: externalSubID,
		ExternalCustomerID:     externalCustomerID,
		Status:                 string(billing.StatusActive),
	}
	if err := db.Create(&sub).Error; err != nil {
		t.Fatalf("create subscription: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", sub.ID).Delete(&model.Subscription{}) })
	return seededSub{OrgID: org.ID, Plan: plan, Sub: sub}
}

// seedSubscription is the legacy helper kept for resolveOrgID tests that
// only need the org id back.
func seedSubscription(t *testing.T, db *gorm.DB, externalSubID, externalCustomerID string) uuid.UUID {
	return seedSubscriptionFull(t, db, externalSubID, externalCustomerID).OrgID
}

// TestBillingWebhook_ResolveOrgID_FromState exercises the happy path:
// Paystack echoed our customer metadata so state.OrgID is already set;
// no DB lookup happens.
func TestBillingWebhook_ResolveOrgID_FromState(t *testing.T) {
	db := connectTestDB(t)
	h := handler.NewBillingWebhookHandler(db, billing.NewRegistry(), billing.NewCreditsService(db))

	orgID := uuid.New()
	got, err := h.ResolveOrgIDForTest("paystack", &billing.SubscriptionState{OrgID: orgID})
	if err != nil {
		t.Fatalf("resolveOrgID: %v", err)
	}
	if got != orgID {
		t.Errorf("got %s, want %s", got, orgID)
	}
}

// TestBillingWebhook_ResolveOrgID_FallbackBySubscriptionCode simulates the
// Paystack payload where customer.metadata was dropped (common in real
// samples). The handler must find the existing subscription row by
// (provider, external_subscription_id) and return its org.
func TestBillingWebhook_ResolveOrgID_FallbackBySubscriptionCode(t *testing.T) {
	db := connectTestDB(t)
	h := handler.NewBillingWebhookHandler(db, billing.NewRegistry(), billing.NewCreditsService(db))

	wantOrg := seedSubscription(t, db, "SUB_fallback_1", "CUS_fallback_1")

	got, err := h.ResolveOrgIDForTest("paystack", &billing.SubscriptionState{
		ExternalSubscriptionID: "SUB_fallback_1",
	})
	if err != nil {
		t.Fatalf("resolveOrgID: %v", err)
	}
	if got != wantOrg {
		t.Errorf("got %s, want %s", got, wantOrg)
	}
}

// TestBillingWebhook_ResolveOrgID_FallbackByCustomerCode exercises the second
// fallback — only customer_code is available, no subscription_code (e.g.
// a charge.success for a new subscription attempt).
func TestBillingWebhook_ResolveOrgID_FallbackByCustomerCode(t *testing.T) {
	db := connectTestDB(t)
	h := handler.NewBillingWebhookHandler(db, billing.NewRegistry(), billing.NewCreditsService(db))

	wantOrg := seedSubscription(t, db, "SUB_cust_fallback_1", "CUS_cust_fallback_1")

	got, err := h.ResolveOrgIDForTest("paystack", &billing.SubscriptionState{
		ExternalCustomerID: "CUS_cust_fallback_1",
	})
	if err != nil {
		t.Fatalf("resolveOrgID: %v", err)
	}
	if got != wantOrg {
		t.Errorf("got %s, want %s", got, wantOrg)
	}
}

// TestBillingWebhook_ResolveOrgID_NoCorrelation ensures the handler fails
// loudly rather than writing a row with uuid.Nil when nothing matches.
func TestBillingWebhook_ResolveOrgID_NoCorrelation(t *testing.T) {
	db := connectTestDB(t)
	h := handler.NewBillingWebhookHandler(db, billing.NewRegistry(), billing.NewCreditsService(db))

	_, err := h.ResolveOrgIDForTest("paystack", &billing.SubscriptionState{})
	if err == nil {
		t.Fatal("expected error when state has no correlation identifier")
	}
	_, err = h.ResolveOrgIDForTest("paystack", &billing.SubscriptionState{
		ExternalSubscriptionID: "SUB_does_not_exist",
	})
	if err == nil {
		t.Fatal("expected error when subscription not found")
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Errorf("error should wrap gorm.ErrRecordNotFound: %v", err)
	}
}
