package tasks

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
	"time"

	"github.com/hibiken/asynq"

	"github.com/usehiveloop/hiveloop/internal/crypto"
)

// WebhookForwardHandler delivers enriched webhook payloads to org endpoints.
type WebhookForwardHandler struct {
	encKey     *crypto.SymmetricKey
	httpClient *http.Client
}

// NewWebhookForwardHandler creates a webhook forward handler.
func NewWebhookForwardHandler(encKey *crypto.SymmetricKey) *WebhookForwardHandler {
	return &WebhookForwardHandler{
		encKey:     encKey,
		httpClient: &http.Client{Timeout: 25 * time.Second},
	}
}

// Handle processes a webhook:forward task.
func (h *WebhookForwardHandler) Handle(ctx context.Context, t *asynq.Task) error {
	var p WebhookForwardPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("unmarshal webhook payload: %w", err)
	}

	if h.encKey == nil {
		return fmt.Errorf("encryption key not configured")
	}

	secret, err := h.encKey.DecryptString(p.EncryptedSecret)
	if err != nil {
		return fmt.Errorf("decrypt webhook secret: %w", err)
	}

	timestamp := time.Now().Unix()
	signature := signPayload(p.Body, secret, timestamp)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.WebhookURL, bytes.NewReader(p.Body))
	if err != nil {
		return fmt.Errorf("create forward request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-HiveLoop-Signature", signature)
	req.Header.Set("X-HiveLoop-Timestamp", fmt.Sprintf("%d", timestamp))

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("forward request failed: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 500 {
		// Returning an error causes Asynq to retry with exponential backoff.
		return fmt.Errorf("org endpoint returned %d", resp.StatusCode)
	}

	slog.Info("webhook forwarded",
		"url", p.WebhookURL,
		"status", resp.StatusCode,
	)
	return nil
}

// signPayload signs a payload for forwarding to the org's endpoint.
// Format: HMAC-SHA256("{timestamp}.{body}", secret), hex-encoded.
func signPayload(body []byte, secret string, timestamp int64) string {
	message := fmt.Sprintf("%d.%s", timestamp, string(body))
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}
