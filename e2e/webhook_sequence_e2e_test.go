// Wave 3 e2e: webhook sequence + Redis Stream ingest verification.
//
// Required infra:
//   DATABASE_URL  → Postgres reachable
//   REDIS_ADDR    → Redis reachable
package e2e

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/e2e/fakebridge"
	"github.com/usehiveloop/hiveloop/internal/crypto"
	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/streaming"
)

// TestWebhookSequence_NoGaps fires a batch of 30 strictly-increasing
// events through the webhook handler, then reads the Redis Stream back
// and asserts: no gaps, monotonic sequence, every event delivered.
func TestWebhookSequence_NoGaps(t *testing.T) {
	h := newHarness(t)
	suffix := uuid.New().String()[:8]

	keyBytes := make([]byte, 32)
	for i := range keyBytes {
		keyBytes[i] = byte(i + 53)
	}
	encKey, err := crypto.NewSymmetricKey(base64.StdEncoding.EncodeToString(keyBytes))
	if err != nil {
		t.Fatalf("symmetric key: %v", err)
	}
	bridgeSecret := "wh-secret-" + suffix
	encryptedKey, err := encKey.EncryptString(bridgeSecret)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	org := model.Org{Name: "wh-org-" + suffix}
	h.db.Create(&org)
	t.Cleanup(func() { h.db.Where("id = ?", org.ID).Delete(&model.Org{}) })

	cred := model.Credential{
		OrgID: org.ID, BaseURL: "https://api.openai.com", AuthScheme: "bearer",
		ProviderID: "openai", EncryptedKey: []byte("enc"), WrappedDEK: []byte("dek"),
	}
	h.db.Create(&cred)
	t.Cleanup(func() { h.db.Where("id = ?", cred.ID).Delete(&model.Credential{}) })

	agent := model.Agent{
		OrgID: &org.ID, Name: "wh-agent-" + suffix,
		CredentialID: &cred.ID, SystemPrompt: "test", Model: "gpt-4o",
	}
	h.db.Create(&agent)
	t.Cleanup(func() { h.db.Where("id = ?", agent.ID).Delete(&model.Agent{}) })

	sb := model.Sandbox{
		OrgID: &org.ID, AgentID: &agent.ID,
		ExternalID: "wh-ext-" + suffix, BridgeURL: "https://test:25434",
		EncryptedBridgeAPIKey: encryptedKey, Status: "running",
	}
	h.db.Create(&sb)
	t.Cleanup(func() { h.db.Where("id = ?", sb.ID).Delete(&model.Sandbox{}) })

	conv := model.AgentConversation{
		OrgID: org.ID, AgentID: agent.ID, SandboxID: sb.ID,
		BridgeConversationID: "wh-conv-" + suffix, Status: "active",
	}
	h.db.Create(&conv)
	t.Cleanup(func() {
		h.db.Where("conversation_id = ?", conv.ID).Delete(&model.ConversationEvent{})
		h.db.Where("id = ?", conv.ID).Delete(&model.AgentConversation{})
	})

	eventBus := streaming.NewEventBus(h.redisClient)
	webhookHandler := handler.NewBridgeWebhookHandler(h.db, encKey, eventBus, nil)

	r := chi.NewRouter()
	r.Post("/internal/webhooks/bridge/{sandboxID}", webhookHandler.Handle)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	fb := fakebridge.New(t)
	fb.SignSecret = []byte(bridgeSecret)
	fb.WebhookURL = srv.URL + "/internal/webhooks/bridge/" + sb.ID.String()

	// Build 30 events with strictly increasing sequence numbers, mixing
	// chunk and non-chunk types so we exercise both Redis publish and
	// (no-op) Postgres skip behavior for chunks.
	events := make([]fakebridge.BridgeEvent, 0, 30)
	now := time.Now()
	cycle := []string{"response_chunk", "tool_call_start", "tool_call_result", "response_chunk"}
	for i := 1; i <= 30; i++ {
		etype := cycle[(i-1)%len(cycle)]
		if i == 30 {
			etype = "turn_completed"
		}
		events = append(events, fakebridge.BridgeEvent{
			EventID:        fmt.Sprintf("ev-%d", i),
			EventType:      etype,
			AgentID:        agent.ID.String(),
			ConversationID: conv.BridgeConversationID,
			Timestamp:      now.Add(time.Duration(i) * time.Millisecond),
			SequenceNumber: int64(i),
			Data:           json.RawMessage(fmt.Sprintf(`{"i":%d}`, i)),
		})
	}

	if status, body := fb.PostWebhook(t, events); status != http.StatusOK {
		t.Fatalf("webhook status=%d body=%s", status, body)
	}

	// Read back from Redis Stream.
	streamKey := eventBus.Prefix() + conv.ID.String()
	t.Cleanup(func() {
		_ = eventBus.Redis().Del(context.Background(), streamKey).Err()
	})
	entries, err := eventBus.Redis().XRange(context.Background(), streamKey, "-", "+").Result()
	if err != nil {
		t.Fatalf("XRANGE: %v", err)
	}
	if len(entries) != 30 {
		t.Fatalf("XRANGE returned %d entries, want 30", len(entries))
	}
	prevSeq := int64(0)
	for i, e := range entries {
		dataStr, _ := e.Values["data"].(string)
		var env map[string]any
		_ = json.Unmarshal([]byte(dataStr), &env)
		seqAny, ok := env["sequence_number"]
		if !ok {
			t.Fatalf("entry[%d] missing sequence_number: %v", i, env)
		}
		seq := int64(seqAny.(float64))
		if seq != prevSeq+1 {
			t.Errorf("sequence gap: prev=%d, got=%d at index %d", prevSeq, seq, i)
		}
		prevSeq = seq
	}
}

