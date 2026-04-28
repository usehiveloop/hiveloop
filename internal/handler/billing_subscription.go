package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/billing"
	"github.com/usehiveloop/hiveloop/internal/billing/subscription"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// SubscriptionHandler exposes self-managed subscription endpoints:
// preview/apply for plan changes, cancel, and resume.
type SubscriptionHandler struct {
	db      *gorm.DB
	service *subscription.Service
}

// NewSubscriptionHandler builds the handler against a Service.
func NewSubscriptionHandler(db *gorm.DB, registry *billing.Registry, credits *billing.CreditsService) *SubscriptionHandler {
	return &SubscriptionHandler{
		db:      db,
		service: subscription.NewService(db, registry, credits),
	}
}

// previewChangeRequest is the body for POST /v1/billing/subscription/preview-change.
type previewChangeRequest struct {
	PlanSlug string `json:"plan_slug"`
}

// previewChangeResponse describes the prorated effect of a switch.
type previewChangeResponse struct {
	QuoteID              string `json:"quote_id"`
	Kind                 string `json:"kind"`
	AmountMinor          int64  `json:"amount_minor"`
	Currency             string `json:"currency"`
	CreditGrantMinor     int64  `json:"credit_grant_minor"`
	EffectiveAt          string `json:"effective_at"`
	ExpiresAt            string `json:"expires_at"`
	FromPlanSlug         string `json:"from_plan_slug"`
	ToPlanSlug           string `json:"to_plan_slug"`
	RequiresPaymentNow   bool   `json:"requires_payment_now"`
}

// PreviewChange computes the proration for switching to plan_slug and stores
// a server-signed quote the customer can apply via /apply-change.
//
// @Summary Preview a subscription plan change
// @Tags billing
// @Accept json
// @Produce json
// @Param body body previewChangeRequest true "Target plan"
// @Success 200 {object} previewChangeResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Security BearerAuth
// @Router /v1/billing/subscription/preview-change [post]
func (h *SubscriptionHandler) PreviewChange(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}
	var body previewChangeRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.PlanSlug == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "plan_slug is required"})
		return
	}

	quote, preview, err := h.service.PreviewChange(r.Context(), org.ID, body.PlanSlug)
	if err != nil {
		switch {
		case errors.Is(err, subscription.ErrNoActiveSubscription):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no active subscription"})
		case errors.Is(err, subscription.ErrUnknownPlan):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown plan"})
		case errors.Is(err, subscription.ErrSamePlan):
			writeJSON(w, http.StatusConflict, map[string]string{"error": "already on this plan"})
		case errors.Is(err, subscription.ErrCurrencyMismatch):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot change between plans of different currencies"})
		case errors.Is(err, subscription.ErrFreeUpgrade):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "use /v1/billing/checkout for upgrades from the free plan"})
		case errors.Is(err, subscription.ErrPeriodEnded):
			writeJSON(w, http.StatusConflict, map[string]string{"error": "current period has ended; please wait for renewal"})
		default:
			slog.Error("preview-change: failed", "org_id", org.ID, "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to compute preview"})
		}
		return
	}

	from, to := h.lookupPlanSlugs(quote.FromPlanID, quote.ToPlanID)
	writeJSON(w, http.StatusOK, previewChangeResponse{
		QuoteID:            quote.ID.String(),
		Kind:               string(preview.Kind),
		AmountMinor:        preview.AmountMinor,
		Currency:           preview.Currency,
		CreditGrantMinor:   preview.CreditGrantMinor,
		EffectiveAt:        preview.EffectiveAt.Format("2006-01-02T15:04:05Z07:00"),
		ExpiresAt:          quote.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
		FromPlanSlug:       from,
		ToPlanSlug:         to,
		RequiresPaymentNow: preview.Kind == subscription.KindUpgrade && preview.AmountMinor > 0,
	})
}

// applyChangeRequest carries the quote id and (for upgrades) the verified
// Paystack reference the customer just paid against.
type applyChangeRequest struct {
	QuoteID           string `json:"quote_id"`
	PaystackReference string `json:"paystack_reference"`
}

// applyChangeResponse mirrors the post-apply state of the subscription.
type applyChangeResponse struct {
	Status   string `json:"status"`
	PlanSlug string `json:"plan_slug"`
}

