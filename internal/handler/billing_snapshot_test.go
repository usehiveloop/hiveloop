package handler_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/billing"
	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/model"
)

func loadSubscription(t *testing.T, db *gorm.DB, id uuid.UUID) model.Subscription {
	t.Helper()
	var sub model.Subscription
	if err := db.Where("id = ?", id).First(&sub).Error; err != nil {
		t.Fatalf("load subscription: %v", err)
	}
	return sub
}

func TestSnapshotPayment_PopulatesFields(t *testing.T) {
	db := connectTestDB(t)
	h := handler.NewBillingWebhookHandler(db, billing.NewRegistry(), billing.NewCreditsService(db))

	seeded := seedSubscriptionFull(t,
		db,
		"SUB_snap_"+uuid.NewString()[:8],
		"CUS_snap_"+uuid.NewString()[:8],
	)

	paidAt := time.Date(2026, 4, 28, 10, 0, 0, 0, time.UTC)
	err := h.SnapshotPaymentForTest("paystack", billing.Event{
		Type: billing.EventInvoicePaid,
		Subscription: &billing.SubscriptionState{
			OrgID:              seeded.OrgID,
			PlanSlug:           seeded.Plan.Slug,
			ExternalCustomerID: seeded.Sub.ExternalCustomerID,
			ChargeReference:    "re_snap_1",
			ChargeAmount:       58_500_00,
			ChargedAt:          &paidAt,
			CardLast4:          "4081",
			CardBrand:          "visa",
			CardExpMonth:       "12",
			CardExpYear:        "2030",
			AuthorizationCode:  "AUTH_snap_1",
		},
	})
	if err != nil {
		t.Fatalf("snapshotPayment: %v", err)
	}

	got := loadSubscription(t, db, seeded.Sub.ID)

	if got.LastChargeReference != "re_snap_1" {
		t.Errorf("LastChargeReference = %q, want %q", got.LastChargeReference, "re_snap_1")
	}
	if got.LastChargeAmount != 5_850_000 {
		t.Errorf("LastChargeAmount = %d, want 5_850_000", got.LastChargeAmount)
	}
	if got.LastChargedAt == nil || !got.LastChargedAt.Equal(paidAt) {
		t.Errorf("LastChargedAt = %v, want %v", got.LastChargedAt, paidAt)
	}
	if got.CardLast4 != "4081" || got.CardBrand != "visa" {
		t.Errorf("card snapshot wrong: last4=%q brand=%q", got.CardLast4, got.CardBrand)
	}
	if got.CardExpMonth != "12" || got.CardExpYear != "2030" {
		t.Errorf("card exp wrong: %q/%q", got.CardExpMonth, got.CardExpYear)
	}
	if got.AuthorizationCode != "AUTH_snap_1" {
		t.Errorf("AuthorizationCode = %q, want AUTH_snap_1", got.AuthorizationCode)
	}
}

func TestSnapshotPayment_NoOpWhenNoChargeReference(t *testing.T) {
	db := connectTestDB(t)
	h := handler.NewBillingWebhookHandler(db, billing.NewRegistry(), billing.NewCreditsService(db))

	seeded := seedSubscriptionFull(t,
		db,
		"SUB_snap_noref_"+uuid.NewString()[:8],
		"CUS_snap_noref_"+uuid.NewString()[:8],
	)

	if err := h.SnapshotPaymentForTest("paystack", billing.Event{
		Subscription: &billing.SubscriptionState{
			OrgID:    seeded.OrgID,
			PlanSlug: seeded.Plan.Slug,
			// no ChargeReference, no card details
		},
	}); err != nil {
		t.Fatalf("snapshotPayment: %v", err)
	}

	got := loadSubscription(t, db, seeded.Sub.ID)
	if got.LastChargeReference != "" || got.CardLast4 != "" {
		t.Errorf("expected fields untouched, got %+v", got)
	}
}

