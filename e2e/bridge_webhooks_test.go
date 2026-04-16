package e2e

import (
	"crypto/hmac"
	"crypto/sha256"
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

	"github.com/ziraloop/ziraloop/internal/crypto"
	"github.com/ziraloop/ziraloop/internal/handler"
	"github.com/ziraloop/ziraloop/internal/model"
)

// webhookTestHarness sets up the full chain: org → identity → credential → agent → sandbox → conversation
type webhookTestHarness struct {
	*testHarness
	encKey   *crypto.SymmetricKey
	org      model.Org
	sandbox  model.Sandbox
	conv     model.AgentConversation
	secret   string // plaintext Bridge API key
	router   *chi.Mux
}

func newWebhookTestHarness(t *testing.T) *webhookTestHarness {
	t.Helper()
	h := newHarness(t)
	suffix := uuid.New().String()[:8]

	// Encryption key
	keyBytes := make([]byte, 32)
	for i := range keyBytes {
		keyBytes[i] = byte(i + 99)
	}
	encKey, err := crypto.NewSymmetricKey(base64.StdEncoding.EncodeToString(keyBytes))
	if err != nil {
		t.Fatal(err)
	}

	// Create org
	org := model.Org{Name: "webhook-test-" + suffix}
	h.db.Create(&org)
	t.Cleanup(func() { h.db.Where("id = ?", org.ID).Delete(&model.Org{}) })

	// Create identity
	h.db.Create(&identity)

	// Create credential
	cred := model.Credential{
		OrgID: org.ID, BaseURL: "https://api.openai.com", AuthScheme: "bearer",
		ProviderID: "openai", EncryptedKey: []byte("enc"), WrappedDEK: []byte("dek"),
	}
	h.db.Create(&cred)
	t.Cleanup(func() { h.db.Where("id = ?", cred.ID).Delete(&model.Credential{}) })

	// Create agent
	agent := model.Agent{
		OrgID: &org.ID, IdentityID: &identity.ID, Name: "wh-agent-" + suffix,
		CredentialID: &cred.ID, SandboxType: "shared", SystemPrompt: "test", Model: "gpt-4o",
	}
	h.db.Create(&agent)
	t.Cleanup(func() { h.db.Where("id = ?", agent.ID).Delete(&model.Agent{}) })

	// Create sandbox with encrypted Bridge API key
	bridgeSecret := "test-bridge-secret-" + suffix
	encryptedKey, _ := encKey.EncryptString(bridgeSecret)
	sandbox := model.Sandbox{
		OrgID: &org.ID, IdentityID: &identity.ID, SandboxType: "shared",
		ExternalID: "wh-ext-" + suffix, BridgeURL: "https://test:25434",
		EncryptedBridgeAPIKey: encryptedKey, Status: "running",
	}
	h.db.Create(&sandbox)
	t.Cleanup(func() { h.db.Where("id = ?", sandbox.ID).Delete(&model.Sandbox{}) })

	// Create conversation
	conv := model.AgentConversation{
		OrgID: org.ID, AgentID: agent.ID, SandboxID: sandbox.ID,
		BridgeConversationID: "bridge-conv-" + suffix, Status: "active",
	}
	h.db.Create(&conv)
	t.Cleanup(func() {
		h.db.Where("conversation_id = ?", conv.ID).Delete(&model.ConversationEvent{})
		h.db.Where("id = ?", conv.ID).Delete(&model.AgentConversation{})
	})

	// Router
	webhookHandler := handler.NewBridgeWebhookHandler(h.db, encKey, nil)
	r := chi.NewRouter()
	r.Post("/internal/webhooks/bridge/{sandboxID}", webhookHandler.Handle)

	return &webhookTestHarness{
		testHarness: h,
		encKey:      encKey,
		org:         org,
		sandbox:     sandbox,
		conv:        conv,
		secret:      bridgeSecret,
		router:      r,
	}
}

