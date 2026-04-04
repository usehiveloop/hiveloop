package handler

import (
	"testing"

	"github.com/ziraloop/ziraloop/internal/nango"
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

func TestBuildNangoConfig_ExtractsWebhookSecretForAPP(t *testing.T) {
	integResp := map[string]any{
		"data": map[string]any{
			"credentials": map[string]any{
				"type":           "APP",
				"app_id":         "123",
				"private_key":    "base64key",
				"app_link":       "https://example.com/app",
				"webhook_secret": "nango-computed-secret",
			},
		},
	}
	template := map[string]any{
		"auth_mode": "APP",
	}

	config := buildNangoConfig(integResp, template, "https://nango.example.com/oauth/callback")

	if config["webhook_secret"] != "nango-computed-secret" {
		t.Fatalf("expected webhook_secret=nango-computed-secret, got %v", config["webhook_secret"])
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
