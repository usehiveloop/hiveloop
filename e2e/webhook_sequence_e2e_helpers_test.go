package e2e

import (
	"encoding/base64"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/usehivy/hivy/e2e/fakebridge"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/streaming"
)

type webhookHarness struct {
	h        *testHarness
	fb       *fakebridge.Server
	eventBus *streaming.EventBus
	agent    model.Employee
	conv     model.EmployeeConversation
}

func newRotatedEncKey(seed byte) (*crypto.SymmetricKey, error) {
	keyBytes := make([]byte, 32)
	for i := range keyBytes {
		keyBytes[i] = byte(i) + seed
	}
	return crypto.NewSymmetricKey(base64.StdEncoding.EncodeToString(keyBytes))
}

func newWebhookHarness(t *testing.T, prefix string, seed byte) *webhookHarness {
	return newWebhookHarnessWithSecret(t, prefix, seed, prefix+"-secret-"+uuid.New().String()[:8])
}

func newWebhookHarnessWithSecret(t *testing.T, prefix string, seed byte, bridgeSecret string) *webhookHarness {
	t.Helper()
	h := newHarness(t)
	suffix := uuid.New().String()[:8]

	encKey, err := newRotatedEncKey(seed)
	if err != nil {
		t.Fatalf("symmetric key: %v", err)
	}
	encryptedKey, err := encKey.EncryptString(bridgeSecret)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	org := model.Org{Name: prefix + "-org-" + suffix}
	h.db.Create(&org)
	t.Cleanup(func() { h.db.Where("id = ?", org.ID).Delete(&model.Org{}) })

	cred := model.Credential{
		OrgID: org.ID, BaseURL: "https://api.openai.com", AuthScheme: "bearer",
		ProviderID: "openai", EncryptedKey: []byte("enc"), WrappedDEK: []byte("dek"),
	}
	h.db.Create(&cred)
	t.Cleanup(func() { h.db.Where("id = ?", cred.ID).Delete(&model.Credential{}) })

	agent := model.Employee{
		OrgID: &org.ID, Name: prefix + "-agent-" + suffix,
		CredentialID: &cred.ID, SystemPrompt: "test", Model: "gpt-4o",
	}
	h.db.Create(&agent)
	t.Cleanup(func() { h.db.Where("id = ?", agent.ID).Delete(&model.Employee{}) })

	sb := model.Sandbox{
		OrgID: &org.ID, EmployeeID: &agent.ID,
		ExternalID: prefix + "-ext-" + suffix, BridgeURL: "https://test:25434",
		EncryptedBridgeAPIKey: encryptedKey, Status: "running",
	}
	h.db.Create(&sb)
	t.Cleanup(func() { h.db.Where("id = ?", sb.ID).Delete(&model.Sandbox{}) })

	conv := model.EmployeeConversation{
		OrgID: org.ID, EmployeeID: agent.ID, SandboxID: sb.ID,
		RuntimeConversationID: prefix + "-conv-" + suffix, Status: "active",
	}
	h.db.Create(&conv)
	t.Cleanup(func() {
		h.db.Where("conversation_id = ?", conv.ID).Delete(&model.ConversationEvent{})
		h.db.Where("id = ?", conv.ID).Delete(&model.EmployeeConversation{})
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

	return &webhookHarness{
		h:        h,
		fb:       fb,
		eventBus: eventBus,
		agent:    agent,
		conv:     conv,
	}
}
