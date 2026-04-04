package e2e

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/ziraloop/ziraloop/internal/crypto"
	"github.com/ziraloop/ziraloop/internal/handler"
	"github.com/ziraloop/ziraloop/internal/model"
)

const testNangoSecretFwd = "test-nango-secret-fwd"

// forwardingHarness extends the test with a downstream httptest.Server and OrgWebhookConfig.
type forwardingHarness struct {
	*testHarness
	org         model.Org
	integration model.Integration
	identity    model.Identity
	connection  model.Connection
	encKey      *crypto.SymmetricKey
	whSecret    string // plaintext webhook signing secret
	router      *chi.Mux

	// downstream captures
	mu              sync.Mutex
	lastReqBody     []byte
	lastReqHeaders  http.Header
	downstreamCode  int
	downstreamBody  string
	downstreamSrv   *httptest.Server
}

func newForwardingHarness(t *testing.T) *forwardingHarness {
	t.Helper()
	h := newHarness(t)
	suffix := uuid.New().String()[:8]

	// Encryption key for webhook secrets
	keyBytes := make([]byte, 32)
	for i := range keyBytes {
		keyBytes[i] = byte(i + 77)
	}
	encKey, err := crypto.NewSymmetricKey(base64.StdEncoding.EncodeToString(keyBytes))
	if err != nil {
		t.Fatal(err)
	}

	// Create org
	org := model.Org{Name: "nango-fwd-test-" + suffix}
	h.db.Create(&org)
	t.Cleanup(func() { h.db.Where("id = ?", org.ID).Delete(&model.Org{}) })

	// Create integration
	uniqueKey := "github-app-" + suffix
	integration := model.Integration{
		OrgID:       org.ID,
		UniqueKey:   uniqueKey,
		Provider:    "github-app",
		DisplayName: "Test GitHub App",
	}
	h.db.Create(&integration)
	t.Cleanup(func() { h.db.Where("id = ?", integration.ID).Delete(&model.Integration{}) })

	// Create identity
	identity := model.Identity{OrgID: org.ID, ExternalID: "fwd-user-" + suffix}
	h.db.Create(&identity)
	t.Cleanup(func() { h.db.Where("id = ?", identity.ID).Delete(&model.Identity{}) })

	// Create connection
	connection := model.Connection{
		OrgID:             org.ID,
		IntegrationID:     integration.ID,
		NangoConnectionID: "nango-conn-" + suffix,
		IdentityID:        &identity.ID,
	}
	h.db.Create(&connection)
	t.Cleanup(func() { h.db.Where("id = ?", connection.ID).Delete(&model.Connection{}) })

	fh := &forwardingHarness{
		testHarness: h,
		org:         org,
		integration: integration,
		identity:    identity,
		connection:  connection,
		encKey:      encKey,
		downstreamCode: http.StatusOK,
		downstreamBody: `{"received":true}`,
	}

	// Set up downstream server
	fh.downstreamSrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		fh.mu.Lock()
		fh.lastReqBody = body
		fh.lastReqHeaders = r.Header.Clone()
		code := fh.downstreamCode
		respBody := fh.downstreamBody
		fh.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		w.Write([]byte(respBody))
	}))
	t.Cleanup(func() { fh.downstreamSrv.Close() })

	// Generate and store webhook secret
	plaintext, prefix, err := model.GenerateWebhookSecret()
	if err != nil {
		t.Fatal(err)
	}
	fh.whSecret = plaintext

	encSecret, err := encKey.EncryptString(plaintext)
	if err != nil {
		t.Fatal(err)
	}

	whConfig := model.OrgWebhookConfig{
		OrgID:           org.ID,
		URL:             fh.downstreamSrv.URL,
		EncryptedSecret: encSecret,
		SecretPrefix:    prefix,
	}
	h.db.Create(&whConfig)
	t.Cleanup(func() { h.db.Where("id = ?", whConfig.ID).Delete(&model.OrgWebhookConfig{}) })

	// Router
	nangoHandler := handler.NewNangoWebhookHandler(h.db, testNangoSecretFwd, encKey)
	r := chi.NewRouter()
	r.Post("/internal/webhooks/nango", nangoHandler.Handle)
	fh.router = r

	return fh
}

func (fh *forwardingHarness) nangoProviderConfigKey() string {
	return fmt.Sprintf("%s_%s", fh.org.ID.String(), fh.integration.UniqueKey)
}

