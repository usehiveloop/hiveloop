package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/crypto"
	"github.com/ziraloop/ziraloop/internal/enqueue"
	"github.com/ziraloop/ziraloop/internal/model"
)

// NangoWebhookHandler receives webhook events forwarded by Nango.
type NangoWebhookHandler struct {
	db          *gorm.DB
	nangoSecret string
	encKey      *crypto.SymmetricKey
	httpClient  *http.Client
	enqueuer    enqueue.TaskEnqueuer
}

// NewNangoWebhookHandler creates a Nango webhook handler.
func NewNangoWebhookHandler(db *gorm.DB, nangoSecret string, encKey *crypto.SymmetricKey, enqueuer ...enqueue.TaskEnqueuer) *NangoWebhookHandler {
	h := &NangoWebhookHandler{
		db:          db,
		nangoSecret: nangoSecret,
		encKey:      encKey,
		httpClient:  &http.Client{Timeout: 25 * time.Second},
	}
	if len(enqueuer) > 0 {
		h.enqueuer = enqueuer[0]
	}
	return h
}

// nangoWebhook is the envelope for all Nango webhook types.
type nangoWebhook struct {
	From              string          `json:"from"`
	Type              string          `json:"type"`
	ConnectionID      string          `json:"connectionId"`
	ProviderConfigKey string          `json:"providerConfigKey"`
	Provider          string          `json:"provider,omitempty"`
	Operation         string          `json:"operation,omitempty"`
	Success           *bool           `json:"success,omitempty"`
	Payload           json.RawMessage `json:"payload,omitempty"`
}

// webhookPayload is the enriched payload sent to the org's webhook endpoint.
// Nango-specific fields are stripped; ZiraLoop IDs are used instead.
type webhookPayload struct {
	Type      string          `json:"type"`
	Provider  string          `json:"provider"`
	Operation string          `json:"operation,omitempty"`
	Success   *bool           `json:"success,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`

	OrgID           string `json:"org_id"`
	IntegrationID   string `json:"integration_id,omitempty"`
	IntegrationName string `json:"integration_name,omitempty"`
	ConnectionID    string `json:"connection_id,omitempty"`
}

// webhookContext holds resolved entities from a Nango webhook.
type webhookContext struct {
	orgID        uuid.UUID
	inConnection *model.InConnection
}

// Handle processes POST /internal/webhooks/nango.
func (h *NangoWebhookHandler) Handle(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Error("nango webhook: failed to read request body", "error", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read body"})
		return
	}

	slog.Info("nango webhook: received",
		"body_size", len(body),
		"content_type", r.Header.Get("Content-Type"),
		"has_signature", r.Header.Get("X-Nango-Hmac-Sha256") != "",
		"raw_body", string(body),
	)

	signature := r.Header.Get("X-Nango-Hmac-Sha256")
	if signature == "" {
		slog.Warn("nango webhook: missing signature header")
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing signature header"})
		return
	}
	if !verifyNangoSignature(body, h.nangoSecret, signature) {
		slog.Warn("nango webhook: invalid signature")
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid signature"})
		return
	}

	var wh nangoWebhook
	if err := json.Unmarshal(body, &wh); err != nil {
		slog.Error("nango webhook: failed to parse payload", "error", err, "body", string(body))
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid webhook payload"})
		return
	}

	slog.Info("nango webhook: parsed",
		"type", wh.Type,
		"from", wh.From,
		"provider", wh.Provider,
		"provider_config_key", wh.ProviderConfigKey,
		"nango_connection_id", wh.ConnectionID,
		"operation", wh.Operation,
		"success", wh.Success,
		"payload_size", len(wh.Payload),
	)

	wctx := h.identify(&wh)
	if wctx == nil {
		slog.Info("nango webhook: no forwarding target, acknowledging")
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}

	// Dispatch agent triggers in parallel with the existing org-webhook forward.
	// The two paths are independent: org-webhook forwards the enriched payload
	// to a customer-supplied URL, while trigger dispatch evaluates AgentTriggers
	// against the payload and queues per-agent runs. Failures in one don't
	// affect the other.
	dispatchWebhookEvent(h.enqueuer, &wh, wctx)

	h.acknowledge(w)
}