// ApplyChange consumes a quote. Upgrades require a verified reference whose
// paid amount matches the quote; downgrades are deferred to period end.
//
// @Summary Apply a subscription plan change
// @Tags billing
// @Accept json
// @Produce json
// @Param body body applyChangeRequest true "Quote and (for upgrades) Paystack reference"
// @Success 200 {object} applyChangeResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 402 {object} errorResponse
// @Failure 410 {object} errorResponse
// @Security BearerAuth
// @Router /v1/billing/subscription/apply-change [post]
func (h *SubscriptionHandler) ApplyChange(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}
	var body applyChangeRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.QuoteID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "quote_id is required"})
		return
	}
	quoteID, err := uuid.Parse(body.QuoteID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "quote_id is not a valid uuid"})
		return
	}

	if err := h.service.ApplyChange(r.Context(), org.ID, quoteID, body.PaystackReference); err != nil {
		switch {
		case errors.Is(err, subscription.ErrQuoteUnknown):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown quote"})
		case errors.Is(err, subscription.ErrQuoteWrongOrg):
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "quote belongs to another org"})
		case errors.Is(err, subscription.ErrQuoteExpired):
			writeJSON(w, http.StatusGone, map[string]string{"error": "quote has expired"})
		case errors.Is(err, subscription.ErrAmountMismatch),
			errors.Is(err, subscription.ErrCurrencyMismatchOnVerify),
			errors.Is(err, subscription.ErrChargeRejected),
			errors.Is(err, subscription.ErrUnsupportedChannel):
			writeJSON(w, http.StatusPaymentRequired, map[string]string{"error": err.Error()})
		default:
			slog.Error("apply-change: failed", "org_id", org.ID, "quote_id", quoteID, "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to apply change"})
		}
		return
	}

	// Reload the org's active subscription to mirror its post-apply state.
	var sub model.Subscription
	_ = h.db.Where("org_id = ? AND status = ?", org.ID, string(billing.StatusActive)).
		Order("created_at DESC").First(&sub).Error
	planSlug := ""
	if sub.PlanID != uuid.Nil {
		var p model.Plan
		_ = h.db.Where("id = ?", sub.PlanID).First(&p).Error
		planSlug = p.Slug
	}
	writeJSON(w, http.StatusOK, applyChangeResponse{Status: sub.Status, PlanSlug: planSlug})
}

// cancelRequest. AtPeriodEnd defaults to true (Stripe-style soft cancel).
type cancelRequest struct {
	AtPeriodEnd *bool `json:"at_period_end,omitempty"`
}

type cancelResponse struct {
	Status            string `json:"status"`
	CancelAtPeriodEnd bool   `json:"cancel_at_period_end"`
	CanceledAt        string `json:"canceled_at,omitempty"`
}

// Cancel marks a subscription canceled. Default is at period end.
//
// @Summary Cancel a subscription
// @Tags billing
// @Accept json
// @Produce json
// @Param body body cancelRequest false "Cancellation options"
// @Success 200 {object} cancelResponse
// @Security BearerAuth
// @Router /v1/billing/subscription/cancel [post]
func (h *SubscriptionHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}
	var body cancelRequest
	_ = json.NewDecoder(r.Body).Decode(&body)
	atPeriodEnd := true
	if body.AtPeriodEnd != nil {
		atPeriodEnd = *body.AtPeriodEnd
	}

	sub, err := h.service.Cancel(r.Context(), org.ID, subscription.CancelInput{AtPeriodEnd: atPeriodEnd})
	if err != nil {
		switch {
		case errors.Is(err, subscription.ErrNoActiveSubscription):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no active subscription"})
		case errors.Is(err, subscription.ErrCannotCancel):
			writeJSON(w, http.StatusConflict, map[string]string{"error": "already canceled"})
		default:
			slog.Error("cancel: failed", "org_id", org.ID, "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to cancel"})
		}
		return
	}
	resp := cancelResponse{Status: sub.Status, CancelAtPeriodEnd: sub.CancelAtPeriodEnd}
	if sub.CanceledAt != nil {
		resp.CanceledAt = sub.CanceledAt.Format("2006-01-02T15:04:05Z07:00")
	}
	writeJSON(w, http.StatusOK, resp)
}

// Resume clears a pending cancel-at-period-end flag.
//
// @Summary Resume a subscription
// @Tags billing
// @Produce json
// @Success 200 {object} cancelResponse
// @Security BearerAuth
// @Router /v1/billing/subscription/resume [post]
func (h *SubscriptionHandler) Resume(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}
	sub, err := h.service.Resume(r.Context(), org.ID)
	if err != nil {
		switch {
		case errors.Is(err, subscription.ErrNoActiveSubscription):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no active subscription"})
		case errors.Is(err, subscription.ErrCannotResume):
			writeJSON(w, http.StatusConflict, map[string]string{"error": "subscription is not active"})
		default:
			slog.Error("resume: failed", "org_id", org.ID, "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to resume"})
		}
		return
	}
	writeJSON(w, http.StatusOK, cancelResponse{Status: sub.Status, CancelAtPeriodEnd: sub.CancelAtPeriodEnd})
}

func (h *SubscriptionHandler) lookupPlanSlugs(fromID, toID uuid.UUID) (from, to string) {
	var f, t model.Plan
	if err := h.db.Where("id = ?", fromID).First(&f).Error; err == nil {
		from = f.Slug
	}
	if err := h.db.Where("id = ?", toID).First(&t).Error; err == nil {
		to = t.Slug
	}
	return from, to
}