func (fh *forwardingHarness) signedRequest(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	bodyBytes := []byte(body)
	signature := signNangoBody(bodyBytes, testNangoSecretFwd)

	req := httptest.NewRequest(http.MethodPost, "/internal/webhooks/nango", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Nango-Hmac-Sha256", signature)

	rr := httptest.NewRecorder()
	fh.router.ServeHTTP(rr, req)
	return rr
}

func (fh *forwardingHarness) setDownstream(code int, body string) {
	fh.mu.Lock()
	defer fh.mu.Unlock()
	fh.downstreamCode = code
	fh.downstreamBody = body
}

func (fh *forwardingHarness) capturedBody() []byte {
	fh.mu.Lock()
	defer fh.mu.Unlock()
	return fh.lastReqBody
}

func (fh *forwardingHarness) capturedHeaders() http.Header {
	fh.mu.Lock()
	defer fh.mu.Unlock()
	return fh.lastReqHeaders
}

// --- Tests ---

func TestNangoWebhookForwarding_SuccessfulForward(t *testing.T) {
	fh := newForwardingHarness(t)

	payload := fmt.Sprintf(`{
		"from": "github",
		"type": "forward",
		"connectionId": %q,
		"providerConfigKey": %q,
		"provider": "github-app",
		"payload": {"action": "opened", "pull_request": {"number": 42}}
	}`, fh.connection.NangoConnectionID, fh.nangoProviderConfigKey())

	rr := fh.signedRequest(t, payload)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify downstream response was proxied
	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["received"] != true {
		t.Errorf("expected proxied response, got: %s", rr.Body.String())
	}
}

func TestNangoWebhookForwarding_NoConfigReturnsOK(t *testing.T) {
	h := newHarness(t)
	suffix := uuid.New().String()[:8]

	// Org WITHOUT webhook config
	org := model.Org{Name: "nango-noconfig-" + suffix}
	h.db.Create(&org)
	t.Cleanup(func() { h.db.Where("id = ?", org.ID).Delete(&model.Org{}) })

	integration := model.Integration{OrgID: org.ID, UniqueKey: "gh-" + suffix, Provider: "github-app", DisplayName: "GH"}
	h.db.Create(&integration)
	t.Cleanup(func() { h.db.Where("id = ?", integration.ID).Delete(&model.Integration{}) })

	identity := model.Identity{OrgID: org.ID, ExternalID: "usr-" + suffix}
	h.db.Create(&identity)
	t.Cleanup(func() { h.db.Where("id = ?", identity.ID).Delete(&model.Identity{}) })

	conn := model.Connection{OrgID: org.ID, IntegrationID: integration.ID, NangoConnectionID: "nc-" + suffix, IdentityID: &identity.ID}
	h.db.Create(&conn)
	t.Cleanup(func() { h.db.Where("id = ?", conn.ID).Delete(&model.Connection{}) })

	nangoHandler := handler.NewNangoWebhookHandler(h.db, testNangoSecretFwd, nil)
	r := chi.NewRouter()
	r.Post("/internal/webhooks/nango", nangoHandler.Handle)

	body := fmt.Sprintf(`{"from":"github","type":"forward","connectionId":%q,"providerConfigKey":"%s_%s","payload":{}}`,
		conn.NangoConnectionID, org.ID.String(), integration.UniqueKey)
	bodyBytes := []byte(body)
	signature := signNangoBody(bodyBytes, testNangoSecretFwd)

	req := httptest.NewRequest(http.MethodPost, "/internal/webhooks/nango", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Nango-Hmac-Sha256", signature)

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["status"] != "ok" {
		t.Errorf("expected {status:ok}, got: %s", rr.Body.String())
	}
}

func TestNangoWebhookForwarding_EnrichedPayload(t *testing.T) {
	fh := newForwardingHarness(t)

	payload := fmt.Sprintf(`{
		"from": "github",
		"type": "forward",
		"connectionId": %q,
		"providerConfigKey": %q,
		"provider": "github-app",
		"payload": {"action": "opened"}
	}`, fh.connection.NangoConnectionID, fh.nangoProviderConfigKey())

	rr := fh.signedRequest(t, payload)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Parse what downstream received
	var enriched map[string]any
	if err := json.Unmarshal(fh.capturedBody(), &enriched); err != nil {
		t.Fatalf("failed to parse captured body: %v", err)
	}

	// ZiraLoop fields present with correct values
	if enriched["org_id"] != fh.org.ID.String() {
		t.Errorf("org_id: got %v, want %s", enriched["org_id"], fh.org.ID)
	}
	if enriched["integration_id"] != fh.integration.ID.String() {
		t.Errorf("integration_id: got %v, want %s", enriched["integration_id"], fh.integration.ID)
	}
	if enriched["integration_name"] != "Test GitHub App" {
		t.Errorf("integration_name: got %v", enriched["integration_name"])
	}
	if enriched["connection_id"] != fh.connection.ID.String() {
		t.Errorf("connection_id: got %v, want %s", enriched["connection_id"], fh.connection.ID)
	}
	if enriched["identity_id"] != fh.identity.ID.String() {
		t.Errorf("identity_id: got %v, want %s", enriched["identity_id"], fh.identity.ID)
	}
	if enriched["type"] != "forward" {
		t.Errorf("type: got %v", enriched["type"])
	}
	if enriched["provider"] != "github-app" {
		t.Errorf("provider: got %v", enriched["provider"])
	}

	// Nango internals must NOT be present
	if _, exists := enriched["connectionId"]; exists {
		t.Error("Nango connectionId should not be in enriched payload")
	}
	if _, exists := enriched["providerConfigKey"]; exists {
		t.Error("Nango providerConfigKey should not be in enriched payload")
	}
	if _, exists := enriched["from"]; exists {
		t.Error("Nango 'from' field should not be in enriched payload")
	}

	// Original payload preserved
	payloadMap, ok := enriched["payload"].(map[string]any)
	if !ok {
		t.Fatal("payload should be an object")
	}
	if payloadMap["action"] != "opened" {
		t.Errorf("payload.action: got %v", payloadMap["action"])
	}
}

func TestNangoWebhookForwarding_SignatureValid(t *testing.T) {
	fh := newForwardingHarness(t)

	payload := fmt.Sprintf(`{
		"from": "github",
		"type": "forward",
		"connectionId": %q,
		"providerConfigKey": %q,
		"provider": "github-app",
		"payload": {}
	}`, fh.connection.NangoConnectionID, fh.nangoProviderConfigKey())

	fh.signedRequest(t, payload)

	headers := fh.capturedHeaders()
	sig := headers.Get("X-ZiraLoop-Signature")
	tsStr := headers.Get("X-ZiraLoop-Timestamp")

	if sig == "" {
		t.Fatal("X-ZiraLoop-Signature header missing")
	}
	if tsStr == "" {
		t.Fatal("X-ZiraLoop-Timestamp header missing")
	}

	ts, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil {
		t.Fatalf("invalid timestamp: %v", err)
	}

	// Recompute signature with known secret
	body := fh.capturedBody()
	message := fmt.Sprintf("%d.%s", ts, string(body))
	mac := hmac.New(sha256.New, []byte(fh.whSecret))
	mac.Write([]byte(message))
	expected := hex.EncodeToString(mac.Sum(nil))

	if sig != expected {
		t.Errorf("signature mismatch:\n  got:  %s\n  want: %s", sig, expected)
	}
}

func TestNangoWebhookForwarding_Downstream422ProxiedBack(t *testing.T) {
	fh := newForwardingHarness(t)
	fh.setDownstream(http.StatusUnprocessableEntity, `{"error":"validation failed"}`)

	payload := fmt.Sprintf(`{
		"from": "github",
		"type": "forward",
		"connectionId": %q,
		"providerConfigKey": %q,
		"provider": "github-app",
		"payload": {}
	}`, fh.connection.NangoConnectionID, fh.nangoProviderConfigKey())

	rr := fh.signedRequest(t, payload)
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["error"] != "validation failed" {
		t.Errorf("expected proxied error, got: %s", rr.Body.String())
	}
}

func TestNangoWebhookForwarding_Downstream503Returns502(t *testing.T) {
	fh := newForwardingHarness(t)
	fh.setDownstream(http.StatusServiceUnavailable, `{"error":"unavailable"}`)

	payload := fmt.Sprintf(`{
		"from": "github",
		"type": "forward",
		"connectionId": %q,
		"providerConfigKey": %q,
		"provider": "github-app",
		"payload": {}
	}`, fh.connection.NangoConnectionID, fh.nangoProviderConfigKey())

	rr := fh.signedRequest(t, payload)
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 (triggers Nango retry), got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestNangoWebhookForwarding_UnreachableReturns502(t *testing.T) {
	h := newHarness(t)
	suffix := uuid.New().String()[:8]

	keyBytes := make([]byte, 32)
	for i := range keyBytes {
		keyBytes[i] = byte(i + 77)
	}
	encKey, _ := crypto.NewSymmetricKey(base64.StdEncoding.EncodeToString(keyBytes))

	org := model.Org{Name: "nango-unreach-" + suffix}
	h.db.Create(&org)
	t.Cleanup(func() { h.db.Where("id = ?", org.ID).Delete(&model.Org{}) })

	integration := model.Integration{OrgID: org.ID, UniqueKey: "gh-" + suffix, Provider: "github-app", DisplayName: "GH"}
	h.db.Create(&integration)
	t.Cleanup(func() { h.db.Where("id = ?", integration.ID).Delete(&model.Integration{}) })

	identity := model.Identity{OrgID: org.ID, ExternalID: "usr-" + suffix}
	h.db.Create(&identity)
	t.Cleanup(func() { h.db.Where("id = ?", identity.ID).Delete(&model.Identity{}) })

	conn := model.Connection{OrgID: org.ID, IntegrationID: integration.ID, NangoConnectionID: "nc-" + suffix, IdentityID: &identity.ID}
	h.db.Create(&conn)
	t.Cleanup(func() { h.db.Where("id = ?", conn.ID).Delete(&model.Connection{}) })

	// Webhook config with unreachable URL
	plaintext, prefix, _ := model.GenerateWebhookSecret()
	encSecret, _ := encKey.EncryptString(plaintext)
	whConfig := model.OrgWebhookConfig{
		OrgID: org.ID, URL: "http://127.0.0.1:1", // nothing listening
		EncryptedSecret: encSecret, SecretPrefix: prefix,
	}
	h.db.Create(&whConfig)
	t.Cleanup(func() { h.db.Where("id = ?", whConfig.ID).Delete(&model.OrgWebhookConfig{}) })

	nangoHandler := handler.NewNangoWebhookHandler(h.db, testNangoSecretFwd, encKey)
	r := chi.NewRouter()
	r.Post("/internal/webhooks/nango", nangoHandler.Handle)

	body := fmt.Sprintf(`{"from":"github","type":"forward","connectionId":%q,"providerConfigKey":"%s_%s","provider":"github-app","payload":{}}`,
		conn.NangoConnectionID, org.ID.String(), integration.UniqueKey)
	bodyBytes := []byte(body)
	signature := signNangoBody(bodyBytes, testNangoSecretFwd)

	req := httptest.NewRequest(http.MethodPost, "/internal/webhooks/nango", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Nango-Hmac-Sha256", signature)

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 for unreachable endpoint, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestNangoWebhookForwarding_SecretRotation(t *testing.T) {
	fh := newForwardingHarness(t)
	oldSecret := fh.whSecret

	// Send first webhook — signature uses old secret
	payload := fmt.Sprintf(`{
		"from": "github",
		"type": "forward",
		"connectionId": %q,
		"providerConfigKey": %q,
		"provider": "github-app",
		"payload": {}
	}`, fh.connection.NangoConnectionID, fh.nangoProviderConfigKey())

	fh.signedRequest(t, payload)

	// Verify old secret works
	headers1 := fh.capturedHeaders()
	sig1 := headers1.Get("X-ZiraLoop-Signature")
	ts1, _ := strconv.ParseInt(headers1.Get("X-ZiraLoop-Timestamp"), 10, 64)
	body1 := fh.capturedBody()

	msg1 := fmt.Sprintf("%d.%s", ts1, string(body1))
	mac1 := hmac.New(sha256.New, []byte(oldSecret))
	mac1.Write([]byte(msg1))
	if sig1 != hex.EncodeToString(mac1.Sum(nil)) {
		t.Fatal("old secret should verify first request")
	}

	// Rotate secret in DB
	newPlaintext, newPrefix, _ := model.GenerateWebhookSecret()
	newEnc, _ := fh.encKey.EncryptString(newPlaintext)
	fh.db.Model(&model.OrgWebhookConfig{}).Where("org_id = ?", fh.org.ID).Updates(map[string]any{
		"encrypted_secret": newEnc,
		"secret_prefix":    newPrefix,
	})

	// Send second webhook
	fh.signedRequest(t, payload)

	headers2 := fh.capturedHeaders()
	sig2 := headers2.Get("X-ZiraLoop-Signature")
	ts2, _ := strconv.ParseInt(headers2.Get("X-ZiraLoop-Timestamp"), 10, 64)
	body2 := fh.capturedBody()

	// New secret should verify
	msg2 := fmt.Sprintf("%d.%s", ts2, string(body2))
	mac2 := hmac.New(sha256.New, []byte(newPlaintext))
	mac2.Write([]byte(msg2))
	if sig2 != hex.EncodeToString(mac2.Sum(nil)) {
		t.Fatal("new secret should verify second request")
	}

	// Old secret should NOT verify the second request
	mac3 := hmac.New(sha256.New, []byte(oldSecret))
	mac3.Write([]byte(msg2))
	if sig2 == hex.EncodeToString(mac3.Sum(nil)) {
		t.Fatal("old secret should NOT verify after rotation")
	}
}