// TestWebhookSequence_BadSignatureRejected re-affirms that the existing
// signature-verification gate still rejects mis-signed batches and that
// nothing is persisted to either Postgres or Redis.
func TestWebhookSequence_BadSignatureRejected(t *testing.T) {
	h := newHarness(t)
	suffix := uuid.New().String()[:8]

	keyBytes := make([]byte, 32)
	for i := range keyBytes {
		keyBytes[i] = byte(i + 67)
	}
	encKey, err := crypto.NewSymmetricKey(base64.StdEncoding.EncodeToString(keyBytes))
	if err != nil {
		t.Fatalf("symmetric key: %v", err)
	}
	encryptedKey, _ := encKey.EncryptString("real-secret-" + suffix)

	org := model.Org{Name: "whbs-org-" + suffix}
	h.db.Create(&org)
	t.Cleanup(func() { h.db.Where("id = ?", org.ID).Delete(&model.Org{}) })
	cred := model.Credential{
		OrgID: org.ID, BaseURL: "x", AuthScheme: "bearer",
		ProviderID: "openai", EncryptedKey: []byte("enc"), WrappedDEK: []byte("dek"),
	}
	h.db.Create(&cred)
	t.Cleanup(func() { h.db.Where("id = ?", cred.ID).Delete(&model.Credential{}) })
	agent := model.Agent{
		OrgID: &org.ID, Name: "whbs-agent-" + suffix,
		CredentialID: &cred.ID, SystemPrompt: "x", Model: "gpt-4o",
	}
	h.db.Create(&agent)
	t.Cleanup(func() { h.db.Where("id = ?", agent.ID).Delete(&model.Agent{}) })
	sb := model.Sandbox{
		OrgID: &org.ID, AgentID: &agent.ID,
		ExternalID:            "whbs-ext-" + suffix,
		BridgeURL:             "https://x:1",
		EncryptedBridgeAPIKey: encryptedKey,
		Status:                "running",
	}
	h.db.Create(&sb)
	t.Cleanup(func() { h.db.Where("id = ?", sb.ID).Delete(&model.Sandbox{}) })
	conv := model.AgentConversation{
		OrgID: org.ID, AgentID: agent.ID, SandboxID: sb.ID,
		BridgeConversationID: "whbs-conv-" + suffix, Status: "active",
	}
	h.db.Create(&conv)
	t.Cleanup(func() {
		h.db.Where("conversation_id = ?", conv.ID).Delete(&model.ConversationEvent{})
		h.db.Where("id = ?", conv.ID).Delete(&model.AgentConversation{})
	})

	eventBus := streaming.NewEventBus(h.redisClient)
	webhookHandler := handler.NewBridgeWebhookHandler(h.db, encKey, eventBus, nil)
	r := chi.NewRouter()
	r.Post("/internal/webhooks/bridge/{sandboxID}", webhookHandler.Handle)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	fb := fakebridge.New(t)
	fb.SignSecret = []byte("WRONG-SECRET")
	fb.WebhookURL = srv.URL + "/internal/webhooks/bridge/" + sb.ID.String()

	events := []fakebridge.BridgeEvent{
		{
			EventID: "ev1", EventType: "message_received",
			AgentID: agent.ID.String(), ConversationID: conv.BridgeConversationID,
			Timestamp: time.Now(), SequenceNumber: 1, Data: json.RawMessage(`{}`),
		},
	}

	status, body := fb.PostWebhook(t, events)
	if status != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d body=%s", status, body)
	}

	// Postgres should not have persisted any event.
	var count int64
	h.db.Model(&model.ConversationEvent{}).Where("conversation_id = ?", conv.ID).Count(&count)
	if count != 0 {
		t.Errorf("postgres events: got %d, want 0", count)
	}

	// Redis stream should be empty.
	streamKey := eventBus.Prefix() + conv.ID.String()
	t.Cleanup(func() {
		_ = eventBus.Redis().Del(context.Background(), streamKey).Err()
	})
	entries, err := eventBus.Redis().XRange(context.Background(), streamKey, "-", "+").Result()
	if err != nil {
		t.Fatalf("XRANGE: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("redis stream: got %d entries, want 0", len(entries))
	}

	// Also test the missing-signature case via PostWebhookUnsigned.
	status2, _ := fb.PostWebhookUnsigned(t, events, "")
	if status2 != http.StatusUnauthorized {
		t.Errorf("missing-sig status=%d, want 401", status2)
	}
}

