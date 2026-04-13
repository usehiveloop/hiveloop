package handler

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/enqueue"
	"github.com/ziraloop/ziraloop/internal/mcp/catalog"
	"github.com/ziraloop/ziraloop/internal/model"
	"github.com/ziraloop/ziraloop/internal/tasks"
)

// IncomingWebhookHandler receives webhook events directly from external
// providers that require manual webhook URL configuration (e.g. Railway).
// Unlike the Nango webhook path, these arrive without an intermediary envelope.
type IncomingWebhookHandler struct {
	db       *gorm.DB
	enqueuer enqueue.TaskEnqueuer
}

// NewIncomingWebhookHandler creates an incoming webhook handler.
func NewIncomingWebhookHandler(db *gorm.DB, enqueuer enqueue.TaskEnqueuer) *IncomingWebhookHandler {
	return &IncomingWebhookHandler{db: db, enqueuer: enqueuer}
}

// Handle processes POST /incoming/webhooks/{provider}/{connectionID}.
//
// The endpoint is unauthenticated — the connectionID in the URL acts as a
// bearer token identifying the org and connection. Providers that support
// HMAC signing should be verified here; providers without signing (e.g.
// Railway) rely on the unguessable UUID for security.
// @Summary Receive incoming webhook from external provider
// @Description Receives webhook events directly from providers that require manual webhook URL configuration (e.g. Railway). The connection UUID in the URL identifies the org and connection.
// @Tags webhooks
// @Accept json
// @Produce json
// @Param provider path string true "Provider name (e.g. railway)"
// @Param connectionID path string true "Connection UUID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Router /incoming/webhooks/{provider}/{connectionID} [post]
func (h *IncomingWebhookHandler) Handle(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")
	connectionIDStr := chi.URLParam(r, "connectionID")

	connectionID, err := uuid.Parse(connectionIDStr)
	if err != nil {
		slog.Warn("incoming webhook: invalid connection ID",
			"provider", provider,
			"connection_id_raw", connectionIDStr,
		)
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid connection ID"})
		return
	}

	// Verify provider has triggers with webhook_config in the catalog.
	cat := catalog.Global()
	providerTriggers, hasTriggers := cat.GetProviderTriggers(provider)
	if !hasTriggers {
		providerTriggers, hasTriggers = cat.GetProviderTriggersForVariant(provider)
	}
	if !hasTriggers || providerTriggers.WebhookConfig == nil || !providerTriggers.WebhookConfig.WebhookURLRequired {
		slog.Warn("incoming webhook: provider not configured for direct webhooks",
			"provider", provider,
		)
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "provider not configured for direct webhooks"})
		return
	}

	// Read the raw body.
	body, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Error("incoming webhook: failed to read body",
			"provider", provider,
			"error", err,
		)
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "failed to read body"})
		return
	}

	if len(body) == 0 {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "empty body"})
		return
	}

	// Resolve connection → integration → org.
	var connection model.Connection
	if err := h.db.Preload("Integration").
		Where("id = ? AND revoked_at IS NULL", connectionID).
		First(&connection).Error; err != nil {
		slog.Warn("incoming webhook: connection not found",
			"provider", provider,
			"connection_id", connectionID,
			"error", err,
		)
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "connection not found"})
		return
	}

	if connection.Integration.DeletedAt != nil {
		slog.Warn("incoming webhook: integration deleted",
			"provider", provider,
			"connection_id", connectionID,
			"integration_id", connection.IntegrationID,
		)
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "integration not found"})
		return
	}

	// Infer event type from the raw payload.
	eventType, eventAction := inferDirectWebhookEvent(provider, body)
	if eventType == "" {
		slog.Warn("incoming webhook: could not determine event type",
			"provider", provider,
			"connection_id", connectionID,
			"body_size", len(body),
		)
		// Still return 200 to avoid the provider retrying.
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored", "reason": "unknown event type"})
		return
	}

	slog.Info("incoming webhook: received",
		"provider", provider,
		"connection_id", connectionID,
		"org_id", connection.OrgID,
		"event_type", eventType,
		"event_action", eventAction,
		"body_size", len(body),
	)

	// Return 200 immediately, then dispatch asynchronously.
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})

	deliveryID := connectionID.String() + ":" + uuid.New().String()
	task, err := tasks.NewRouterDispatchTask(tasks.TriggerDispatchPayload{
		Provider:     provider,
		EventType:    eventType,
		EventAction:  eventAction,
		DeliveryID:   deliveryID,
		OrgID:        connection.OrgID,
		ConnectionID: connectionID,
		PayloadJSON:  body,
	})
	if err != nil {
		slog.Error("incoming webhook: failed to build dispatch task",
			"provider", provider,
			"error", err,
		)
		return
	}

	if _, err := h.enqueuer.Enqueue(task); err != nil {
		slog.Error("incoming webhook: failed to enqueue dispatch task",
			"provider", provider,
			"error", err,
		)
		return
	}

	slog.Info("incoming webhook: dispatched",
		"provider", provider,
		"event_type", eventType,
		"event_action", eventAction,
		"delivery_id", deliveryID,
		"connection_id", connectionID,
	)
}

// inferDirectWebhookEvent extracts the event type and action from a raw
// webhook payload for providers that send webhooks directly (not via Nango).
func inferDirectWebhookEvent(provider string, body []byte) (eventType, eventAction string) {
	switch {
	case provider == "railway" || strings.HasPrefix(provider, "railway"):
		return inferRailwayEvent(body)
	}
	return "", ""
}

// inferRailwayEvent extracts the event type from a Railway webhook payload.
// Railway sends {"type": "Deployment.failed", ...} — the type field maps
// directly to trigger keys in railway.triggers.json.
func inferRailwayEvent(body []byte) (eventType, eventAction string) {
	var probe struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(body, &probe); err != nil || probe.Type == "" {
		return "", ""
	}

	// The type field is "Deployment.failed", "Deployment.success", etc.
	// Split into event type and action: "Deployment" + "failed".
	parts := strings.SplitN(probe.Type, ".", 2)
	if len(parts) == 2 {
		return probe.Type, parts[1]
	}
	return probe.Type, ""
}
