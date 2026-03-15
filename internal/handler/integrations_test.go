package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/llmvault/llmvault/internal/model"
	"github.com/llmvault/llmvault/internal/nango"
)

func TestBuildNangoConfig_ExtractsWebhookSecretFromCredentials(t *testing.T) {
	integResp := map[string]any{
		"data": map[string]any{
			"logo":        "https://example.com/logo.png",
			"webhook_url": "https://nango.example.com/webhook/abc",
			"credentials": map[string]any{
				"client_id":      "should-not-appear",
				"client_secret":  "should-not-appear",
				"webhook_secret": "user-defined-secret-123",
			},
		},
	}
	template := map[string]any{
		"auth_mode":                  "OAUTH2",
		"webhook_user_defined_secret": true,
	}

	config := buildNangoConfig(integResp, template, "https://nango.example.com/oauth/callback")

	// webhook_secret should be extracted
	if config["webhook_secret"] != "user-defined-secret-123" {
		t.Fatalf("expected webhook_secret=user-defined-secret-123, got %v", config["webhook_secret"])
	}

	// client_id/client_secret should NOT be extracted
	if _, exists := config["client_id"]; exists {
		t.Fatal("client_id should not be in nango_config")
	}
	if _, exists := config["client_secret"]; exists {
		t.Fatal("client_secret should not be in nango_config")
	}

	// webhook_user_defined_secret flag should be extracted from template
	if config["webhook_user_defined_secret"] != true {
		t.Fatalf("expected webhook_user_defined_secret=true, got %v", config["webhook_user_defined_secret"])
	}
}

func TestBuildNangoConfig_NoWebhookSecretWhenAbsent(t *testing.T) {
	integResp := map[string]any{
		"data": map[string]any{
			"logo":        "https://example.com/logo.png",
			"webhook_url": "https://nango.example.com/webhook/abc",
			"credentials": map[string]any{
				"client_id":     "id",
				"client_secret": "secret",
			},
		},
	}
	template := map[string]any{
		"auth_mode": "OAUTH2",
	}

	config := buildNangoConfig(integResp, template, "https://nango.example.com/oauth/callback")

	if _, exists := config["webhook_secret"]; exists {
		t.Fatal("webhook_secret should not be in config when not present in credentials")
	}
	if _, exists := config["webhook_user_defined_secret"]; exists {
		t.Fatal("webhook_user_defined_secret should not be in config when not in template")
	}
}

func TestBuildNangoConfig_NoCredentialsInResponse(t *testing.T) {
	integResp := map[string]any{
		"data": map[string]any{
			"logo":        "https://example.com/logo.png",
			"webhook_url": "https://nango.example.com/webhook/abc",
		},
	}
	template := map[string]any{
		"auth_mode": "APP",
	}

	config := buildNangoConfig(integResp, template, "https://nango.example.com/oauth/callback")

	// Should not crash and webhook_secret should be absent
	if _, exists := config["webhook_secret"]; exists {
		t.Fatal("webhook_secret should not be set when no credentials in response")
	}
}

func TestBuildNangoConfig_EmptyWebhookSecretIgnored(t *testing.T) {
	integResp := map[string]any{
		"data": map[string]any{
			"credentials": map[string]any{
				"webhook_secret": "",
			},
		},
	}
	template := map[string]any{
		"auth_mode": "OAUTH2",
	}

	config := buildNangoConfig(integResp, template, "https://nango.example.com/oauth/callback")

	if _, exists := config["webhook_secret"]; exists {
		t.Fatal("empty webhook_secret should not be stored in config")
	}
}

