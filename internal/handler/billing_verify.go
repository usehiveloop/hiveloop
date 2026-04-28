package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/billing"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

type verifyRequest struct {
	Provider  string `json:"provider"`
	Reference string `json:"reference"`
}

type verifyResponse struct {
	Status   string `json:"status"`
	PlanSlug string `json:"plan_slug,omitempty"`
}

// MonthlyPeriod is the duration of one billing cycle. We don't yet support
// other cadences; annual support would key off plan.Cycle.
const MonthlyPeriod = 30 * 24 * time.Hour

// Verify resolves a Paystack reference and applies the result. It is the
// final hop of the fresh-subscribe flow:
//
//  1. PaystackPop popup completes on the client
//  2. Client POSTs /v1/billing/verify with the reference
//  3. Server calls /transaction/verify, asserts the paid amount and
//     currency match the plan we sent the customer to (defense against a
//     tampered popup), then inserts/updates the Subscription row, snapshots
//     the saved authorization, and grants the plan's monthly credits.
//
// @Summary Verify checkout completed
// @Description Resolves a Paystack transaction reference, asserts the paid amount matches the plan's price, and provisions the Subscription row.
// @Tags billing
// @Accept json
// @Produce json
// @Param body body verifyRequest true "Reference returned from /v1/billing/checkout"
// @Success 200 {object} verifyResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 402 {object} errorResponse
// @Failure 502 {object} errorResponse
// @Security BearerAuth
// @Router /v1/billing/verify [post]
func (h *BillingHandler) Verify(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	var body verifyRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if body.Reference == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "reference is required"})
		return
	}
	if body.Provider == "" {
		body.Provider = "paystack"
	}

	provider, err := h.registry.Get(body.Provider)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown provider"})
		return
	}

	res, err := provider.ResolveCheckout(r.Context(), billing.ResolveCheckoutRequest{
		Reference:     body.Reference,
		ExpectedOrgID: org.ID,
	})
	if err != nil {
		slog.Error("billing verify: resolve failed", "provider", body.Provider, "reference", body.Reference, "error", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "could not resolve checkout"})
		return
	}
	if res.Status != billing.StatusActive {
		writeJSON(w, http.StatusOK, verifyResponse{Status: string(res.Status)})
		return
	}

	planSlug := res.Metadata["plan_slug"]
	if planSlug == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "transaction has no plan_slug metadata"})
		return
	}

	var plan model.Plan
	if err := h.db.Where("slug = ? AND active = true", planSlug).First(&plan).Error; err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown plan"})
		return
	}

	if res.PaidAmountMinor != plan.PriceCents {
		slog.Warn("billing verify: amount mismatch",
			"reference", body.Reference,
			"paid", res.PaidAmountMinor,
			"expected", plan.PriceCents,
			"plan", plan.Slug)
		writeJSON(w, http.StatusPaymentRequired, map[string]string{"error": "paid amount does not match plan price"})
		return
	}
	if res.Currency != plan.Currency {
		writeJSON(w, http.StatusPaymentRequired, map[string]string{"error": "paid currency does not match plan"})
		return
	}
	if !res.PaymentMethod.Channel.IsReusable() {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "payment channel is not eligible for subscriptions"})
		return
	}

	if err := h.upsertFreshSubscription(body.Provider, org.ID, plan, res); err != nil {
		slog.Error("billing verify: upsert failed", "provider", body.Provider, "org_id", org.ID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to record subscription"})
		return
	}

	writeJSON(w, http.StatusOK, verifyResponse{Status: "active", PlanSlug: plan.Slug})
}

func (h *BillingHandler) upsertFreshSubscription(providerName string, orgID uuid.UUID, plan model.Plan, res *billing.ResolveCheckoutResult) error {
	now := time.Now()
	periodStart := now
	if res.PaidAt != nil {
		periodStart = *res.PaidAt
	}
	periodEnd := periodStart.Add(MonthlyPeriod)

	return h.db.Transaction(func(tx *gorm.DB) error {
		var existing model.Subscription
		err := tx.Where("provider = ? AND org_id = ? AND plan_id = ? AND status = ?",
			providerName, orgID, plan.ID, string(billing.StatusActive)).
			First(&existing).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		isNew := errors.Is(err, gorm.ErrRecordNotFound)
		var sub model.Subscription
		if isNew {
			sub = model.Subscription{
				OrgID:              orgID,
				PlanID:             plan.ID,
				Provider:           providerName,
				Status:             string(billing.StatusActive),
				CurrentPeriodStart: periodStart,
				CurrentPeriodEnd:   periodEnd,
			}
		} else {
			sub = existing
			sub.CurrentPeriodStart = periodStart
			sub.CurrentPeriodEnd = periodEnd
		}

		applyPaymentMethodToRow(&sub, res)

		if isNew {
			if err := tx.Create(&sub).Error; err != nil {
				return err
			}
		} else {
			if err := tx.Save(&sub).Error; err != nil {
				return err
			}
		}

		if err := tx.Model(&model.Org{}).Where("id = ?", orgID).
			Update("plan_slug", plan.Slug).Error; err != nil {
			return err
		}

		if plan.MonthlyCredits > 0 {
			expires := periodEnd.Add(billing.PlanGrantGracePeriod)
			if err := billing.GrantWithTx(tx, orgID, plan.MonthlyCredits,
				billing.ReasonPlanGrant, "subscription", res.Reference, &expires); err != nil &&
				!errors.Is(err, billing.ErrAlreadyRecorded) {
				return err
			}
		}
		return nil
	})
}

// applyPaymentMethodToRow lifts the payment-method snapshot from a verify
// result onto a Subscription row. Mirrors the helper inside the
// subscription package — kept here so this package doesn't depend on it.
func applyPaymentMethodToRow(sub *model.Subscription, res *billing.ResolveCheckoutResult) {
	pm := res.PaymentMethod
	sub.AuthorizationCode = pm.AuthorizationCode
	sub.PaymentChannel = string(pm.Channel)
	sub.CardLast4 = pm.CardLast4
	sub.CardBrand = pm.CardBrand
	sub.CardExpMonth = pm.CardExpMonth
	sub.CardExpYear = pm.CardExpYear
	sub.PaymentBankName = pm.BankName
	sub.PaymentAccountName = pm.AccountName
	sub.LastChargeReference = res.Reference
	sub.LastChargeAmount = res.PaidAmountMinor
	sub.LastChargedAt = res.PaidAt
	if res.ExternalCustomerID != "" {
		sub.ExternalCustomerID = res.ExternalCustomerID
	}
}
