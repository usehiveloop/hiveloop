package handler

import (
	"bytes"
	"compress/gzip"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/model"
)

func TestEmployeeOutboundWebhookBatch_IngestsCoalescedStreamWithoutPerDeltaRows(t *testing.T) {
	db := connectEmployeeSkillSyncTestDB(t)
	encKey := outboundWebhookTestSymmetricKey(t)
	org := model.Org{Name: "batch-webhook-" + uuid.NewString(), RateLimit: 1000, Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", org.ID).Delete(&model.Org{}) })
	agent := model.Agent{OrgID: &org.ID, Name: "Higu", Model: "test", IsEmployee: true}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	bridgeKey := "batch-webhook-secret-" + uuid.NewString()
	encryptedKey, err := encKey.EncryptString(bridgeKey)
	if err != nil {
		t.Fatalf("encrypt bridge key: %v", err)
	}
	sandbox := model.Sandbox{
		OrgID:                 &org.ID,
		AgentID:               &agent.ID,
		ExternalID:            "batch-webhook-sandbox",
		BridgeURL:             "http://localhost:7080",
		EncryptedBridgeAPIKey: encryptedKey,
		Status:                "running",
	}
	if err := db.Create(&sandbox).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	h := NewEmployeeOutboundWebhookHandler(db, encKey, nil)
	r := chi.NewRouter()
	r.Post("/internal/webhooks/employee/{sandboxID}/batch", h.HandleBatch)

	streamPayload := map[string]any{
		"session_id":     "C123-1779137680.936289",
		"source":         "slack",
		"coalesced":      true,
		"delta_count":    1500,
		"sequence_start": 1,
		"sequence_end":   1500,
		"agent_event": map[string]any{
			"kind": "thinking_chunk",
			"text": "the full thinking stream is preserved as one business event",
		},
	}
	finalPayload := map[string]any{
		"session_id": "C123-1779137680.936289",
		"source":     "slack",
		"agent_event": map[string]any{
			"kind": "final_message",
			"text": "Done.",
		},
	}
	body := gzipNDJSON(t,
		employeeOutboundEvent{EventType: "agent.stream.thinking", Payload: mustJSON(t, streamPayload), At: time.Now().UTC()},
		employeeOutboundEvent{EventType: "agent.final_message", Payload: mustJSON(t, finalPayload), At: time.Now().UTC()},
	)
	req := httptest.NewRequest(http.MethodPost, "/internal/webhooks/employee/"+sandbox.ID.String()+"/batch", bytes.NewReader(body))
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("Content-Type", "application/x-ndjson")
	req.Header.Set("X-Hivy-Signature", "sha256="+hmacHex(bridgeKey, body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var count int64
	db.Model(&model.EmployeeMemoryEvent{}).
		Where("sandbox_id = ? AND session_id = ?", sandbox.ID, "C123-1779137680.936289").
		Count(&count)
	if count != 2 {
		t.Fatalf("stored event count = %d, want 2", count)
	}
	var stored model.EmployeeMemoryEvent
	if err := db.Where("sandbox_id = ? AND event_type = ?", sandbox.ID, "agent.stream.thinking").First(&stored).Error; err != nil {
		t.Fatalf("load coalesced stream event: %v", err)
	}
	var storedPayload map[string]any
	if err := json.Unmarshal(stored.Payload, &storedPayload); err != nil {
		t.Fatalf("decode stored payload: %v", err)
	}
	if storedPayload["coalesced"] != true || int(storedPayload["delta_count"].(float64)) != 1500 {
		t.Fatalf("stream payload was not preserved as one coalesced event: %#v", storedPayload)
	}
}

func TestEmployeeOutboundWebhookBatch_RejectsBadSignature(t *testing.T) {
	db := connectEmployeeSkillSyncTestDB(t)
	encKey := outboundWebhookTestSymmetricKey(t)
	org := model.Org{Name: "batch-webhook-" + uuid.NewString(), RateLimit: 1000, Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", org.ID).Delete(&model.Org{}) })
	agent := model.Agent{OrgID: &org.ID, Name: "Higu", Model: "test", IsEmployee: true}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	encryptedKey, err := encKey.EncryptString("right-secret")
	if err != nil {
		t.Fatalf("encrypt bridge key: %v", err)
	}
	sandbox := model.Sandbox{OrgID: &org.ID, AgentID: &agent.ID, EncryptedBridgeAPIKey: encryptedKey, Status: "running"}
	if err := db.Create(&sandbox).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	h := NewEmployeeOutboundWebhookHandler(db, encKey, nil)
	r := chi.NewRouter()
	r.Post("/internal/webhooks/employee/{sandboxID}/batch", h.HandleBatch)
	body := gzipNDJSON(t, employeeOutboundEvent{EventType: "agent.stream.token", Payload: mustJSON(t, map[string]any{"session_id": "s"}), At: time.Now().UTC()})
	req := httptest.NewRequest(http.MethodPost, "/internal/webhooks/employee/"+sandbox.ID.String()+"/batch", bytes.NewReader(body))
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("X-Hivy-Signature", "sha256="+hmacHex("wrong-secret", body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rr.Code, rr.Body.String())
	}
}

func outboundWebhookTestSymmetricKey(t *testing.T) *crypto.SymmetricKey {
	t.Helper()
	key := make([]byte, 32)
	for idx := range key {
		key[idx] = byte(idx + 17)
	}
	encKey, err := crypto.NewSymmetricKey(base64.StdEncoding.EncodeToString(key))
	if err != nil {
		t.Fatalf("create symmetric key: %v", err)
	}
	return encKey
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	return raw
}

func gzipNDJSON(t *testing.T, events ...employeeOutboundEvent) []byte {
	t.Helper()
	var raw bytes.Buffer
	for _, event := range events {
		if err := json.NewEncoder(&raw).Encode(event); err != nil {
			t.Fatalf("encode ndjson: %v", err)
		}
	}
	var compressed bytes.Buffer
	gz := gzip.NewWriter(&compressed)
	if _, err := gz.Write(raw.Bytes()); err != nil {
		t.Fatalf("gzip batch: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return compressed.Bytes()
}

func hmacHex(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}
