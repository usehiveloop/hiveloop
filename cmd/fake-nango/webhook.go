package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

type webhookSender struct {
	secret string
	target string
	client *http.Client
}

func newWebhookSender(secret, target string) *webhookSender {
	return &webhookSender{
		secret: secret,
		target: target,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (s *webhookSender) fireForward(target string, body forwardWebhook, providerHeaders map[string]string) {
	dst := s.target
	if target != "" {
		dst = target
	}
	s.send(dst, body, providerHeaders)
}

func (s *webhookSender) fireAuth(body authWebhook) {
	s.send(s.target, body, nil)
}

// send signs the body with the dual-header scheme real Nango uses
// (X-Nango-Signature is sha256(secret + body); X-Nango-Hmac-Sha256 is the
// proper HMAC). The backend at nango_webhooks_identify.go:118 verifies the
// HMAC one — without these headers it returns 401.
func (s *webhookSender) send(dst string, body any, providerHeaders map[string]string) {
	raw, err := json.Marshal(body)
	if err != nil {
		slog.Error("webhook marshal failed", "error", err)
		return
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, dst, bytes.NewReader(raw))
	if err != nil {
		slog.Error("webhook request build failed", "error", err)
		return
	}

	for k, v := range providerHeaders {
		req.Header.Set(k, v)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Nango-Signature", legacySignature(s.secret, raw))
	req.Header.Set("X-Nango-Hmac-Sha256", hmacSignature(s.secret, raw))

	resp, err := s.client.Do(req)
	if err != nil {
		slog.Error("webhook send failed", "error", err, "target", dst)
		return
	}
	_ = resp.Body.Close()
	slog.Info("webhook delivered", "target", dst, "status", resp.StatusCode)
}

func legacySignature(secret string, body []byte) string {
	h := sha256.New()
	h.Write([]byte(secret))
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}

func hmacSignature(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

