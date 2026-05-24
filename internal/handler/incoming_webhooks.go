package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/mcp/catalog"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/tasks"
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
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid connection ID"})
		return
	}

	cat := catalog.Global()
	providerTriggers, hasTriggers := cat.GetProviderTriggers(provider)
	if !hasTriggers {
		providerTriggers, hasTriggers = cat.GetProviderTriggersForVariant(provider)
	}
	if !hasTriggers || providerTriggers.WebhookConfig == nil || !providerTriggers.WebhookConfig.WebhookURLRequired {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "provider not configured for direct webhooks"})
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "incoming webhook: failed to read body",
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

	var connection model.Connection
	if err := h.db.Preload("Integration").
		Where("id = ? AND revoked_at IS NULL", connectionID).
		First(&connection).Error; err != nil {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "connection not found"})
		return
	}

	if connection.Integration.DeletedAt != nil {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "integration not found"})
		return
	}

	eventType, eventAction := inferDirectWebhookEvent(provider, body)
	if eventType == "" {

		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored", "reason": "unknown event type"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})

	deliveryID := connectionID.String() + ":" + uuid.New().String()
	task, err := tasks.NewEmployeeTriggerDispatchTask(tasks.EmployeeTriggerDispatchPayload{
		Provider:     provider,
		EventType:    eventType,
		EventAction:  eventAction,
		DeliveryID:   deliveryID,
		OrgID:        connection.OrgID,
		ConnectionID: connectionID,
		PayloadJSON:  body,
	})
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "incoming webhook: failed to build dispatch task",
			"provider", provider,
			"error", err,
		)
		logging.CaptureWithFields(r.Context(), fmt.Errorf("incoming webhook: failed to build dispatch task: %w", err), map[string]any{
			"org_id":      connection.OrgID.String(),
			"delivery_id": deliveryID,
			"event_key":   eventKeyForHandler(eventType, eventAction),
		})
		return
	}

	if _, err := h.enqueuer.Enqueue(task); err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "incoming webhook: failed to enqueue dispatch task",
			"provider", provider,
			"error", err,
		)
		logging.CaptureWithFields(r.Context(), fmt.Errorf("incoming webhook: failed to enqueue dispatch task: %w", err), map[string]any{
			"org_id":      connection.OrgID.String(),
			"delivery_id": deliveryID,
			"event_key":   eventKeyForHandler(eventType, eventAction),
		})
		return
	}
}

func eventKeyForHandler(eventType, eventAction string) string {
	if eventAction == "" {
		return eventType
	}
	return eventType + "." + eventAction
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
// Railway sends {"type": "Deployment.failed", ...}. The type field maps
// directly to trigger keys — no splitting needed.
func inferRailwayEvent(body []byte) (eventType, eventAction string) {
	var probe struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(body, &probe); err != nil || probe.Type == "" {
		return "", ""
	}
	return probe.Type, ""
}
