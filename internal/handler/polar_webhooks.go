package handler

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	standardwebhooks "github.com/standard-webhooks/standard-webhooks/libraries/go"

	"github.com/usehiveloop/hiveloop/internal/model"
)

// PolarWebhookHandler receives and verifies webhook events from Polar.
type PolarWebhookHandler struct {
	db             *gorm.DB
	webhookSecret  string
	freeProductID  string
}

// NewPolarWebhookHandler creates a Polar webhook handler.
func NewPolarWebhookHandler(db *gorm.DB, webhookSecret string, freeProductID string) *PolarWebhookHandler {
	return &PolarWebhookHandler{
		db:            db,
		webhookSecret: webhookSecret,
		freeProductID: freeProductID,
	}
}

// polarWebhookEnvelope is the top-level structure of a Polar webhook event.
type polarWebhookEnvelope struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// polarSubscriptionData holds the subscription fields we care about.
type polarSubscriptionData struct {
	ID                 string     `json:"id"`
	Status             string     `json:"status"`
	ProductID          string     `json:"product_id"`
	CustomerID         string     `json:"customer_id"`
	CurrentPeriodStart *time.Time `json:"current_period_start"`
	CurrentPeriodEnd   *time.Time `json:"current_period_end"`
	CanceledAt         *time.Time `json:"canceled_at"`
	Metadata           map[string]any `json:"metadata"`
	Customer           *polarCustomerRef `json:"customer"`
}

type polarCustomerRef struct {
	ExternalID *string `json:"external_id"`
}

// Handle processes incoming Polar webhook events.
func (h *PolarWebhookHandler) Handle(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read body"})
		return
	}

	// Verify webhook signature — Polar uses Standard Webhooks.
	// The secret must be base64-encoded for the library.
	encodedSecret := base64.StdEncoding.EncodeToString([]byte(h.webhookSecret))
	webhook, err := standardwebhooks.NewWebhook(encodedSecret)
	if err != nil {
		slog.Error("polar webhook: failed to create verifier", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "webhook verification setup failed"})
		return
	}

	if err := webhook.Verify(body, r.Header); err != nil {
		slog.Warn("polar webhook: signature verification failed", "error", err)
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "invalid signature"})
		return
	}

	var envelope polarWebhookEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}

	slog.Info("polar webhook received", "type", envelope.Type)

	switch envelope.Type {
	case "subscription.created", "subscription.active", "subscription.updated":
		h.handleSubscriptionUpsert(envelope.Data)
	case "subscription.canceled":
		h.handleSubscriptionCanceled(envelope.Data)
	case "subscription.revoked":
		h.handleSubscriptionRevoked(envelope.Data)
	default:
		slog.Info("polar webhook: unhandled event type", "type", envelope.Type)
	}

	w.WriteHeader(http.StatusOK)
}

