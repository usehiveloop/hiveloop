package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/billing"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// BillingHandler exposes checkout, portal, and subscription endpoints against
// whatever billing provider the caller picks. The handler has no knowledge of
// any specific provider — it goes through billing.Registry.
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

type createPortalRequest struct {
	Provider string `json:"provider"`
}

type portalResponse struct {
	PortalURL string `json:"portal_url"`
}

// CreatePortal creates a provider customer portal session for the org.
// @Summary Create billing portal session
// @Description Creates a customer portal session where the user can manage their subscription, payment methods, and invoices.
// @Tags billing
// @Accept json
// @Produce json
// @Param body body createPortalRequest true "Portal request"
// @Success 200 {object} portalResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/billing/portal [post]
func (h *BillingHandler) CreatePortal(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	var body createPortalRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	provider, err := h.registry.Get(body.Provider)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown provider"})
		return
	}

	// The customer must already exist with this provider, which means there's
	// at least one subscription row for (org, provider).
	var sub model.Subscription
	if err := h.db.Where("org_id = ? AND provider = ?", org.ID, provider.Name()).
		Order("created_at DESC").First(&sub).Error; err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no billing account with this provider"})
		return
	}

	session, err := provider.CreatePortal(r.Context(), billing.PortalRequest{
		OrgID:                  org.ID,
		ExternalCustomerID:     sub.ExternalCustomerID,
		ExternalSubscriptionID: sub.ExternalSubscriptionID,
	})
	if err != nil {
		if errors.Is(err, billing.ErrNoActiveSubscription) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no active subscription"})
			return
		}
		slog.Error("billing: failed to create portal", "provider", provider.Name(), "org_id", org.ID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create portal session"})
		return
	}

	writeJSON(w, http.StatusOK, portalResponse{PortalURL: session.URL})
}

type subscriptionResponse struct {
	PlanSlug        string `json:"plan_slug"`
	Status          string `json:"status"`
	Provider        string `json:"provider,omitempty"`
	CreditsBalance  int64  `json:"credits_balance"`
	CurrentPeriodEnd string `json:"current_period_end,omitempty"`
}

// GetSubscription returns the org's current subscription and credit balance.
// @Summary Get subscription status
// @Description Returns the org's active plan, provider, and credit balance.
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
		resp.Provider = sub.Provider
		resp.Status = sub.Status
		if !sub.CurrentPeriodEnd.IsZero() {
			resp.CurrentPeriodEnd = sub.CurrentPeriodEnd.Format("2006-01-02T15:04:05Z07:00")
		}
	} else if err != gorm.ErrRecordNotFound {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load subscription"})
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

type verifyRequest struct {
	PlanSlug string `json:"plan_slug"`
}

type verifyResponse struct {
	Status   string `json:"status"`              // "active" | "timeout"
	PlanSlug string `json:"plan_slug,omitempty"`
}

// Verify polls the local DB for a Subscription matching (org, plan_slug,
// status=active). Returns as soon as one appears or after a 5s timeout.
//
// Used by the popup checkout flow: the browser fires this immediately after
// react-paystack's onSuccess callback, and the response confirms the
// subscription.create / charge.success webhook landed and our state has
// caught up. Webhooks are still the source of truth — this endpoint just
// surfaces "is it ready to render?" to the client.
//
// @Summary Verify subscription is active
// @Description Waits up to 5s for the subscription webhook to land and the local Subscription row to become active for the requested plan.
// @Tags billing
// @Accept json
// @Produce json
// @Param body body verifyRequest true "Plan to wait for"
// @Success 200 {object} verifyResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 408 {object} errorResponse
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
	if body.PlanSlug == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "plan_slug is required"})
		return
	}

	deadline := time.Now().Add(5 * time.Second)
	tick := 300 * time.Millisecond
	ctx := r.Context()

	for {
		var sub model.Subscription
		err := h.db.
			Joins("JOIN plans ON plans.id = subscriptions.plan_id").
			Where("subscriptions.org_id = ? AND plans.slug = ? AND subscriptions.status = ?",
				org.ID, body.PlanSlug, string(billing.StatusActive)).
			Order("subscriptions.created_at DESC").
			First(&sub).Error
		if err == nil {
			writeJSON(w, http.StatusOK, verifyResponse{Status: "active", PlanSlug: body.PlanSlug})
			return
		}
		if err != gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to query subscription"})
			return
		}

		if time.Now().After(deadline) {
			writeJSON(w, http.StatusRequestTimeout, verifyResponse{Status: "timeout", PlanSlug: body.PlanSlug})
			return
		}

		select {
		case <-ctx.Done():
			writeJSON(w, http.StatusRequestTimeout, verifyResponse{Status: "timeout", PlanSlug: body.PlanSlug})
			return
		case <-time.After(tick):
		}
	}
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
