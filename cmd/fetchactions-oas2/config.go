package main

// ServiceConfig defines how to fetch and parse a Swagger 2.0 spec for a service.
type ServiceConfig struct {
	Name           string            // service name (maps to metadata.json key)
	SpecSource     string            // URL to the Swagger 2.0 spec file
	NangoProviders []string          // nango provider IDs that share this API surface
	PathFilters    []string          // include only paths matching these prefixes (empty = all)
	PathExcludes   []string          // exclude paths matching these prefixes
	ExtraHeaders   map[string]string // added to every action's execution.headers
	TagResourceMap map[string]string // OpenAPI tags → resource_type
}

// AllServices returns the registry of Swagger 2.0 providers.
func AllServices() []ServiceConfig {
	return []ServiceConfig{
		{
			Name:           "slack",
			SpecSource:     "https://raw.githubusercontent.com/slackapi/slack-api-specs/refs/heads/master/web-api/slack_web_openapi_v2.json",
			NangoProviders: []string{"slack"},
			TagResourceMap: map[string]string{
				"conversations": "channel",
				"chat":          "channel",
			},
		},
		{
			Name:           "zoom",
			SpecSource:     "https://raw.githubusercontent.com/zoom/api/refs/heads/master/openapi.v2.json",
			NangoProviders: []string{"zoom"},
		},
	}
}
