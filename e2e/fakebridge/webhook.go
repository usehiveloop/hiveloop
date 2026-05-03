package fakebridge

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"
)

// PostWebhook signs payload as HMAC-SHA256 over "{timestamp}.{payload}" with
// SignSecret — matches verifyWebhookSignature in
// internal/handler/bridge_webhooks.go.
func (s *Server) PostWebhook(t *testing.T, events []BridgeEvent) (int, []byte) {
	t.Helper()
	if s.WebhookURL == "" {
		t.Fatal("fakebridge: WebhookURL not configured")
	}
	if len(s.SignSecret) == 0 {
		t.Fatal("fakebridge: SignSecret not configured")
	}

	body, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("marshal events: %v", err)
	}

	timestamp := time.Now().Unix()
	message := fmt.Sprintf("%d.%s", timestamp, string(body))
	mac := hmac.New(sha256.New, s.SignSecret)
	mac.Write([]byte(message))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, s.WebhookURL, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build webhook request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Signature", signature)
	req.Header.Set("X-Webhook-Timestamp", fmt.Sprintf("%d", timestamp))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("webhook POST: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, respBody
}

func (s *Server) PostWebhookUnsigned(t *testing.T, events []BridgeEvent, wrongSig string) (int, []byte) {
	t.Helper()
	if s.WebhookURL == "" {
		t.Fatal("fakebridge: WebhookURL not configured")
	}
	body, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("marshal events: %v", err)
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, s.WebhookURL, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build webhook request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if wrongSig != "" {
		req.Header.Set("X-Webhook-Signature", wrongSig)
		req.Header.Set("X-Webhook-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("webhook POST: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, respBody
}
