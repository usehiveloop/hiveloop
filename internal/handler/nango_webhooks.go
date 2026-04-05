package handler

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/crypto"
	"github.com/ziraloop/ziraloop/internal/model"
)

// NangoWebhookHandler receives webhook events forwarded by Nango.
type NangoWebhookHandler struct {
	db          *gorm.DB
	nangoSecret string
	encKey      *crypto.SymmetricKey
	httpClient  *http.Client
}

// NewNangoWebhookHandler creates a Nango webhook handler.
func NewNangoWebhookHandler(db *gorm.DB, nangoSecret string, encKey *crypto.SymmetricKey) *NangoWebhookHandler {
	return &NangoWebhookHandler{
		db:          db,
		nangoSecret: nangoSecret,
		encKey:      encKey,
		httpClient:  &http.Client{Timeout: 25 * time.Second},
	}
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
	IdentityID      string `json:"identity_id,omitempty"`
}

// webhookContext holds resolved entities from a Nango webhook.
type webhookContext struct {
	orgID       uuid.UUID
	integration *model.Integration
	connection  *model.Connection
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

	statusCode, respBody, forwarded := h.forwardToOrg(r.Context(), &wh, wctx)
	if !forwarded {
		slog.Info("nango webhook: no org webhook config, acknowledging",
			"org_id", wctx.orgID,
		)
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}

	slog.Info("nango webhook: forwarding complete",
		"org_id", wctx.orgID,
		"response_status", statusCode,
		"response_size", len(respBody),
	)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if respBody != nil {
		w.Write(respBody)
	}
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

	var integration model.Integration
	if err := h.db.Where("org_id = ? AND unique_key = ? AND deleted_at IS NULL", orgID, uniqueKey).
		First(&integration).Error; err != nil {
		slog.Warn("nango webhook: integration not found",
			"org_id", orgID,
			"unique_key", uniqueKey,
			"type", wh.Type,
			"error", err,
		)
		return wctx
	}
	wctx.integration = &integration

	slog.Info("nango webhook: resolved integration",
		"org_id", orgID,
		"integration_id", integration.ID,
		"provider", integration.Provider,
		"display_name", integration.DisplayName,
	)

	var connection model.Connection
	if err := h.db.Where("nango_connection_id = ? AND integration_id = ? AND revoked_at IS NULL",
		wh.ConnectionID, integration.ID).First(&connection).Error; err != nil {
		slog.Warn("nango webhook: connection not found",
			"org_id", orgID,
			"integration_id", integration.ID,
			"nango_connection_id", wh.ConnectionID,
			"type", wh.Type,
			"error", err,
		)
		return wctx
	}
	wctx.connection = &connection

	logAttrs := []any{
		"type", wh.Type,
		"provider", integration.Provider,
		"org_id", orgID,
		"integration_id", integration.ID,
		"connection_id", connection.ID,
		"nango_connection_id", wh.ConnectionID,
	}
	if connection.IdentityID != nil {
		logAttrs = append(logAttrs, "identity_id", *connection.IdentityID)
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

// forwardToOrg forwards the enriched webhook to the org's configured endpoint.
func (h *NangoWebhookHandler) forwardToOrg(
	ctx context.Context,
	wh *nangoWebhook,
	wctx *webhookContext,
) (statusCode int, respBody []byte, forwarded bool) {
	var config model.OrgWebhookConfig
	if err := h.db.Where("org_id = ?", wctx.orgID).First(&config).Error; err != nil {
		return 0, nil, false
	}

	slog.Info("nango webhook: org webhook config found",
		"org_id", wctx.orgID,
		"webhook_url", config.URL,
	)

	if h.encKey == nil {
		slog.Error("nango webhook: encryption key not configured, cannot decrypt signing secret",
			"org_id", wctx.orgID,
		)
		return 0, nil, false
	}
	secret, err := h.encKey.DecryptString(config.EncryptedSecret)
	if err != nil {
		slog.Error("nango webhook: failed to decrypt webhook secret",
			"org_id", wctx.orgID,
			"error", err,
		)
		return http.StatusBadGateway, nil, true
	}

	provider := wh.Provider
	payload := webhookPayload{
		Type:      wh.Type,
		Provider:  provider,
		Operation: wh.Operation,
		Success:   wh.Success,
		Payload:   wh.Payload,
		OrgID:     wctx.orgID.String(),
	}
	if wctx.integration != nil {
		payload.IntegrationID = wctx.integration.ID.String()
		payload.IntegrationName = wctx.integration.DisplayName
		if provider == "" {
			provider = wctx.integration.Provider
			payload.Provider = provider
		}
	}
	if wctx.connection != nil {
		payload.ConnectionID = wctx.connection.ID.String()
		if wctx.connection.IdentityID != nil {
			payload.IdentityID = wctx.connection.IdentityID.String()
		}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		slog.Error("nango webhook: failed to marshal enriched payload",
			"org_id", wctx.orgID,
			"error", err,
		)
		return http.StatusBadGateway, nil, true
	}

	slog.Info("nango webhook: forwarding to org",
		"org_id", wctx.orgID,
		"webhook_url", config.URL,
		"enriched_payload_size", len(body),
		"provider", provider,
		"type", wh.Type,
	)

	timestamp := time.Now().Unix()
	signature := signWebhookPayload(body, secret, timestamp)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, config.URL, bytes.NewReader(body))
	if err != nil {
		slog.Error("nango webhook: failed to create forward request",
			"org_id", wctx.orgID,
			"webhook_url", config.URL,
			"error", err,
		)
		return http.StatusBadGateway, nil, true
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-ZiraLoop-Signature", signature)
	req.Header.Set("X-ZiraLoop-Timestamp", fmt.Sprintf("%d", timestamp))

	resp, err := h.httpClient.Do(req)
	if err != nil {
		slog.Error("nango webhook: forward request failed",
			"org_id", wctx.orgID,
			"webhook_url", config.URL,
			"error", err,
		)
		return http.StatusBadGateway, nil, true
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)

	slog.Info("nango webhook: forward response received",
		"org_id", wctx.orgID,
		"webhook_url", config.URL,
		"response_status", resp.StatusCode,
		"response_size", len(respBytes),
	)

	if resp.StatusCode >= 500 {
		slog.Warn("nango webhook: org endpoint returned 5xx, returning 502 to trigger Nango retry",
			"org_id", wctx.orgID,
			"webhook_url", config.URL,
			"response_status", resp.StatusCode,
		)
		return http.StatusBadGateway, respBytes, true
	}

	return resp.StatusCode, respBytes, true
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

// signWebhookPayload signs a payload for forwarding to the org's endpoint.
// Format: HMAC-SHA256("{timestamp}.{body}", secret), hex-encoded.
func signWebhookPayload(body []byte, secret string, timestamp int64) string {
	message := fmt.Sprintf("%d.%s", timestamp, string(body))
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}
