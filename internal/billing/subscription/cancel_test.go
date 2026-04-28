package subscription_test

import (
	"context"
	"errors"
	"testing"

	"github.com/usehiveloop/hiveloop/internal/billing"
	subpkg "github.com/usehiveloop/hiveloop/internal/billing/subscription"
	"github.com/usehiveloop/hiveloop/internal/model"
)

func TestService_Cancel_AtPeriodEnd(t *testing.T) {
	h := newHarness(t)
	h.seedSub(t, h.planPro, 0)

	sub, err := h.service.Cancel(context.Background(), h.orgID, subpkg.CancelInput{AtPeriodEnd: true})
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if !sub.CancelAtPeriodEnd {
		t.Error("expected CancelAtPeriodEnd=true")
	}
	if sub.Status != string(billing.StatusActive) {
		t.Errorf("Status = %q, want active (cancellation deferred)", sub.Status)
	}
}

func TestService_Cancel_Immediate(t *testing.T) {
	h := newHarness(t)
	h.seedSub(t, h.planPro, 0)

	sub, err := h.service.Cancel(context.Background(), h.orgID, subpkg.CancelInput{AtPeriodEnd: false})
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if sub.Status != string(billing.StatusCanceled) {
		t.Errorf("Status = %q, want canceled", sub.Status)
	}
	if sub.CanceledAt == nil {
		t.Error("CanceledAt should be set")
	}

	var org model.Org
	h.db.First(&org, "id = ?", h.orgID)
	if org.PlanSlug != billing.FreePlanSlug {
		t.Errorf("org.plan_slug = %q, want %q", org.PlanSlug, billing.FreePlanSlug)
	}
}

func TestService_Resume_FromAtPeriodEnd(t *testing.T) {
	h := newHarness(t)
	h.seedSub(t, h.planPro, 0)
	if _, err := h.service.Cancel(context.Background(), h.orgID, subpkg.CancelInput{AtPeriodEnd: true}); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	sub, err := h.service.Resume(context.Background(), h.orgID)
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if sub.CancelAtPeriodEnd {
		t.Error("expected CancelAtPeriodEnd=false after resume")
	}
}

func TestService_Resume_AfterImmediateCancelRejected(t *testing.T) {
	h := newHarness(t)
	h.seedSub(t, h.planPro, 0)
	if _, err := h.service.Cancel(context.Background(), h.orgID, subpkg.CancelInput{AtPeriodEnd: false}); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	_, err := h.service.Resume(context.Background(), h.orgID)
	if !errors.Is(err, subpkg.ErrNoActiveSubscription) && !errors.Is(err, subpkg.ErrCannotResume) {
		t.Fatalf("expected ErrNoActiveSubscription or ErrCannotResume, got %v", err)
	}
}
