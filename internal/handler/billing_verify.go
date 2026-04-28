package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

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

// Verify resolves an in-flight checkout by reference and upserts the
// Subscription row.
//
// @Summary Verify checkout completed
// @Description Synchronously resolves a checkout reference, asserts the transaction succeeded, and upserts the local Subscription row.
// @Tags billing
// @Accept json
// @Produce json
// @Param body body verifyRequest true "Reference returned from /v1/billing/checkout"
// @Success 200 {object} verifyResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
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

	if err := h.upsertSubscriptionFromResolve(body.Provider, org.ID, res); err != nil {
		slog.Error("billing verify: upsert failed", "provider", body.Provider, "org_id", org.ID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to record subscription"})
		return
	}

	writeJSON(w, http.StatusOK, verifyResponse{Status: "active", PlanSlug: res.PlanSlug})
}

func (h *BillingHandler) upsertSubscriptionFromResolve(providerName string, orgID uuid.UUID, res *billing.ResolveCheckoutResult) error {
	var plan model.Plan
	if err := h.db.Where("slug = ?", res.PlanSlug).First(&plan).Error; err != nil {
		return err
	}

	var existing model.Subscription
	err := h.db.Where("provider = ? AND org_id = ? AND plan_id = ? AND status = ?",
		providerName, orgID, plan.ID, string(billing.StatusActive)).
		First(&existing).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	if errors.Is(err, gorm.ErrRecordNotFound) {
		sub := model.Subscription{
			OrgID:                  orgID,
			PlanID:                 plan.ID,
			Provider:               providerName,
			ExternalSubscriptionID: res.ExternalSubscriptionID,
			ExternalCustomerID:     res.ExternalCustomerID,
			Status:                 string(billing.StatusActive),
			CurrentPeriodStart:     res.CurrentPeriodStart,
			CurrentPeriodEnd:       res.CurrentPeriodEnd,
			LastChargeReference:    res.ChargeReference,
			LastChargeAmount:       res.ChargeAmount,
			CardLast4:              res.CardLast4,
			CardBrand:              res.CardBrand,
			CardExpMonth:           res.CardExpMonth,
			CardExpYear:            res.CardExpYear,
			AuthorizationCode:      res.AuthorizationCode,
		}
		if res.ChargedAt != nil {
			sub.LastChargedAt = res.ChargedAt
		}
		if err := h.db.Create(&sub).Error; err != nil {
			return err
		}
	} else {
		updates := map[string]any{
			"external_subscription_id": res.ExternalSubscriptionID,
			"external_customer_id":     res.ExternalCustomerID,
			"current_period_start":     res.CurrentPeriodStart,
			"current_period_end":       res.CurrentPeriodEnd,
			"last_charge_reference":    res.ChargeReference,
			"last_charge_amount":       res.ChargeAmount,
			"card_last4":               res.CardLast4,
			"card_brand":               res.CardBrand,
			"card_exp_month":           res.CardExpMonth,
			"card_exp_year":            res.CardExpYear,
			"authorization_code":       res.AuthorizationCode,
		}
		if res.ChargedAt != nil {
			updates["last_charged_at"] = res.ChargedAt
		}
		if err := h.db.Model(&existing).Updates(updates).Error; err != nil {
			return err
		}
	}

	return h.db.Model(&model.Org{}).Where("id = ?", orgID).Update("plan_slug", plan.Slug).Error
}
