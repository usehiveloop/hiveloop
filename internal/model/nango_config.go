package model

// ConnectionConfigField describes a single field in a Nango connection_config schema.
type ConnectionConfigField struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Type        string `json:"type"`
	Automated   bool   `json:"automated,omitempty"`
	Optional    bool   `json:"optional,omitempty"`
	Format      string `json:"format,omitempty"`
	Pattern     string `json:"pattern,omitempty"`
	Example     string `json:"example,omitempty"`
	DocSection  string `json:"doc_section,omitempty"`
}

// NangoConfig holds the sanitized, non-secret configuration extracted from Nango
// for a provider integration. Stored as JSONB in the database.
type NangoConfig struct {
	AuthMode                 string                        `json:"auth_mode"`
	AuthorizationURL         string                        `json:"authorization_url,omitempty"`
	CallbackURL              string                        `json:"callback_url"`
	Categories               []string                      `json:"categories,omitempty"`
	ConnectionConfig         map[string]ConnectionConfigField `json:"connection_config,omitempty"`
	CredentialsSchema        map[string]any                `json:"credentials_schema,omitempty"`
	Docs                     string                        `json:"docs,omitempty"`
	DocsConnect              string                        `json:"docs_connect,omitempty"`
	ForwardWebhooks          bool                          `json:"forward_webhooks,omitempty"`
	Installation             string                        `json:"installation,omitempty"`
	Logo                     string                        `json:"logo,omitempty"`
	SetupGuideURL            string                        `json:"setup_guide_url,omitempty"`
	WebhookRoutingScript     string                        `json:"webhook_routing_script,omitempty"`
	WebhookSecret            string                        `json:"webhook_secret,omitempty"`
	WebhookURL               string                        `json:"webhook_url,omitempty"`
	WebhookUserDefinedSecret bool                          `json:"webhook_user_defined_secret,omitempty"`
}
