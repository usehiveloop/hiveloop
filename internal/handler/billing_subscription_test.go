package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/billing"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// ---- preview-change ----

func TestSubscription_PreviewChange_Upgrade(t *testing.T) {
	h := newSubHarness(t)
	f := h.seedFixture(t, 500_000, 2_000_000)

	rr := h.post(t, "/v1/billing/subscription/preview-change", f.user.ID, f.org.ID, map[string]string{"plan_slug": f.planTo.Slug})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["kind"] != "upgrade" {
		t.Errorf("kind = %v, want upgrade", resp["kind"])
	}
	if resp["requires_payment_now"] != true {
		t.Errorf("requires_payment_now = %v, want true", resp["requires_payment_now"])
	}
	if resp["amount_minor"] == nil || resp["amount_minor"].(float64) <= 0 {
		t.Errorf("amount_minor = %v, want positive", resp["amount_minor"])
	}
}

func TestSubscription_PreviewChange_Downgrade(t *testing.T) {
	h := newSubHarness(t)
	f := h.seedFixture(t, 2_000_000, 500_000)

	rr := h.post(t, "/v1/billing/subscription/preview-change", f.user.ID, f.org.ID, map[string]string{"plan_slug": f.planTo.Slug})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["kind"] != "downgrade" {
		t.Errorf("kind = %v, want downgrade", resp["kind"])
	}
	if resp["requires_payment_now"] != false {
		t.Errorf("requires_payment_now = %v, want false", resp["requires_payment_now"])
	}
}

