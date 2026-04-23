package main

// ServiceConfig defines how to fetch and parse an OpenAPI spec for a service.
type ServiceConfig struct {
	Name           string            // service name (maps to metadata.json key)
	SpecSource     string            // URL to the OpenAPI spec file
	NangoProviders []string          // nango provider IDs that share this API surface
	PathFilters    []string          // include only paths matching these prefixes (empty = all)
	PathExcludes   []string          // exclude paths matching these prefixes
	TagFilters     []string          // include only operations with these tags (empty = all)
	BasePathStrip  string            // strip this prefix from paths before output
	ExtraHeaders   map[string]string // added to every action's execution.headers
	// TagResourceMap maps OpenAPI tags to resource_type values.
	// e.g. {"Issues": "repo", "Pull Requests": "repo"}
	// Ignored when Resources is set.
	TagResourceMap map[string]string

	// Resources defines the resources for this provider and how to filter actions.
	// When set, replaces PathFilters, PathExcludes, and TagResourceMap entirely.
	// Only actions matching a resource's path patterns are included.
	Resources map[string]ResourceFilterConfig
}

// ResourceFilterConfig defines a resource and the path patterns used to filter actions for it.
type ResourceFilterConfig struct {
	// Display metadata (output to JSON)
	DisplayName string
	Description string
	IDField     string
	NameField   string
	Icon        string

	// List endpoint configuration for resource discovery
	ListAction        string
	ListRequestConfig *RequestConfig

	// Ref bindings — maps action param names to $refs for auto-filling context action params.
	// When a context action says ref: "issue", the system finds this resource and uses these bindings.
	RefBindings map[string]string

	// ResourceKeyTemplate — $refs.x template producing a stable identifier
	// used by the trigger dispatcher to decide continue-vs-new-conversation.
	// Empty means "no continuation" (always new conversation per event).
	ResourceKeyTemplate string

	// Action filtering — actions matching these paths belong to this resource
	PathPrefixes []string // any action path starting with these prefixes
	ExactPaths   []string // any action path exactly equal to these
}

// AllServices returns the full registry of OpenAPI 3.x providers.
func AllServices() []ServiceConfig {
	return []ServiceConfig{
		// --- Phase 2a: Simple, well-structured OAS 3.0 specs ---
		{
			Name:           "jira",
			SpecSource:     "https://developer.atlassian.com/cloud/jira/platform/swagger-v3.v3.json",
			NangoProviders: []string{"jira", "jira-basic", "jira-data-center", "jira-data-center-api-key", "jira-data-center-basic"},
			PathFilters:    []string{"/rest/api/3/"},
			PathExcludes:   []string{"/rest/api/3/app", "/rest/api/3/auditing", "/rest/api/3/configuration", "/rest/api/3/jql"},
		},
		{
			Name:           "confluence",
			SpecSource:     "https://developer.atlassian.com/cloud/confluence/swagger.v3.json",
			NangoProviders: []string{"confluence", "confluence-basic", "confluence-data-center"},
		},
		{
			Name:           "asana",
			SpecSource:     "https://raw.githubusercontent.com/Asana/openapi/refs/heads/master/defs/asana_oas.yaml",
			NangoProviders: []string{"asana", "asana-mcp"},
		},
		{
			Name:           "pagerduty",
			SpecSource:     "https://raw.githubusercontent.com/PagerDuty/api-schema/refs/heads/main/reference/REST/openapiv3.json",
			NangoProviders: []string{"pagerduty"},
		},
		{
			Name:           "intercom",
			SpecSource:     "https://raw.githubusercontent.com/intercom/Intercom-OpenAPI/refs/heads/main/descriptions/2.15/api.intercom.io.yaml",
			NangoProviders: []string{"intercom"},
		},
		{
			Name:           "box",
			SpecSource:     "https://raw.githubusercontent.com/box/box-openapi/refs/heads/main/openapi/openapi.json",
			NangoProviders: []string{"box"},
		},
		{
			Name:           "sentry",
			SpecSource:     "https://raw.githubusercontent.com/getsentry/sentry-api-schema/refs/heads/main/openapi-derefed.json",
			NangoProviders: []string{"sentry", "sentry-oauth"},
		},
		{
			Name:           "zendesk",
			SpecSource:     "https://developer.zendesk.com/zendesk/oas.yaml",
			NangoProviders: []string{"zendesk"},
		},

		// --- Phase 2b: Large/complex OAS 3.0 specs ---
		{
			Name:           "hubspot",
			SpecSource:     "https://raw.githubusercontent.com/HubSpot/HubSpot-public-api-spec-collection/refs/heads/main/PublicApiSpecs/CRM/Contacts/Rollouts/424/v3/contacts.json",
			NangoProviders: []string{"hubspot", "hubspot-mcp"},
		},
		{
			Name:           "stripe",
			SpecSource:     "https://raw.githubusercontent.com/stripe/openapi/refs/heads/master/openapi/spec3.json",
			NangoProviders: []string{"stripe", "stripe-api-key", "stripe-app", "stripe-app-sandbox", "stripe-express"},
			PathFilters:    []string{"/v1/charges", "/v1/customers", "/v1/subscriptions", "/v1/invoices", "/v1/payment_intents", "/v1/products", "/v1/prices", "/v1/refunds", "/v1/payment_methods", "/v1/checkout", "/v1/billing_portal"},
		},
		{
			Name:           "cloudflare",
			SpecSource:     "https://raw.githubusercontent.com/cloudflare/api-schemas/refs/heads/main/openapi.json",
			NangoProviders: []string{"cloudflare"},
			PathFilters:    []string{"/zones", "/dns", "/workers"},
		},
		{
			Name:           "vercel",
			SpecSource:     "https://openapi.vercel.sh/",
			NangoProviders: []string{"vercel"},
		},
		{
			Name:           "twilio",
			SpecSource:     "https://raw.githubusercontent.com/twilio/twilio-oai/refs/heads/main/spec/json/twilio_api_v2010.json",
			NangoProviders: []string{"twilio"},
		},

		// --- Phase 2c: OAS 3.1 specs (libopenapi handles 3.1 natively) ---
		{
			Name:           "github",
			SpecSource:     "https://raw.githubusercontent.com/github/rest-api-description/refs/heads/main/descriptions/api.github.com/api.github.com.json",
			NangoProviders: []string{"github", "github-app", "github-app-oauth", "github-pat"},
			Resources:      githubResources(),
		},
		{
			Name:           "figma",
			SpecSource:     "https://raw.githubusercontent.com/figma/rest-api-spec/refs/heads/main/openapi/openapi.yaml",
			NangoProviders: []string{"figma"},
		},
		{
			Name:           "discord",
			SpecSource:     "https://raw.githubusercontent.com/discord/discord-api-spec/refs/heads/main/specs/openapi.json",
			NangoProviders: []string{"discord"},
		},
	}
}
