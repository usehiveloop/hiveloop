package handler

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/crypto"
	"github.com/usehiveloop/hiveloop/internal/enqueue"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// NangoWebhookHandler receives webhook events forwarded by Nango.
type NangoWebhookHandler struct {
	db          *gorm.DB
	nangoSecret string
	encKey      *crypto.SymmetricKey
	httpClient  *http.Client
	enqueuer    enqueue.TaskEnqueuer
}

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

type webhookPayload struct {
	Type            string          `json:"type"`
	Provider        string          `json:"provider"`
	Operation       string          `json:"operation,omitempty"`
	Success         *bool           `json:"success,omitempty"`
	Payload         json.RawMessage `json:"payload,omitempty"`
	OrgID           string          `json:"org_id"`
	IntegrationID   string          `json:"integration_id,omitempty"`
	IntegrationName string          `json:"integration_name,omitempty"`
	ConnectionID    string          `json:"connection_id,omitempty"`
}

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

	dispatchWebhookEvent(h.enqueuer, &wh, wctx)

	h.acknowledge(w)
}

func (h *NangoWebhookHandler) acknowledge(w http.ResponseWriter) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

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