func TestSnapshotPayment_CreatesRowWhenNoneExists(t *testing.T) {
	db := connectTestDB(t)
	h := handler.NewBillingWebhookHandler(db, billing.NewRegistry(), billing.NewCreditsService(db))

	// No prior subscription row — only an org and a plan exist.
	org := createTestOrg(t, db)
	plan := model.Plan{
		ID:             uuid.New(),
		Slug:           "pro-create-" + uuid.NewString()[:8],
		Name:           "Pro create test",
		MonthlyCredits: 39_000,
	}
	if err := db.Create(&plan).Error; err != nil {
		t.Fatalf("create plan: %v", err)
	}
	t.Cleanup(func() {
		db.Where("org_id = ?", org.ID).Delete(&model.Subscription{})
		db.Where("id = ?", plan.ID).Delete(&model.Plan{})
	})

	customerCode := "CUS_create_" + uuid.NewString()[:8]
	paidAt := time.Date(2026, 4, 28, 10, 0, 0, 0, time.UTC)
	if err := h.SnapshotPaymentForTest("paystack", billing.Event{
		Subscription: &billing.SubscriptionState{
			OrgID:              org.ID,
			PlanSlug:           plan.Slug,
			ExternalCustomerID: customerCode,
			ChargeReference:    "re_create_1",
			ChargeAmount:       58_500_00,
			ChargedAt:          &paidAt,
			CardLast4:          "4081",
			CardBrand:          "visa",
			CardExpMonth:       "12",
			CardExpYear:        "2030",
			AuthorizationCode:  "AUTH_create_1",
		},
	}); err != nil {
		t.Fatalf("snapshotPayment: %v", err)
	}

	var got model.Subscription
	if err := db.Where("provider = ? AND org_id = ? AND plan_id = ?", "paystack", org.ID, plan.ID).
		First(&got).Error; err != nil {
		t.Fatalf("expected new subscription row, got: %v", err)
	}
	if got.LastChargeReference != "re_create_1" || got.CardLast4 != "4081" {
		t.Errorf("payment snapshot not populated on insert: %+v", got)
	}
	if got.ExternalCustomerID != customerCode {
		t.Errorf("ExternalCustomerID = %q, want %q", got.ExternalCustomerID, customerCode)
	}
	if got.Status != string(billing.StatusActive) {
		t.Errorf("Status = %q, want active", got.Status)
	}

	// Org's denormalised plan_slug should reflect the new active plan.
	var orgRow model.Org
	if err := db.Where("id = ?", org.ID).First(&orgRow).Error; err != nil {
		t.Fatalf("reload org: %v", err)
	}
	if orgRow.PlanSlug != plan.Slug {
		t.Errorf("org.plan_slug = %q, want %q", orgRow.PlanSlug, plan.Slug)
	}
}

func TestSnapshotPayment_OnlyTouchesActiveRow(t *testing.T) {
	db := connectTestDB(t)
	h := handler.NewBillingWebhookHandler(db, billing.NewRegistry(), billing.NewCreditsService(db))

	seeded := seedSubscriptionFull(t,
		db,
		"SUB_snap_canceled_"+uuid.NewString()[:8],
		"CUS_snap_canceled_"+uuid.NewString()[:8],
	)
	if err := db.Model(&seeded.Sub).Update("status", string(billing.StatusCanceled)).Error; err != nil {
		t.Fatalf("mark canceled: %v", err)
	}

	if err := h.SnapshotPaymentForTest("paystack", billing.Event{
		Subscription: &billing.SubscriptionState{
			OrgID:              seeded.OrgID,
			PlanSlug:           seeded.Plan.Slug,
			ExternalCustomerID: seeded.Sub.ExternalCustomerID,
			ChargeReference:    "re_create_after_cancel",
			CardLast4:          "9999",
		},
	}); err != nil {
		t.Fatalf("snapshotPayment: %v", err)
	}

	// The canceled row should remain untouched.
	got := loadSubscription(t, db, seeded.Sub.ID)
	if got.LastChargeReference != "" || got.CardLast4 != "" {
		t.Errorf("canceled row was modified: %+v", got)
	}

	// And a *new* active row should have been created (charge.success now
	// always materialises a row when none is active).
	var fresh model.Subscription
	if err := db.Where("provider = ? AND org_id = ? AND plan_id = ? AND status = ?",
		"paystack", seeded.OrgID, seeded.Plan.ID, string(billing.StatusActive)).
		First(&fresh).Error; err != nil {
		t.Fatalf("expected new active row, got: %v", err)
	}
	if fresh.LastChargeReference != "re_create_after_cancel" {
		t.Errorf("new row charge ref wrong: %q", fresh.LastChargeReference)
	}
}
