package nango

func BuildConfig(integResp map[string]any, template map[string]any, callbackURL string) map[string]any {
	config := map[string]any{}
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
		for _, key := range []string{
			"auth_mode",
			"authorization_url",
			"categories",
			"connection_config",
			"credentials_schema",
			"docs",
			"docs_connect",
			"installation",
			"logo",
			"setup_guide_url",
			"webhook_routing_script",
			"webhook_user_defined_secret",
		} {
			if v, exists := template[key]; exists {
				config[key] = v
			}
		}
	}
	if callbackURL != "" {
		config["callback_url"] = callbackURL
	}
	return config
}
