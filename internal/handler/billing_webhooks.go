package handler

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/billing"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// BillingWebhookHandler is the provider-agnostic webhook receiver. It is
// mounted at /internal/webhooks/{provider} — the path param selects which
// provider verifies and parses the event, after which the same application
// logic runs regardless of source.
type BillingWebhookHandler struct {
	db       *gorm.DB
	registry *billing.Registry
	credits  *billing.CreditsService
}

// NewBillingWebhookHandler creates the shared webhook router.
func NewBillingWebhookHandler(db *gorm.DB, registry *billing.Registry, credits *billing.CreditsService) *BillingWebhookHandler {
	return &BillingWebhookHandler{db: db, registry: registry, credits: credits}
}

// Handle verifies the signature with the named provider, parses the payload
// into a normalized event, and applies it to our data model.
func (h *BillingWebhookHandler) Handle(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "provider")
	provider, err := h.registry.Get(name)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown provider"})
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read body"})
		return
	}

	if err := provider.VerifyWebhook(r, body); err != nil {
		slog.Warn("billing webhook: signature verification failed", "provider", name, "error", err)
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "invalid signature"})
		return
	}

	event, err := provider.ParseEvent(body)
	if err != nil {
		slog.Error("billing webhook: failed to parse event", "provider", name, "error", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}

	slog.Info("billing webhook received", "provider", name, "type", event.Type, "raw_type", event.RawProviderType)

	if err := h.apply(provider.Name(), event); err != nil {
		slog.Error("billing webhook: apply failed", "provider", name, "type", event.Type, "error", err)
		// Return 200 anyway so the provider doesn't retry on logic errors —
		// signature was good, payload was good, our state just didn't fit.
	}

	w.WriteHeader(http.StatusOK)
}

// apply mutates local state based on the normalized event.
func (h *BillingWebhookHandler) apply(providerName string, event billing.Event) error {
	switch event.Type {
	case billing.EventSubscriptionActivated, billing.EventSubscriptionUpdated:
		return h.upsertSubscription(providerName, event)
	case billing.EventSubscriptionCanceled:
		return h.cancelSubscription(providerName, event)
	case billing.EventSubscriptionRevoked:
		return h.revokeSubscription(providerName, event)
	case billing.EventInvoicePaid:
		if err := h.snapshotPayment(providerName, event); err != nil {
			slog.Warn("billing webhook: snapshot payment failed", "provider", providerName, "error", err)
		}
		return h.grantPeriodCredits(providerName, event)
	case billing.EventPaymentFailed, billing.EventUnhandled:
		return nil
	}
	return nil
}

func (h *BillingWebhookHandler) upsertSubscription(providerName string, event billing.Event) error {
	state := event.Subscription
	if state == nil {
		return errors.New("subscription event missing state")
	}

	// For subscription.create (no prior row) we require org_id from customer
	// metadata. EnsureCustomer sets this on POST /customer, so it's always
	// present for API-created customers — which is the only way we create
	// subscriptions in production.
	orgID, err := h.resolveOrgID(providerName, state)
	if err != nil {
		return err
	}

	var plan model.Plan
	if err := h.db.Where("slug = ?", state.PlanSlug).First(&plan).Error; err != nil {
		return err
	}

	sub := model.Subscription{
		OrgID:                  orgID,
		PlanID:                 plan.ID,
		Provider:               providerName,
		ExternalSubscriptionID: state.ExternalSubscriptionID,
		ExternalCustomerID:     state.ExternalCustomerID,
		Status:                 string(state.Status),
		CurrentPeriodStart:     state.CurrentPeriodStart,
		CurrentPeriodEnd:       state.CurrentPeriodEnd,
		CanceledAt:             state.CanceledAt,
	}

	// Upsert by (provider, external_subscription_id).
	var existing model.Subscription
	err = h.db.Where("provider = ? AND external_subscription_id = ?",
		providerName, state.ExternalSubscriptionID).First(&existing).Error
	switch {
	case err == gorm.ErrRecordNotFound:
		if err := h.db.Create(&sub).Error; err != nil {
			return err
		}
	case err != nil:
		return err
	default:
		sub.ID = existing.ID
		sub.CreatedAt = existing.CreatedAt
		if err := h.db.Save(&sub).Error; err != nil {
			return err
		}
	}

	// Denormalise the plan slug onto the org so runtime checks stay cheap.
	return h.db.Model(&model.Org{}).Where("id = ?", orgID).
		Update("plan_slug", plan.Slug).Error
}