func TestWebhookSecretComputation_APP(t *testing.T) {
	// Replicate the hash logic used in Create/Update handlers
	creds := &nango.Credentials{
		AppID:      "12345",
		PrivateKey: "-----BEGIN RSA PRIVATE KEY-----\nfake\n-----END RSA PRIVATE KEY-----",
		AppLink:    "https://example.com/app",
	}

	hash := sha256.Sum256([]byte(creds.AppID + creds.PrivateKey + creds.AppLink))
	secret := hex.EncodeToString(hash[:])

	if secret == "" {
		t.Fatal("computed webhook_secret should not be empty")
	}
	if len(secret) != 64 {
		t.Fatalf("expected 64-char hex string, got %d chars: %s", len(secret), secret)
	}

	// Verify deterministic — same input produces same output
	hash2 := sha256.Sum256([]byte(creds.AppID + creds.PrivateKey + creds.AppLink))
	secret2 := hex.EncodeToString(hash2[:])
	if secret != secret2 {
		t.Fatal("webhook_secret computation should be deterministic")
	}

	// Different input produces different output
	hash3 := sha256.Sum256([]byte("different" + creds.PrivateKey + creds.AppLink))
	secret3 := hex.EncodeToString(hash3[:])
	if secret == secret3 {
		t.Fatal("different inputs should produce different webhook_secrets")
	}
}

func TestValidateCredentials_OAUTH2_AllowsWebhookSecret(t *testing.T) {
	provider := nango.Provider{Name: "slack", AuthMode: "OAUTH2"}
	creds := &nango.Credentials{
		Type:          "OAUTH2",
		ClientID:      "id",
		ClientSecret:  "secret",
		WebhookSecret: "user-webhook-secret",
	}

	err := validateCredentials(provider, creds)
	if err != nil {
		t.Fatalf("OAUTH2 with webhook_secret should pass validation, got: %v", err)
	}
}

func TestValidateCredentials_OAUTH2_WithoutWebhookSecret(t *testing.T) {
	provider := nango.Provider{Name: "slack", AuthMode: "OAUTH2"}
	creds := &nango.Credentials{
		Type:         "OAUTH2",
		ClientID:     "id",
		ClientSecret: "secret",
	}

	err := validateCredentials(provider, creds)
	if err != nil {
		t.Fatalf("OAUTH2 without webhook_secret should pass validation, got: %v", err)
	}
}

func TestWebhookSecretSetInNangoConfig_APP(t *testing.T) {
	// Simulate Create handler logic: compute webhook_secret for APP mode
	creds := &nango.Credentials{
		Type:       "APP",
		AppID:      "app-123",
		PrivateKey: "pk-secret",
		AppLink:    "https://github.com/apps/test",
	}
	nangoConfig := model.JSON{
		"auth_mode":    "APP",
		"callback_url": "https://nango.example.com/oauth/callback",
	}

	// Simulate the Create handler logic
	provider := nango.Provider{AuthMode: "APP"}
	if _, alreadySet := nangoConfig["webhook_secret"]; !alreadySet {
		switch provider.AuthMode {
		case "APP":
			hash := sha256.Sum256([]byte(creds.AppID + creds.PrivateKey + creds.AppLink))
			nangoConfig["webhook_secret"] = hex.EncodeToString(hash[:])
		}
	}

	ws, ok := nangoConfig["webhook_secret"].(string)
	if !ok || ws == "" {
		t.Fatal("expected webhook_secret to be set for APP mode")
	}
	if len(ws) != 64 {
		t.Fatalf("expected 64-char hex SHA256, got %d chars", len(ws))
	}

	// Verify it doesn't overwrite existing webhook_secret
	nangoConfig2 := model.JSON{
		"auth_mode":      "APP",
		"webhook_secret": "already-set",
	}
	if _, alreadySet := nangoConfig2["webhook_secret"]; !alreadySet {
		hash := sha256.Sum256([]byte(creds.AppID + creds.PrivateKey + creds.AppLink))
		nangoConfig2["webhook_secret"] = hex.EncodeToString(hash[:])
	}
	if nangoConfig2["webhook_secret"] != "already-set" {
		t.Fatal("should not overwrite existing webhook_secret")
	}
}
