package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/billing"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

type BillingHandler struct {
	db       *gorm.DB
	registry *billing.Registry
	credits  *billing.CreditsService
}

// NewBillingHandler creates a provider-agnostic billing handler.
func NewBillingHandler(db *gorm.DB, registry *billing.Registry, credits *billing.CreditsService) *BillingHandler {
	return &BillingHandler{db: db, registry: registry, credits: credits}
}

type createCheckoutRequest struct {
	Provider   string `json:"provider"`
	PlanSlug   string `json:"plan_slug"`
	Currency   string `json:"currency"` // e.g. "USD", "NGN"
	Cycle      string `json:"cycle"`    // "monthly" | "annual"
	SuccessURL string `json:"success_url"`
	CancelURL  string `json:"cancel_url"`
}

type createCheckoutResponse struct {
	CheckoutURL string `json:"checkout_url"`
	AccessCode  string `json:"access_code,omitempty"` // popup flow: hand to PaystackPop().resumeTransaction()
	Reference   string `json:"reference,omitempty"`
}

// CreateCheckout creates a checkout session with the requested provider.
// @Summary Create checkout session
// @Description Creates a checkout session for subscribing to a plan. The client chooses the provider.
// @Tags billing
// @Accept json
// @Produce json
// @Param body body createCheckoutRequest true "Checkout request"
// @Success 200 {object} createCheckoutResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/billing/checkout [post]
func (h *BillingHandler) CreateCheckout(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	var body createCheckoutRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	provider, err := h.registry.Get(body.Provider)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown provider"})
		return
	}

	cycle := billing.Cycle(body.Cycle)
	if !cycle.IsValid() {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cycle must be 'monthly' or 'annual'"})
		return
	}
	if body.Currency == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "currency is required"})
		return
	}

	var plan model.Plan
	if err := h.db.Where("slug = ? AND active = true", body.PlanSlug).First(&plan).Error; err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown plan"})
		return
	}

	email, err := h.lookupOrgOwnerEmail(org.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to resolve org owner"})
		return
	}

	customerID, err := provider.EnsureCustomer(r.Context(), org.ID, email, org.Name)
	if err != nil {
		slog.Error("billing: failed to ensure customer", "provider", provider.Name(), "org_id", org.ID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create billing customer"})
		return
	}

	session, err := provider.CreateCheckout(r.Context(), customerID, billing.CheckoutIntent{
		OrgID:         org.ID,
		OrgName:       org.Name,
		CustomerEmail: email,
		PlanSlug:      plan.Slug,
		AmountMinor:   plan.PriceCents,
		Currency:      body.Currency,
		Cycle:         cycle,
		SuccessURL:    body.SuccessURL,
		CancelURL:     body.CancelURL,
		Metadata: map[string]string{
			"org_id":    org.ID.String(),
			"plan_slug": plan.Slug,
		},
	})
	if err != nil {
		switch {
		case errors.Is(err, billing.ErrUnsupportedCurrency):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "currency not supported by this provider"})
			return
		case errors.Is(err, billing.ErrUnknownPlan):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "plan not available on this provider"})
			return
		}
		slog.Error("billing: failed to create checkout", "provider", provider.Name(), "org_id", org.ID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create checkout session"})
		return
	}

	writeJSON(w, http.StatusOK, createCheckoutResponse{
		CheckoutURL: session.URL,
		AccessCode:  session.AccessCode,
		Reference:   session.ExternalID,
	})
}

// lookupOrgOwnerEmail returns the email of the earliest-joined member of the
// org. Used as the customer record email when provisioning a billing account.
func (h *BillingHandler) lookupOrgOwnerEmail(orgID any) (string, error) {
	var membership model.OrgMembership
	if err := h.db.Where("org_id = ?", orgID).Order("created_at ASC").First(&membership).Error; err != nil {
		return "", err
	}
	var user model.User
	if err := h.db.Where("id = ?", membership.UserID).First(&user).Error; err != nil {
		return "", err
	}
	return user.Email, nil
}