func (h *BillingWebhookHandler) cancelSubscription(providerName string, event billing.Event) error {
	state := event.Subscription
	if state == nil {
		return errors.New("subscription event missing state")
	}
	now := time.Now()
	canceledAt := state.CanceledAt
	if canceledAt == nil {
		canceledAt = &now
	}
	// No org_id needed — update is scoped by (provider, external_subscription_id).
	return h.db.Model(&model.Subscription{}).
		Where("provider = ? AND external_subscription_id = ?", providerName, state.ExternalSubscriptionID).
		Updates(map[string]any{
			"status":      string(billing.StatusCanceled),
			"canceled_at": canceledAt,
		}).Error
}

func (h *BillingWebhookHandler) revokeSubscription(providerName string, event billing.Event) error {
	state := event.Subscription
	if state == nil {
		return errors.New("subscription event missing state")
	}

	orgID, err := h.resolveOrgID(providerName, state)
	if err != nil {
		return err
	}

	if err := h.db.Model(&model.Subscription{}).
		Where("provider = ? AND external_subscription_id = ?", providerName, state.ExternalSubscriptionID).
		Update("status", string(billing.StatusRevoked)).Error; err != nil {
		return err
	}

	// If no active subscription remains for the org, drop back to free.
	var activeCount int64
	h.db.Model(&model.Subscription{}).
		Where("org_id = ? AND status = ?", orgID, string(billing.StatusActive)).
		Count(&activeCount)
	if activeCount == 0 {
		return h.db.Model(&model.Org{}).Where("id = ?", orgID).
			Update("plan_slug", "free").Error
	}
	return nil
}

// grantPeriodCredits adds the plan's monthly credit allowance to the org
// ledger, tagged with the subscription id so double-grants for the same
// invoice can be detected by the caller if needed.
func (h *BillingWebhookHandler) grantPeriodCredits(providerName string, event billing.Event) error {
	state := event.Subscription
	if state == nil {
		return nil
	}
	orgID, err := h.resolveOrgID(providerName, state)
	if err != nil {
		return err
	}

	var plan model.Plan
	if err := h.db.Where("slug = ?", state.PlanSlug).First(&plan).Error; err != nil {
		return err
	}
	if plan.MonthlyCredits <= 0 {
		return nil
	}

	// Plan-grant credits expire at the end of the billing period plus a
	// 3-day grace window, so a cycle-boundary spend isn't refused while the
	// next invoice's webhook is still in flight.
	var expiresAt *time.Time
	if !state.CurrentPeriodEnd.IsZero() {
		t := state.CurrentPeriodEnd.Add(billing.PlanGrantGracePeriod)
		expiresAt = &t
	}

	return h.credits.Grant(
		orgID,
		plan.MonthlyCredits,
		billing.ReasonPlanGrant,
		"subscription",
		state.ExternalSubscriptionID,
		expiresAt,
	)
}

// resolveOrgID returns the org for a subscription event. When the provider
// echoed our customer metadata (EnsureCustomer sets {org_id, org_name}),
// state.OrgID is already populated. Otherwise we look up an existing
// subscription row by (provider, external_subscription_id) or
// (provider, external_customer_id) — useful for lifecycle events on
// subscriptions we already know about. Fails loudly rather than writing a
// zero-UUID row.
func (h *BillingWebhookHandler) resolveOrgID(providerName string, state *billing.SubscriptionState) (uuid.UUID, error) {
	if state.OrgID != uuid.Nil {
		return state.OrgID, nil
	}
	q := h.db.Model(&model.Subscription{}).Where("provider = ?", providerName)
	switch {
	case state.ExternalSubscriptionID != "":
		q = q.Where("external_subscription_id = ?", state.ExternalSubscriptionID)
	case state.ExternalCustomerID != "":
		q = q.Where("external_customer_id = ?", state.ExternalCustomerID)
	default:
		return uuid.Nil, errors.New("event has no correlation identifier (no org_id, no subscription_code, no customer_code)")
	}
	var sub model.Subscription
	if err := q.First(&sub).Error; err != nil {
		return uuid.Nil, fmt.Errorf("resolve org from %s event: %w", providerName, err)
	}
	return sub.OrgID, nil
}

// ResolveOrgIDForTest exposes resolveOrgID for package-external tests.
// Not intended for production callers — the webhook handler drives it
// internally from Handle.
func (h *BillingWebhookHandler) ResolveOrgIDForTest(providerName string, state *billing.SubscriptionState) (uuid.UUID, error) {
	return h.resolveOrgID(providerName, state)
}

