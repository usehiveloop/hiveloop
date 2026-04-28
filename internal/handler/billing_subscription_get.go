package handler

import (
	"net/http"

	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/billing"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

type subscriptionResponse struct {
	PlanSlug          string `json:"plan_slug"`
	Status            string `json:"status"`
	Provider          string `json:"provider,omitempty"`
	CreditsBalance    int64  `json:"credits_balance"`
	CurrentPeriodEnd  string `json:"current_period_end,omitempty"`
	CancelAtPeriodEnd bool   `json:"cancel_at_period_end"`

	// Payment-method snapshot from the most recent successful charge.
	PaymentChannel     string `json:"payment_channel,omitempty"`
	CardLast4          string `json:"card_last4,omitempty"`
	CardBrand          string `json:"card_brand,omitempty"`
	CardExpMonth       string `json:"card_exp_month,omitempty"`
	CardExpYear        string `json:"card_exp_year,omitempty"`
	PaymentBankName    string `json:"payment_bank_name,omitempty"`
	PaymentAccountName string `json:"payment_account_name,omitempty"`

	// Pending plan change scheduled at PendingChangeAt (downgrade flow).
	PendingPlanSlug string `json:"pending_plan_slug,omitempty"`
	PendingChangeAt string `json:"pending_change_at,omitempty"`
}

// GetSubscription returns the org's current subscription and credit balance.
// @Summary Get subscription status
// @Description Returns the org's active plan, provider, payment-method snapshot, and any pending plan change.
// @Tags billing
// @Produce json
// @Success 200 {object} subscriptionResponse
// @Failure 401 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/billing/subscription [get]
func (h *BillingHandler) GetSubscription(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	balance, err := h.credits.Balance(org.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load balance"})
		return
	}

	resp := subscriptionResponse{
		PlanSlug:       org.PlanSlug,
		Status:         "active",
		CreditsBalance: balance,
	}

	var sub model.Subscription
	err = h.db.Where("org_id = ? AND status = ?", org.ID, string(billing.StatusActive)).
		Order("created_at DESC").First(&sub).Error
	if err == nil {
		fillSubscriptionResponse(h.db, &sub, &resp)
	} else if err != gorm.ErrRecordNotFound {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load subscription"})
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func fillSubscriptionResponse(db *gorm.DB, sub *model.Subscription, resp *subscriptionResponse) {
	resp.Provider = sub.Provider
	resp.Status = sub.Status
	resp.CancelAtPeriodEnd = sub.CancelAtPeriodEnd
	if !sub.CurrentPeriodEnd.IsZero() {
		resp.CurrentPeriodEnd = sub.CurrentPeriodEnd.Format("2006-01-02T15:04:05Z07:00")
	}
	resp.PaymentChannel = sub.PaymentChannel
	resp.CardLast4 = sub.CardLast4
	resp.CardBrand = sub.CardBrand
	resp.CardExpMonth = sub.CardExpMonth
	resp.CardExpYear = sub.CardExpYear
	resp.PaymentBankName = sub.PaymentBankName
	resp.PaymentAccountName = sub.PaymentAccountName

	if sub.PendingPlanID != nil {
		var pending model.Plan
		if err := db.Where("id = ?", *sub.PendingPlanID).First(&pending).Error; err == nil {
			resp.PendingPlanSlug = pending.Slug
		}
		if sub.PendingChangeAt != nil {
			resp.PendingChangeAt = sub.PendingChangeAt.Format("2006-01-02T15:04:05Z07:00")
		}
	}
}