// TestWebhookSequence_StatusTransitions confirms that ConversationEnded
// flips the AgentConversation row to "ended" and AgentError flips it to
// "error" (mirrors the existing webhook tests but exercises the path
// from a real fakebridge POST flow).
func TestWebhookSequence_StatusTransitions(t *testing.T) {
	h := newHarness(t)
	suffix := uuid.New().String()[:8]

	keyBytes := make([]byte, 32)
	for i := range keyBytes {
		keyBytes[i] = byte(i + 71)
	}
	encKey, _ := crypto.NewSymmetricKey(base64.StdEncoding.EncodeToString(keyBytes))
	bridgeSecret := "wh3-secret-" + suffix
	encryptedKey, _ := encKey.EncryptString(bridgeSecret)

	org := model.Org{Name: "wh3-org-" + suffix}
	h.db.Create(&org)
	t.Cleanup(func() { h.db.Where("id = ?", org.ID).Delete(&model.Org{}) })
	cred := model.Credential{
		OrgID: org.ID, BaseURL: "x", AuthScheme: "bearer",
		ProviderID: "openai", EncryptedKey: []byte("enc"), WrappedDEK: []byte("dek"),
	}
	h.db.Create(&cred)
	t.Cleanup(func() { h.db.Where("id = ?", cred.ID).Delete(&model.Credential{}) })
	agent := model.Agent{
		OrgID: &org.ID, Name: "wh3-agent-" + suffix,
		CredentialID: &cred.ID, SystemPrompt: "x", Model: "gpt-4o",
	}
	h.db.Create(&agent)
	t.Cleanup(func() { h.db.Where("id = ?", agent.ID).Delete(&model.Agent{}) })

	mkSB := func() (model.Sandbox, model.AgentConversation) {
		sb := model.Sandbox{
			OrgID: &org.ID, AgentID: &agent.ID,
			ExternalID: "wh3-ext-" + uuid.New().String()[:6],
			BridgeURL:  "x", EncryptedBridgeAPIKey: encryptedKey, Status: "running",
		}
		h.db.Create(&sb)
		t.Cleanup(func() { h.db.Where("id = ?", sb.ID).Delete(&model.Sandbox{}) })
		conv := model.AgentConversation{
			OrgID: org.ID, AgentID: agent.ID, SandboxID: sb.ID,
			BridgeConversationID: "wh3-conv-" + uuid.New().String()[:6], Status: "active",
		}
		h.db.Create(&conv)
		t.Cleanup(func() {
			h.db.Where("conversation_id = ?", conv.ID).Delete(&model.ConversationEvent{})
			h.db.Where("id = ?", conv.ID).Delete(&model.AgentConversation{})
		})
		return sb, conv
	}

	eventBus := streaming.NewEventBus(h.redisClient)
	webhookHandler := handler.NewBridgeWebhookHandler(h.db, encKey, eventBus, nil)
	r := chi.NewRouter()
	r.Post("/internal/webhooks/bridge/{sandboxID}", webhookHandler.Handle)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	fb := fakebridge.New(t)
	fb.SignSecret = []byte(bridgeSecret)

	// ConversationEnded → status=ended, ended_at set.
	sb1, conv1 := mkSB()
	fb.WebhookURL = srv.URL + "/internal/webhooks/bridge/" + sb1.ID.String()
	endEvents := []fakebridge.BridgeEvent{
		{
			EventID: "ev-end", EventType: "ConversationEnded",
			AgentID: agent.ID.String(), ConversationID: conv1.BridgeConversationID,
			Timestamp: time.Now(), SequenceNumber: 1, Data: json.RawMessage(`{}`),
		},
	}
	if status, _ := fb.PostWebhook(t, endEvents); status != http.StatusOK {
		t.Fatalf("end webhook status=%d", status)
	}
	var c1 model.AgentConversation
	h.db.Where("id = ?", conv1.ID).First(&c1)
	if c1.Status != "ended" {
		t.Errorf("status: got %q, want ended", c1.Status)
	}
	if c1.EndedAt == nil {
		t.Error("ended_at should be set")
	}
	t.Cleanup(func() { _ = eventBus.Redis().Del(context.Background(), eventBus.Prefix()+conv1.ID.String()).Err() })

	// AgentError → status=error.
	sb2, conv2 := mkSB()
	fb.WebhookURL = srv.URL + "/internal/webhooks/bridge/" + sb2.ID.String()
	errEvents := []fakebridge.BridgeEvent{
		{
			EventID: "ev-err", EventType: "AgentError",
			AgentID: agent.ID.String(), ConversationID: conv2.BridgeConversationID,
			Timestamp: time.Now(), SequenceNumber: 1, Data: json.RawMessage(`{"error":"boom"}`),
		},
	}
	if status, _ := fb.PostWebhook(t, errEvents); status != http.StatusOK {
		t.Fatalf("err webhook status=%d", status)
	}
	var c2 model.AgentConversation
	h.db.Where("id = ?", conv2.ID).First(&c2)
	if c2.Status != "error" {
		t.Errorf("status: got %q, want error", c2.Status)
	}
	t.Cleanup(func() { _ = eventBus.Redis().Del(context.Background(), eventBus.Prefix()+conv2.ID.String()).Err() })

	_ = strings.Builder{} // keep import alive
}