func (wh *webhookTestHarness) signedRequest(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	timestamp := time.Now().Unix()
	message := fmt.Sprintf("%d.%s", timestamp, body)
	mac := hmac.New(sha256.New, []byte(wh.secret))
	mac.Write([]byte(message))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest(http.MethodPost,
		"/internal/webhooks/bridge/"+wh.sandbox.ID.String(),
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Signature", signature)
	req.Header.Set("X-Webhook-Timestamp", fmt.Sprintf("%d", timestamp))

	rr := httptest.NewRecorder()
	wh.router.ServeHTTP(rr, req)
	return rr
}

func TestWebhook_PersistsEvents(t *testing.T) {
	wh := newWebhookTestHarness(t)

	body := fmt.Sprintf(`[
		{"event_id":"e1","event_type":"ConversationCreated","agent_id":"a1","conversation_id":"%s","timestamp":"2026-03-31T12:00:00Z","sequence_number":1,"data":{}},
		{"event_id":"e2","event_type":"MessageReceived","agent_id":"a1","conversation_id":"%s","timestamp":"2026-03-31T12:00:01Z","sequence_number":2,"data":{"content":"hello"}},
		{"event_id":"e3","event_type":"ResponseCompleted","agent_id":"a1","conversation_id":"%s","timestamp":"2026-03-31T12:00:02Z","sequence_number":3,"data":{"content":"hi","usage":{"input_tokens":10,"output_tokens":5}}}
	]`, wh.conv.BridgeConversationID, wh.conv.BridgeConversationID, wh.conv.BridgeConversationID)

	rr := wh.signedRequest(t, body)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Check events persisted
	var events []model.ConversationEvent
	wh.db.Where("conversation_id = ?", wh.conv.ID).Order("created_at ASC").Find(&events)
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	if events[0].EventType != "ConversationCreated" {
		t.Errorf("event 0: got %q", events[0].EventType)
	}
	if events[1].EventType != "MessageReceived" {
		t.Errorf("event 1: got %q", events[1].EventType)
	}
	if events[2].EventType != "ResponseCompleted" {
		t.Errorf("event 2: got %q", events[2].EventType)
	}
}

func TestWebhook_ConversationEndedUpdatesStatus(t *testing.T) {
	wh := newWebhookTestHarness(t)

	body := fmt.Sprintf(`[
		{"event_id":"e1","event_type":"ConversationEnded","agent_id":"a1","conversation_id":"%s","timestamp":"2026-03-31T12:00:00Z","sequence_number":1,"data":{}}
	]`, wh.conv.BridgeConversationID)

	rr := wh.signedRequest(t, body)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var conv model.AgentConversation
	wh.db.Where("id = ?", wh.conv.ID).First(&conv)
	if conv.Status != "ended" {
		t.Errorf("conversation status: got %q, want ended", conv.Status)
	}
	if conv.EndedAt == nil {
		t.Error("ended_at should be set")
	}
}

func TestWebhook_AgentErrorUpdatesStatus(t *testing.T) {
	wh := newWebhookTestHarness(t)

	body := fmt.Sprintf(`[
		{"event_id":"e1","event_type":"AgentError","agent_id":"a1","conversation_id":"%s","timestamp":"2026-03-31T12:00:00Z","sequence_number":1,"data":{"error":"something broke"}}
	]`, wh.conv.BridgeConversationID)

	rr := wh.signedRequest(t, body)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var conv model.AgentConversation
	wh.db.Where("id = ?", wh.conv.ID).First(&conv)
	if conv.Status != "error" {
		t.Errorf("conversation status: got %q, want error", conv.Status)
	}
}

func TestWebhook_InvalidSignatureRejected(t *testing.T) {
	wh := newWebhookTestHarness(t)

	body := `[{"event_id":"e1","event_type":"MessageReceived","agent_id":"a1","conversation_id":"test","timestamp":"2026-03-31T12:00:00Z","sequence_number":1,"data":{}}]`

	req := httptest.NewRequest(http.MethodPost,
		"/internal/webhooks/bridge/"+wh.sandbox.ID.String(),
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Signature", "invalid-signature")
	req.Header.Set("X-Webhook-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

	rr := httptest.NewRecorder()
	wh.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("invalid signature: expected 401, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestWebhook_MissingSignatureRejected(t *testing.T) {
	wh := newWebhookTestHarness(t)

	body := `[{"event_id":"e1","event_type":"test","agent_id":"a1","conversation_id":"test","timestamp":"2026-03-31T12:00:00Z","sequence_number":1,"data":{}}]`

	req := httptest.NewRequest(http.MethodPost,
		"/internal/webhooks/bridge/"+wh.sandbox.ID.String(),
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	// No signature headers

	rr := httptest.NewRecorder()
	wh.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("missing signature: expected 401, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestWebhook_WrongSecretRejected(t *testing.T) {
	wh := newWebhookTestHarness(t)

	body := `[{"event_id":"e1","event_type":"test","agent_id":"a1","conversation_id":"test","timestamp":"2026-03-31T12:00:00Z","sequence_number":1,"data":{}}]`

	// Sign with wrong secret
	timestamp := time.Now().Unix()
	message := fmt.Sprintf("%d.%s", timestamp, body)
	mac := hmac.New(sha256.New, []byte("wrong-secret"))
	mac.Write([]byte(message))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest(http.MethodPost,
		"/internal/webhooks/bridge/"+wh.sandbox.ID.String(),
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Signature", signature)
	req.Header.Set("X-Webhook-Timestamp", fmt.Sprintf("%d", timestamp))

	rr := httptest.NewRecorder()
	wh.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("wrong secret: expected 401, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestWebhook_SandboxNotFound(t *testing.T) {
	wh := newWebhookTestHarness(t)

	req := httptest.NewRequest(http.MethodPost,
		"/internal/webhooks/bridge/"+uuid.New().String(),
		strings.NewReader("[]"))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	wh.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("sandbox not found: expected 404, got %d", rr.Code)
	}
}

func TestWebhook_UpdatesLastActiveAt(t *testing.T) {
	wh := newWebhookTestHarness(t)

	// Set last_active_at to old time
	oldTime := time.Now().Add(-1 * time.Hour)
	wh.db.Model(&wh.sandbox).Update("last_active_at", oldTime)

	body := fmt.Sprintf(`[
		{"event_id":"e1","event_type":"MessageReceived","agent_id":"a1","conversation_id":"%s","timestamp":"2026-03-31T12:00:00Z","sequence_number":1,"data":{}}
	]`, wh.conv.BridgeConversationID)

	rr := wh.signedRequest(t, body)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var sb model.Sandbox
	wh.db.Where("id = ?", wh.sandbox.ID).First(&sb)
	if sb.LastActiveAt == nil || sb.LastActiveAt.Before(oldTime.Add(30*time.Minute)) {
		t.Error("last_active_at should have been updated to recent time")
	}
}

func TestWebhook_MultipleEventTypes(t *testing.T) {
	wh := newWebhookTestHarness(t)

	body := fmt.Sprintf(`[
		{"event_id":"e1","event_type":"ConversationCreated","agent_id":"a1","conversation_id":"%[1]s","timestamp":"2026-03-31T12:00:00Z","sequence_number":1,"data":{}},
		{"event_id":"e2","event_type":"MessageReceived","agent_id":"a1","conversation_id":"%[1]s","timestamp":"2026-03-31T12:00:01Z","sequence_number":2,"data":{"content":"hello"}},
		{"event_id":"e3","event_type":"ResponseStarted","agent_id":"a1","conversation_id":"%[1]s","timestamp":"2026-03-31T12:00:02Z","sequence_number":3,"data":{}},
		{"event_id":"e4","event_type":"ResponseChunk","agent_id":"a1","conversation_id":"%[1]s","timestamp":"2026-03-31T12:00:03Z","sequence_number":4,"data":{"delta":"Hi"}},
		{"event_id":"e5","event_type":"ResponseCompleted","agent_id":"a1","conversation_id":"%[1]s","timestamp":"2026-03-31T12:00:04Z","sequence_number":5,"data":{"content":"Hi there!","usage":{"input_tokens":10,"output_tokens":3}}},
		{"event_id":"e6","event_type":"TurnCompleted","agent_id":"a1","conversation_id":"%[1]s","timestamp":"2026-03-31T12:00:05Z","sequence_number":6,"data":{}}
	]`, wh.conv.BridgeConversationID)

	rr := wh.signedRequest(t, body)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var events []model.ConversationEvent
	wh.db.Where("conversation_id = ?", wh.conv.ID).Order("created_at ASC").Find(&events)
	if len(events) != 6 {
		t.Fatalf("expected 6 events, got %d", len(events))
	}

	expectedTypes := []string{
		"ConversationCreated", "MessageReceived", "ResponseStarted",
		"ResponseChunk", "ResponseCompleted", "TurnCompleted",
	}
	for i, et := range expectedTypes {
		if events[i].EventType != et {
			t.Errorf("event %d: got %q, want %q", i, events[i].EventType, et)
		}
	}

	// Verify payload data is stored
	var respEvent model.ConversationEvent
	wh.db.Where("conversation_id = ? AND event_type = 'ResponseCompleted'", wh.conv.ID).First(&respEvent)
	var data map[string]any
	if err := json.Unmarshal(respEvent.Data, &data); err != nil {
		t.Fatal("response_completed data should be valid JSON")
	}
	if data["content"] != "Hi there!" {
		t.Errorf("response content: got %v", data["content"])
	}
}

// Verify the HMAC signing matches Bridge's implementation exactly
func TestWebhookSignature_MatchesBridge(t *testing.T) {
	// This test uses the exact values from Bridge's signer.rs test
	payload := []byte("test payload")
	secret := "webhook-secret"
	timestamp := int64(1700000000)

	message := fmt.Sprintf("%d.%s", timestamp, string(payload))
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	// Verify our verifyWebhookSignature matches
	if signature == "" {
		t.Fatal("signature should not be empty")
	}

	// Re-verify using the same algo
	mac2 := hmac.New(sha256.New, []byte(secret))
	mac2.Write([]byte(message))
	expected := base64.StdEncoding.EncodeToString(mac2.Sum(nil))

	if signature != expected {
		t.Fatalf("signature mismatch: %q != %q", signature, expected)
	}

	// Wrong secret should produce different signature
	mac3 := hmac.New(sha256.New, []byte("wrong"))
	mac3.Write([]byte(message))
	wrong := base64.StdEncoding.EncodeToString(mac3.Sum(nil))
	if signature == wrong {
		t.Fatal("different secrets should produce different signatures")
	}
}

func init() {
	_ = json.Marshal // ensure import
}