// acknowledge responds with a 200 OK to the Nango webhook.
func (h *NangoWebhookHandler) acknowledge(w http.ResponseWriter) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// buildEnrichedBody builds the JSON body for the enriched webhook payload.
func (h *NangoWebhookHandler) buildEnrichedBody(wh *nangoWebhook, wctx *webhookContext) []byte {
	provider := wh.Provider
	payload := webhookPayload{
		Type:      wh.Type,
		Provider:  provider,
		Operation: wh.Operation,
		Success:   wh.Success,
		Payload:   wh.Payload,
		OrgID:     wctx.orgID.String(),
	}
	if wctx.inConnection != nil {
		payload.IntegrationID = wctx.inConnection.InIntegrationID.String()
		payload.IntegrationName = wctx.inConnection.InIntegration.DisplayName
		payload.ConnectionID = wctx.inConnection.ID.String()
		if provider == "" {
			payload.Provider = wctx.inConnection.InIntegration.Provider
		}
	}

	body, _ := json.Marshal(payload)
	return body
}

// identify resolves the org, integration, and connection from the webhook.
func (h *NangoWebhookHandler) identify(wh *nangoWebhook) *webhookContext {
	if strings.HasPrefix(wh.ProviderConfigKey, "in_") {
		slog.Info("nango webhook: in-integration event",
			"type", wh.Type,
			"provider_config_key", wh.ProviderConfigKey,
			"nango_connection_id", wh.ConnectionID,
			"operation", wh.Operation,
			"success", wh.Success,
			"provider", wh.Provider,
		)
		return nil
	}

	orgID, uniqueKey, ok := parseProviderConfigKey(wh.ProviderConfigKey)
	if !ok {
		slog.Warn("nango webhook: unable to parse provider config key",
			"provider_config_key", wh.ProviderConfigKey,
			"type", wh.Type,
		)
		return nil
	}

	slog.Info("nango webhook: resolved org from config key",
		"org_id", orgID,
		"unique_key", uniqueKey,
	)

	wctx := &webhookContext{orgID: orgID}

	var inConnection model.InConnection
	if err := h.db.Preload("InIntegration").
		Where("nango_connection_id = ? AND org_id = ? AND revoked_at IS NULL",
			wh.ConnectionID, orgID).First(&inConnection).Error; err != nil {
		slog.Warn("nango webhook: in-connection not found",
			"org_id", orgID,
			"nango_connection_id", wh.ConnectionID,
			"type", wh.Type,
			"error", err,
		)
		return wctx
	}
	wctx.inConnection = &inConnection

	logAttrs := []any{
		"type", wh.Type,
		"provider", inConnection.InIntegration.Provider,
		"org_id", orgID,
		"connection_id", inConnection.ID,
		"nango_connection_id", wh.ConnectionID,
	}
	if wh.Type == "auth" {
		logAttrs = append(logAttrs, "operation", wh.Operation)
		if wh.Success != nil {
			logAttrs = append(logAttrs, "success", *wh.Success)
		}
	}
	if wh.Type == "forward" {
		logAttrs = append(logAttrs, "payload_size", len(wh.Payload))
	}
	slog.Info("nango webhook: fully resolved", logAttrs...)

	return wctx
}

// parseProviderConfigKey splits "{orgID}_{uniqueKey}" into its parts.
func parseProviderConfigKey(key string) (uuid.UUID, string, bool) {
	parts := strings.SplitN(key, "_", 2)
	if len(parts) != 2 {
		return uuid.Nil, "", false
	}
	orgID, err := uuid.Parse(parts[0])
	if err != nil {
		return uuid.Nil, "", false
	}
	return orgID, parts[1], true
}

// verifyNangoSignature verifies the HMAC-SHA256 signature from Nango.
// Nango signs with: HMAC-SHA256(secret, rawBody), hex-encoded.
func verifyNangoSignature(body []byte, secret string, signature string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}