func (h *PolarWebhookHandler) handleSubscriptionUpsert(data json.RawMessage) {
	var subData polarSubscriptionData
	if err := json.Unmarshal(data, &subData); err != nil {
		slog.Error("polar webhook: failed to parse subscription data", "error", err)
		return
	}

	// Resolve org from customer's external_id (which is the org UUID)
	orgID := h.resolveOrgID(subData)
	if orgID == "" {
		slog.Error("polar webhook: could not resolve org from subscription", "subscription_id", subData.ID)
		return
	}

	// Upsert subscription record
	subscription := model.Subscription{
		PolarSubscriptionID: subData.ID,
	}

	result := h.db.Where("polar_subscription_id = ?", subData.ID).First(&subscription)

	subscription.PolarProductID = subData.ProductID
	subscription.Status = subData.Status
	if subData.CurrentPeriodStart != nil {
		subscription.CurrentPeriodStart = *subData.CurrentPeriodStart
	}
	if subData.CurrentPeriodEnd != nil {
		subscription.CurrentPeriodEnd = *subData.CurrentPeriodEnd
	}

	if result.Error == gorm.ErrRecordNotFound {
		// Create new subscription
		parsedOrgID, parseErr := uuid.Parse(orgID)
		if parseErr != nil {
			slog.Error("polar webhook: invalid org UUID", "org_id", orgID, "error", parseErr)
			return
		}
		subscription.OrgID = parsedOrgID
		subscription.ProductType = h.resolveProductType(subData.ProductID)
		if err := h.db.Create(&subscription).Error; err != nil {
			slog.Error("polar webhook: failed to create subscription", "error", err)
			return
		}
		slog.Info("polar webhook: subscription created", "subscription_id", subData.ID, "org_id", orgID)
	} else if result.Error == nil {
		if err := h.db.Save(&subscription).Error; err != nil {
			slog.Error("polar webhook: failed to update subscription", "error", err)
			return
		}
		slog.Info("polar webhook: subscription updated", "subscription_id", subData.ID, "status", subData.Status)
	}

	// Update org billing plan based on product
	billingPlan := "pro"
	if subData.ProductID == h.freeProductID {
		billingPlan = "free"
	}
	h.db.Model(&model.Org{}).Where("id = ?", orgID).Update("billing_plan", billingPlan)

	slog.Info("polar webhook: org billing plan updated", "org_id", orgID, "billing_plan", billingPlan, "product_id", subData.ProductID)
}

func (h *PolarWebhookHandler) handleSubscriptionCanceled(data json.RawMessage) {
	var subData polarSubscriptionData
	if err := json.Unmarshal(data, &subData); err != nil {
		slog.Error("polar webhook: failed to parse subscription data", "error", err)
		return
	}

	now := time.Now()
	h.db.Model(&model.Subscription{}).
		Where("polar_subscription_id = ?", subData.ID).
		Updates(map[string]any{
			"status":      "canceled",
			"canceled_at": &now,
		})

	slog.Info("polar webhook: subscription canceled", "subscription_id", subData.ID)
}

func (h *PolarWebhookHandler) handleSubscriptionRevoked(data json.RawMessage) {
	var subData polarSubscriptionData
	if err := json.Unmarshal(data, &subData); err != nil {
		slog.Error("polar webhook: failed to parse subscription data", "error", err)
		return
	}

	h.db.Model(&model.Subscription{}).
		Where("polar_subscription_id = ?", subData.ID).
		Update("status", "revoked")

	// Downgrade org to free
	orgID := h.resolveOrgID(subData)
	if orgID != "" {
		// Only downgrade if no other active subscriptions remain
		var activeCount int64
		h.db.Model(&model.Subscription{}).
			Where("org_id = ? AND status = 'active' AND polar_subscription_id != ?", orgID, subData.ID).
			Count(&activeCount)
		if activeCount == 0 {
			h.db.Model(&model.Org{}).Where("id = ?", orgID).Update("billing_plan", "free")
		}
	}

	slog.Info("polar webhook: subscription revoked", "subscription_id", subData.ID)
}

// resolveOrgID extracts the org ID from the customer's external_id.
func (h *PolarWebhookHandler) resolveOrgID(subData polarSubscriptionData) string {
	// Try customer external_id first (set during checkout as org UUID)
	if subData.Customer != nil && subData.Customer.ExternalID != nil {
		return *subData.Customer.ExternalID
	}

	// Fallback: look up by PolarCustomerID
	if subData.CustomerID != "" {
		var org model.Org
		if err := h.db.Where("polar_customer_id = ?", subData.CustomerID).First(&org).Error; err == nil {
			return org.ID.String()
		}
	}

	return ""
}

// resolveProductType maps a Polar product ID to our internal product type.
// Falls back to "pro_shared" if the product ID is not recognized.
func (h *PolarWebhookHandler) resolveProductType(_ string) string {
	// Product type resolution happens at a higher level (config-based).
	// For now, default to pro_shared. The billing handler sets this correctly
	// during checkout by including it in metadata.
	return "pro_shared"
}

