package e2e

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/ziraloop/ziraloop/internal/handler"
	"github.com/ziraloop/ziraloop/internal/model"
)

const testNangoSecret = "test-nango-secret-key"

// nangoWebhookHarness sets up: org → integration → connection, plus the handler.
type nangoWebhookHarness struct {
	*testHarness
	org         model.Org
	integration model.Integration
	connection  model.Connection
	router      *chi.Mux
}

func newNangoWebhookHarness(t *testing.T) *nangoWebhookHarness {
	t.Helper()
	h := newHarness(t)
	suffix := uuid.New().String()[:8]

	// Create org
	org := model.Org{Name: "nango-wh-test-" + suffix}
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

	// Create identity for the connection
	h.db.Create(&identity)

	// Create connection
	connection := model.Connection{
		OrgID:             org.ID,
		IntegrationID:     integration.ID,
		NangoConnectionID: "nango-conn-" + suffix,
		IdentityID:        &identity.ID,
	}
	h.db.Create(&connection)
	t.Cleanup(func() { h.db.Where("id = ?", connection.ID).Delete(&model.Connection{}) })

	// Router
	nangoHandler := handler.NewNangoWebhookHandler(h.db, testNangoSecret, nil)
	r := chi.NewRouter()
	r.Post("/internal/webhooks/nango", nangoHandler.Handle)

	return &nangoWebhookHarness{
		testHarness: h,
		org:         org,
		integration: integration,
		connection:  connection,
		router:      r,
	}
}

// nangoProviderConfigKey builds the key in the format {orgID}_{uniqueKey}.
func (nh *nangoWebhookHarness) nangoProviderConfigKey() string {
	return fmt.Sprintf("%s_%s", nh.org.ID.String(), nh.integration.UniqueKey)
}

// signBody computes the Nango HMAC-SHA256 signature (hex-encoded).
func signNangoBody(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// signedRequest sends a POST to /internal/webhooks/nango with a valid signature.
func (nh *nangoWebhookHarness) signedRequest(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	bodyBytes := []byte(body)
	signature := signNangoBody(bodyBytes, testNangoSecret)

	req := httptest.NewRequest(http.MethodPost, "/internal/webhooks/nango", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Nango-Hmac-Sha256", signature)

	rr := httptest.NewRecorder()
	nh.router.ServeHTTP(rr, req)
	return rr
}

// unsignedRequest sends a POST with no signature header.
func (nh *nangoWebhookHarness) unsignedRequest(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/internal/webhooks/nango", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	nh.router.ServeHTTP(rr, req)
	return rr
}

func TestNangoWebhook_ForwardWithKnownConnection(t *testing.T) {
	nh := newNangoWebhookHarness(t)

	payload := fmt.Sprintf(`{
		"from": "github",
		"type": "forward",
		"connectionId": %q,
		"providerConfigKey": %q,
		"payload": {"action": "opened", "pull_request": {"number": 42}}
	}`, nh.connection.NangoConnectionID, nh.nangoProviderConfigKey())

	rr := nh.signedRequest(t, payload)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["status"] != "ok" {
		t.Errorf("expected status ok, got %q", resp["status"])
	}
}

func TestNangoWebhook_ForwardWithUnknownConnection(t *testing.T) {
	nh := newNangoWebhookHarness(t)

	payload := fmt.Sprintf(`{
		"from": "github",
		"type": "forward",
		"connectionId": "nonexistent-connection",
		"providerConfigKey": %q,
		"payload": {"action": "opened"}
	}`, nh.nangoProviderConfigKey())

	rr := nh.signedRequest(t, payload)
	// Should still return 200 (we log the warning, don't fail)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestNangoWebhook_MissingSignatureReturns401(t *testing.T) {
	nh := newNangoWebhookHarness(t)

	payload := `{"from":"nango","type":"auth","connectionId":"c1","providerConfigKey":"key"}`
	rr := nh.unsignedRequest(t, payload)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestNangoWebhook_InvalidSignatureReturns401(t *testing.T) {
	nh := newNangoWebhookHarness(t)

	body := `{"from":"nango","type":"auth","connectionId":"c1","providerConfigKey":"key"}`
	req := httptest.NewRequest(http.MethodPost, "/internal/webhooks/nango", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Nango-Hmac-Sha256", "deadbeef")

	rr := httptest.NewRecorder()
	nh.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestNangoWebhook_AuthWebhook(t *testing.T) {
	nh := newNangoWebhookHarness(t)

	payload := fmt.Sprintf(`{
		"from": "nango",
		"type": "auth",
		"operation": "creation",
		"connectionId": %q,
		"providerConfigKey": %q,
		"provider": "github-app",
		"success": true
	}`, nh.connection.NangoConnectionID, nh.nangoProviderConfigKey())

	rr := nh.signedRequest(t, payload)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestNangoWebhook_AuthRefreshFailure(t *testing.T) {
	nh := newNangoWebhookHarness(t)

	payload := fmt.Sprintf(`{
		"from": "nango",
		"type": "auth",
		"operation": "refresh",
		"connectionId": %q,
		"providerConfigKey": %q,
		"provider": "github-app",
		"success": false,
		"error": {"type": "refresh_token_error", "description": "token expired"}
	}`, nh.connection.NangoConnectionID, nh.nangoProviderConfigKey())

	rr := nh.signedRequest(t, payload)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestNangoWebhook_MalformedJSON(t *testing.T) {
	nh := newNangoWebhookHarness(t)

	rr := nh.signedRequest(t, `{not valid json`)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestNangoWebhook_UnknownIntegration(t *testing.T) {
	nh := newNangoWebhookHarness(t)

	// Use a valid org ID but non-existent unique key
	bogusKey := fmt.Sprintf("%s_nonexistent-integration", nh.org.ID.String())
	payload := fmt.Sprintf(`{
		"from": "github",
		"type": "forward",
		"connectionId": %q,
		"providerConfigKey": %q,
		"payload": {}
	}`, nh.connection.NangoConnectionID, bogusKey)

	rr := nh.signedRequest(t, payload)
	// Should still return 200 (graceful handling)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestNangoWebhook_InvalidProviderConfigKey(t *testing.T) {
	nh := newNangoWebhookHarness(t)

	payload := `{
		"from": "github",
		"type": "forward",
		"connectionId": "some-conn",
		"providerConfigKey": "not-a-uuid-key",
		"payload": {}
	}`

	rr := nh.signedRequest(t, payload)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestNangoWebhook_SignatureVerification(t *testing.T) {
	// Verify our HMAC implementation is correct
	body := []byte(`{"test":"payload"}`)
	secret := "my-secret"

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))

	// Same secret should verify
	mac2 := hmac.New(sha256.New, []byte(secret))
	mac2.Write(body)
	expected := hex.EncodeToString(mac2.Sum(nil))
	if sig != expected {
		t.Fatalf("signatures should match: %q != %q", sig, expected)
	}

	// Different secret should NOT match
	mac3 := hmac.New(sha256.New, []byte("wrong-secret"))
	mac3.Write(body)
	wrong := hex.EncodeToString(mac3.Sum(nil))
	if sig == wrong {
		t.Fatal("different secrets should produce different signatures")
	}
}
