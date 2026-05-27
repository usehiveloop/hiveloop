package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/gateway"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
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

type webhookContext struct {
	orgID      uuid.UUID
	connection *model.Connection
}

// Handle processes POST /internal/webhooks/nango.
func (h *NangoWebhookHandler) Handle(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "nango webhook: failed to read request body", "error", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read body"})
		return
	}

	signature := r.Header.Get("X-Nango-Hmac-Sha256")
	if signature == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing signature header"})
		return
	}
	if !verifyNangoSignature(body, h.nangoSecret, signature) {
		logging.FromContext(r.Context()).WarnContext(r.Context(), "nango webhook: invalid signature")
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid signature"})
		return
	}

	var wh nangoWebhook
	if err := json.Unmarshal(body, &wh); err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "nango webhook: failed to parse payload", "error", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid webhook payload"})
		return
	}

	wctx := h.identify(r.Context(), &wh)
	if wctx == nil {
		headers := make(map[string]string)
		for k, v := range r.Header {
			if len(v) > 0 {
				headers[k] = v[0]
			}
		}
		logging.FromContext(r.Context()).InfoContext(r.Context(), "nango_webhook_connection_not_found",
			"nango_connection_id", wh.ConnectionID,
			"provider_config_key", wh.ProviderConfigKey,
			"type", wh.Type,
			"from", wh.From,
			"payload", string(body),
			"headers", headers,
		)
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}

	if isSlackProvider(wctx.connection) && wh.Type == "forward" {
		employee, err := ensureHivyEmployee(r.Context(), h.db, wctx.connection.OrgID)
		if err != nil {
			logging.FromContext(r.Context()).ErrorContext(r.Context(), "slack_webhook_failed_to_ensure_employee",
				"org_id", wctx.connection.OrgID.String(),
				"error", err,
			)
			logging.CaptureWithFields(r.Context(), fmt.Errorf("slack webhook: ensure employee: %w", err), map[string]any{
				"org_id": wctx.connection.OrgID.String(),
			})
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load employee"})
			return
		}

		envelope := gateway.WebhookEnvelope{
			ConnectionID: wctx.connection.ID,
			OrgID:        wctx.connection.OrgID,
			EmployeeID:   employee.ID,
			Provider:     wctx.connection.Integration.Provider,
			Headers:      normalizedHeaders(r.Header),
			Body:         wh.Payload,
		}

		logging.FromContext(r.Context()).InfoContext(r.Context(), "slack_webhook_envelope_built",
			"connection_id", envelope.ConnectionID.String(),
			"org_id", envelope.OrgID.String(),
			"employee_id", envelope.EmployeeID.String(),
			"provider", envelope.Provider,
			"payload", string(envelope.Body),
		)

		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}

	if !isSlackProvider(wctx.connection) {
		logging.FromContext(r.Context()).InfoContext(r.Context(), "nango_webhook_skipped",
			"org_id", wctx.orgID.String(),
			"connection_id", wctx.connection.ID.String(),
			"provider", wctx.connection.Integration.Provider,
			"type", wh.Type,
		)
	}

	dispatchWebhookEvent(r.Context(), h.enqueuer, &wh, wctx)

	h.acknowledge(w)
}

func (h *NangoWebhookHandler) acknowledge(w http.ResponseWriter) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
