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

// loadSubscription returns the row by id; helper for assertions.
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

	customerCode := "CUS_snap_" + uuid.NewString()[:8]
	seedSubscription(t, db, "SUB_snap_"+uuid.NewString()[:8], customerCode)

	paidAt := time.Date(2026, 4, 28, 10, 0, 0, 0, time.UTC)
	err := h.SnapshotPaymentForTest("paystack", billing.Event{
		Type: billing.EventInvoicePaid,
		Subscription: &billing.SubscriptionState{
			ExternalCustomerID: customerCode,
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

	var got model.Subscription
	if err := db.Where("provider = ? AND external_customer_id = ?", "paystack", customerCode).First(&got).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}

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

	customerCode := "CUS_snap_noref_" + uuid.NewString()[:8]
	seedSubscription(t, db, "SUB_snap_noref_"+uuid.NewString()[:8], customerCode)

	if err := h.SnapshotPaymentForTest("paystack", billing.Event{
		Subscription: &billing.SubscriptionState{
			ExternalCustomerID: customerCode,
			// no ChargeReference, no card details
		},
	}); err != nil {
		t.Fatalf("snapshotPayment: %v", err)
	}

	var got model.Subscription
	if err := db.Where("provider = ? AND external_customer_id = ?", "paystack", customerCode).First(&got).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.LastChargeReference != "" || got.CardLast4 != "" {
		t.Errorf("expected fields untouched, got %+v", got)
	}
}

func TestSnapshotPayment_NoOpWhenNoMatchingSub(t *testing.T) {
	db := connectTestDB(t)
	h := handler.NewBillingWebhookHandler(db, billing.NewRegistry(), billing.NewCreditsService(db))

	// No subscription seeded for this customer code — handler must return
	// nil without erroring or affecting other rows.
	err := h.SnapshotPaymentForTest("paystack", billing.Event{
		Subscription: &billing.SubscriptionState{
			ExternalCustomerID: "CUS_does_not_exist",
			ChargeReference:    "re_orphan",
			CardLast4:          "1234",
		},
	})
	if err != nil {
		t.Fatalf("snapshotPayment should be a silent no-op, got: %v", err)
	}
}

func TestSnapshotPayment_OnlyTouchesActiveRow(t *testing.T) {
	db := connectTestDB(t)
	h := handler.NewBillingWebhookHandler(db, billing.NewRegistry(), billing.NewCreditsService(db))

	customerCode := "CUS_snap_canceled_" + uuid.NewString()[:8]
	seedSubscription(t, db, "SUB_snap_canceled_"+uuid.NewString()[:8], customerCode)
	// Mark it canceled so snapshotPayment's WHERE status='active' filters it out.
	if err := db.Model(&model.Subscription{}).
		Where("provider = ? AND external_customer_id = ?", "paystack", customerCode).
		Update("status", string(billing.StatusCanceled)).Error; err != nil {
		t.Fatalf("mark canceled: %v", err)
	}

	if err := h.SnapshotPaymentForTest("paystack", billing.Event{
		Subscription: &billing.SubscriptionState{
			ExternalCustomerID: customerCode,
			ChargeReference:    "re_should_skip",
			CardLast4:          "9999",
		},
	}); err != nil {
		t.Fatalf("snapshotPayment: %v", err)
	}

	var got model.Subscription
	if err := db.Where("provider = ? AND external_customer_id = ?", "paystack", customerCode).First(&got).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.LastChargeReference != "" || got.CardLast4 != "" {
		t.Errorf("canceled row was modified: %+v", got)
	}
}
