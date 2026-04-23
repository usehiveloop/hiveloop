package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/enqueue"
	"github.com/usehiveloop/hiveloop/internal/mcp/catalog"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/tasks"
)

// maxIncomingWebhookBodyBytes caps the size of a single incoming webhook
// payload at 10 MiB to prevent memory-exhaustion DoS. Legitimate webhook
// payloads from supported providers are well under this limit.
const maxIncomingWebhookBodyBytes = 10 * 1024 * 1024

// providersWithoutNativeSignatures is the set of providers that do not
// support HMAC webhook signing. They rely on the unguessable connection
// UUID as the sole authentication factor (defense-in-depth limitation
// tracked in issue #42).
var providersWithoutNativeSignatures = map[string]bool{
	"railway": true,
}

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
// Authentication is two-layered:
//   - The connectionID UUID in the URL identifies the org and connection.
//   - For providers that support HMAC signatures (e.g. GitHub, Slack) the
//     signature is verified against the connection's stored webhook secret
//     and the request is rejected on mismatch.
//
// Providers listed in providersWithoutNativeSignatures rely on the UUID
// alone (tracked in issue #42 — remove once all providers gain native
// signature support or an explicit shared-secret opt-in).
//
// The request body is capped at maxIncomingWebhookBodyBytes to protect
// against memory exhaustion attacks (issue #44).
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

	// Read the raw body with a hard size cap (issue #44).
	r.Body = http.MaxBytesReader(w, r.Body, maxIncomingWebhookBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			slog.Warn("incoming webhook: body exceeds size limit",
				"provider", provider,
				"limit_bytes", maxIncomingWebhookBodyBytes,
			)
			writeJSON(w, http.StatusRequestEntityTooLarge, errorResponse{Error: "request body too large"})
			return
		}
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
	var connection model.InConnection
	if err := h.db.Preload("InIntegration").
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

	if connection.InIntegration.DeletedAt != nil {
		slog.Warn("incoming webhook: integration deleted",
			"provider", provider,
			"connection_id", connectionID,
			"integration_id", connection.InIntegrationID,
		)
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "integration not found"})
		return
	}

	// Per-provider signature verification (issue #42). When the provider
	// supports HMAC signing, require a matching signature against the
	// connection's stored webhook secret. For providers known to lack
	// native signing (tracked in providersWithoutNativeSignatures) fall
	// back to connectionID-only authentication and log a warning.
	if !providersWithoutNativeSignatures[provider] {
		if err := verifyIncomingWebhookSignature(provider, r, body, &connection); err != nil {
			slog.Warn("incoming webhook: signature verification failed",
				"provider", provider,
				"connection_id", connectionID,
				"error", err,
			)
			writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "invalid signature"})
			return
		}
	} else {
		slog.Debug("incoming webhook: provider has no native signing, authenticated by connection UUID only",
			"provider", provider,
			"connection_id", connectionID,
		)
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

// verifyIncomingWebhookSignature dispatches to the appropriate provider
// signature verifier. Returns nil on success, an error describing the
// failure otherwise. Providers are expected to store their HMAC secret
// in the connection's credentials via the integration config; if no
// secret is configured for a provider that would normally sign, the
// request is rejected.
func verifyIncomingWebhookSignature(provider string, r *http.Request, body []byte, conn *model.InConnection) error {
	secret := extractWebhookSecret(conn)
	if secret == "" {
		// No secret configured; cannot verify. Reject to fail closed.
		return errors.New("no webhook secret configured for connection")
	}
	switch {
	case provider == "github" || strings.HasPrefix(provider, "github"):
		return verifyGitHubSignature(r, body, secret)
	case provider == "slack" || strings.HasPrefix(provider, "slack"):
		return verifySlackSignature(r, body, secret)
	default:
		// Unknown provider and not in the allowlist — fail closed.
		return fmt.Errorf("no signature verifier for provider %q", provider)
	}
}

// extractWebhookSecret returns the webhook secret configured for the
// connection, if any. The schema is still evolving; once InConnection
// gains a dedicated webhook_secret column this helper becomes trivial.
// For now it returns an empty string, causing verification to fail
// closed for signature-capable providers until the plumbing lands.
// TODO(security-42): populate from connection/integration config.
func extractWebhookSecret(_ *model.InConnection) string {
	return ""
}

// verifyGitHubSignature validates the X-Hub-Signature-256 header.
// GitHub signs with HMAC-SHA256 over the raw request body and formats
// the header as "sha256=<hex>".
func verifyGitHubSignature(r *http.Request, body []byte, secret string) error {
	header := r.Header.Get("X-Hub-Signature-256")
	if header == "" {
		return errors.New("missing X-Hub-Signature-256 header")
	}
	const prefix = "sha256="
	if !strings.HasPrefix(header, prefix) {
		return errors.New("malformed X-Hub-Signature-256 header")
	}
	provided, err := hex.DecodeString(header[len(prefix):])
	if err != nil {
		return fmt.Errorf("invalid signature hex: %w", err)
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := mac.Sum(nil)
	if !hmac.Equal(expected, provided) {
		return errors.New("signature mismatch")
	}
	return nil
}

// verifySlackSignature validates Slack's X-Slack-Signature header using
// the timestamp and signing secret. Rejects requests with timestamps
// older than 5 minutes to resist replay attacks.
func verifySlackSignature(r *http.Request, body []byte, secret string) error {
	timestamp := r.Header.Get("X-Slack-Request-Timestamp")
	signature := r.Header.Get("X-Slack-Signature")
	if timestamp == "" || signature == "" {
		return errors.New("missing slack signature headers")
	}
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid timestamp: %w", err)
	}
	if abs(time.Now().Unix()-ts) > 300 {
		return errors.New("timestamp outside acceptable window")
	}
	const prefix = "v0="
	if !strings.HasPrefix(signature, prefix) {
		return errors.New("malformed X-Slack-Signature header")
	}
	provided, err := hex.DecodeString(signature[len(prefix):])
	if err != nil {
		return fmt.Errorf("invalid signature hex: %w", err)
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("v0:" + timestamp + ":"))
	mac.Write(body)
	expected := mac.Sum(nil)
	if !hmac.Equal(expected, provided) {
		return errors.New("signature mismatch")
	}
	return nil
}

func abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
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
