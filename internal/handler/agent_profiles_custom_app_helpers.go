package handler

import (
	"context"
	"strings"

	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/nango"
)

func profileCustomAppPlaceholderCredentials(authMode string, scopes []string, creds *nango.Credentials, webhookSecret string) *nango.Credentials {
	if creds == nil {
		creds = &nango.Credentials{Type: authMode}
	}
	if creds.Type == "" {
		creds.Type = authMode
	}
	switch authMode {
	case "OAUTH1", "OAUTH2", "TBA":
		if creds.ClientID == "" {
			creds.ClientID = "hiveloop-placeholder-client-id-8f47c2d91b6a"
		}
		if creds.ClientSecret == "" {
			creds.ClientSecret = "hiveloop-placeholder-client-secret-3a91e58c0d74"
		}
	}
	if webhookSecret != "" && creds.WebhookSecret == "" {
		creds.WebhookSecret = webhookSecret
	}
	applyProfileCustomAppScopes(creds, scopes)
	return creds
}

func profileCustomAppConfigured(meta model.JSON) bool {
	if meta == nil {
		return false
	}
	v, _ := meta["custom_app_configured"].(bool)
	return v
}

func profileCustomAppConfiguredMeta(existing model.JSON, incoming model.JSON) model.JSON {
	next := model.JSON{}
	for key, value := range existing {
		next[key] = value
	}
	for key, value := range incoming {
		next[key] = value
	}
	next["custom_app_configured"] = true
	return next
}

func applyProfileCustomAppScopes(creds *nango.Credentials, scopes []string) {
	if creds == nil || len(scopes) == 0 {
		return
	}
	creds.Scopes = strings.Join(scopes, ",")
}

func (h *AgentProfileHandler) refreshedProfileCustomAppConfig(ctx context.Context, nangoProvider string, nangoKey string, needsWebhookSecret bool) (model.JSON, error) {
	config := h.profileCustomAppNangoConfig(ctx, nangoProvider, nangoKey)
	if !needsWebhookSecret || stringFromJSON(config, "webhook_secret") != "" {
		return config, nil
	}
	secret, err := randomHex(24)
	if err != nil {
		return nil, err
	}
	config["webhook_secret"] = secret
	return config, nil
}

func (h *AgentProfileHandler) profileCustomAppNangoConfig(ctx context.Context, nangoProvider string, nangoKey string) model.JSON {
	integResp, err := h.nango.GetIntegration(ctx, nangoKey)
	if err != nil {
		logging.FromContext(ctx).WarnContext(ctx, "failed to fetch profile custom app integration details", "error", err, "nango_key", nangoKey)
		template, _ := h.nango.GetProviderTemplate(nangoProvider)
		return buildNangoConfig(nil, template, h.nango.CallbackURL())
	}
	template, _ := h.nango.GetProviderTemplate(nangoProvider)
	return buildNangoConfig(integResp, template, h.nango.CallbackURL())
}