func TestSubscription_PreviewChange_NoSubscription(t *testing.T) {
	h := newSubHarness(t)
	user := model.User{Email: "no-sub@test.com", Name: "X"}
	h.db.Create(&user)
	org := model.Org{Name: "no-sub", Active: true}
	h.db.Create(&org)
	h.db.Create(&model.OrgMembership{UserID: user.ID, OrgID: org.ID, Role: "owner"})
	t.Cleanup(func() {
		h.db.Where("org_id = ?", org.ID).Delete(&model.OrgMembership{})
		h.db.Delete(&org)
		h.db.Delete(&user)
	})

	plan := model.Plan{ID: uuid.New(), Slug: "x-" + uuid.NewString()[:8], PriceCents: 100, Currency: "NGN", Active: true}
	h.db.Create(&plan)
	t.Cleanup(func() { h.db.Delete(&plan) })

	rr := h.post(t, "/v1/billing/subscription/preview-change", user.ID, org.ID, map[string]string{"plan_slug": plan.Slug})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

// ---- apply-change ----

func TestSubscription_ApplyChange_Upgrade(t *testing.T) {
	h := newSubHarness(t)
	f := h.seedFixture(t, 500_000, 2_000_000)

	rr := h.post(t, "/v1/billing/subscription/preview-change", f.user.ID, f.org.ID, map[string]string{"plan_slug": f.planTo.Slug})
	if rr.Code != http.StatusOK {
		t.Fatalf("preview: %d %s", rr.Code, rr.Body.String())
	}
	var preview map[string]any
	json.Unmarshal(rr.Body.Bytes(), &preview)
	quoteID := preview["quote_id"].(string)
	amount := int64(preview["amount_minor"].(float64))

	h.provider.NextResolveResult = &billing.ResolveCheckoutResult{
		Status:          billing.StatusActive,
		PaidAmountMinor: amount,
		Currency:        "NGN",
		Reference:       "ref_apply",
		PaymentMethod:   billing.PaymentMethod{Channel: billing.ChannelCard, AuthorizationCode: "AUTH_apply", CardLast4: "9999"},
	}

	rr = h.post(t, "/v1/billing/subscription/apply-change", f.user.ID, f.org.ID,
		map[string]string{"quote_id": quoteID, "paystack_reference": "ref_apply"})
	if rr.Code != http.StatusOK {
		t.Fatalf("apply: %d %s", rr.Code, rr.Body.String())
	}

	var sub model.Subscription
	if err := h.db.First(&sub, "id = ?", f.sub.ID).Error; err != nil {
		t.Fatalf("reload sub: %v", err)
	}
	if sub.PlanID != f.planTo.ID {
		t.Errorf("PlanID = %v, want planTo", sub.PlanID)
	}
	if sub.CardLast4 != "9999" {
		t.Errorf("CardLast4 = %q, want 9999", sub.CardLast4)
	}
}

func TestSubscription_ApplyChange_Downgrade(t *testing.T) {
	h := newSubHarness(t)
	f := h.seedFixture(t, 2_000_000, 500_000)

	rr := h.post(t, "/v1/billing/subscription/preview-change", f.user.ID, f.org.ID, map[string]string{"plan_slug": f.planTo.Slug})
	var preview map[string]any
	json.Unmarshal(rr.Body.Bytes(), &preview)
	quoteID := preview["quote_id"].(string)

	rr = h.post(t, "/v1/billing/subscription/apply-change", f.user.ID, f.org.ID, map[string]string{"quote_id": quoteID})
	if rr.Code != http.StatusOK {
		t.Fatalf("apply downgrade: %d %s", rr.Code, rr.Body.String())
	}
	var sub model.Subscription
	h.db.First(&sub, "id = ?", f.sub.ID)
	if sub.PendingPlanID == nil || *sub.PendingPlanID != f.planTo.ID {
		t.Errorf("PendingPlanID = %v, want planTo", sub.PendingPlanID)
	}
}

func TestSubscription_ApplyChange_AmountMismatch(t *testing.T) {
	h := newSubHarness(t)
	f := h.seedFixture(t, 500_000, 2_000_000)

	rr := h.post(t, "/v1/billing/subscription/preview-change", f.user.ID, f.org.ID, map[string]string{"plan_slug": f.planTo.Slug})
	var preview map[string]any
	json.Unmarshal(rr.Body.Bytes(), &preview)
	quoteID := preview["quote_id"].(string)
	amount := int64(preview["amount_minor"].(float64))

	h.provider.NextResolveResult = &billing.ResolveCheckoutResult{
		Status:          billing.StatusActive,
		PaidAmountMinor: amount - 1,
		Currency:        "NGN",
		Reference:       "ref_short",
		PaymentMethod:   billing.PaymentMethod{Channel: billing.ChannelCard, AuthorizationCode: "AUTH"},
	}

	rr = h.post(t, "/v1/billing/subscription/apply-change", f.user.ID, f.org.ID, map[string]string{"quote_id": quoteID, "paystack_reference": "ref_short"})
	if rr.Code != http.StatusPaymentRequired {
		t.Fatalf("expected 402, got %d %s", rr.Code, rr.Body.String())
	}
}

// ---- cancel & resume ----

func TestSubscription_Cancel_DefaultAtPeriodEnd(t *testing.T) {
	h := newSubHarness(t)
	f := h.seedFixture(t, 500_000, 2_000_000)

	rr := h.post(t, "/v1/billing/subscription/cancel", f.user.ID, f.org.ID, map[string]any{})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["cancel_at_period_end"] != true {
		t.Errorf("cancel_at_period_end = %v, want true", resp["cancel_at_period_end"])
	}
	if resp["status"] != "active" {
		t.Errorf("status = %v, want active (deferred)", resp["status"])
	}
}

func TestSubscription_Cancel_Immediate(t *testing.T) {
	h := newSubHarness(t)
	f := h.seedFixture(t, 500_000, 2_000_000)

	rr := h.post(t, "/v1/billing/subscription/cancel", f.user.ID, f.org.ID, map[string]any{"at_period_end": false})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["status"] != "canceled" {
		t.Errorf("status = %v, want canceled", resp["status"])
	}
}

func TestSubscription_Resume(t *testing.T) {
	h := newSubHarness(t)
	f := h.seedFixture(t, 500_000, 2_000_000)

	if rr := h.post(t, "/v1/billing/subscription/cancel", f.user.ID, f.org.ID, map[string]any{"at_period_end": true}); rr.Code != 200 {
		t.Fatalf("cancel: %d", rr.Code)
	}
	rr := h.post(t, "/v1/billing/subscription/resume", f.user.ID, f.org.ID, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("resume: %d %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["cancel_at_period_end"] != false {
		t.Errorf("cancel_at_period_end = %v, want false", resp["cancel_at_period_end"])
	}
}
