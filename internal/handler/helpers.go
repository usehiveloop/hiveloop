package handler

import (
	"fmt"
	"strings"

	"github.com/usehiveloop/hiveloop/internal/mcp/catalog"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/nango"
)

func boolPtr(v bool) *bool { return &v }

func derefBool(p *bool, fallback bool) bool {
	if p != nil {
		return *p
	}
	return fallback
}

func providerRequiresWebhookConfig(provider string) bool {
	cat := catalog.Global()
	pt, ok := cat.GetProviderTriggers(provider)
	if !ok {
		pt, ok = cat.GetProviderTriggersForVariant(provider)
	}
	if !ok || pt.WebhookConfig == nil {
		return false
	}
	return pt.WebhookConfig.WebhookURLRequired
}

func buildConnectionProviderConfig(nangoResp map[string]any) model.JSON {
	config := model.JSON{}
	for _, key := range []string{"connection_config", "metadata", "credentials", "provider"} {
		if v, exists := nangoResp[key]; exists && v != nil {
			config[key] = v
		}
	}
	if cc, ok := config["connection_config"].(map[string]any); ok {
		delete(cc, "jwtToken")
	}
	if creds, ok := config["credentials"].(map[string]any); ok {
		delete(creds, "jwtToken")
	}
	return config
}

func isDuplicateKeyError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "duplicate key")
}

func buildNangoConfig(integResp map[string]any, template map[string]any, callbackURL string) model.JSON {
	config := model.JSON{}
	if data, ok := integResp["data"].(map[string]any); ok {
		for _, key := range []string{"logo", "webhook_url", "forward_webhooks"} {
			if v, exists := data[key]; exists {
				config[key] = v
			}
		}
		if creds, ok := data["credentials"].(map[string]any); ok {
			if ws, ok := creds["webhook_secret"].(string); ok && ws != "" {
				config["webhook_secret"] = ws
			}
		}
	}
	if template != nil {
		if authMode, ok := template["auth_mode"].(string); ok {
			config["auth_mode"] = authMode
		}
	}
	if callbackURL != "" {
		config["callback_url"] = callbackURL
	}
	return config
}

func validateCredentials(provider nango.Provider, creds *nango.Credentials) error {
	mode := provider.AuthMode
	switch mode {
	case "OAUTH1", "OAUTH2", "TBA":
		if creds == nil {
			return fmt.Errorf("credentials required for %s auth mode", mode)
		}
		if creds.Type != mode {
			return fmt.Errorf("credentials.type must be %q for provider %q", mode, provider.Name)
		}
		if creds.ClientID == "" {
			return fmt.Errorf("client_id is required for %s auth mode", mode)
		}
		if creds.ClientSecret == "" {
			return fmt.Errorf("client_secret is required for %s auth mode", mode)
		}
	case "APP":
		if creds == nil {
			return fmt.Errorf("credentials required for APP auth mode")
		}
		if creds.Type != "APP" {
			return fmt.Errorf("credentials.type must be \"APP\" for provider %q", provider.Name)
		}
	}
	return nil
}
