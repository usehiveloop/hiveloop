package main

// ServiceConfig defines how to introspect a GraphQL API and generate actions.
type ServiceConfig struct {
	Name             string   // service name (maps to metadata.json key)
	IntrospectionURL string   // GraphQL endpoint for live introspection (used if SchemaURL is empty)
	SchemaURL        string   // URL to a pre-published introspection JSON file (preferred over live introspection)
	NangoProviders   []string // nango provider IDs that share this API
	QueryFilters     []string // include only query fields matching these prefixes (empty = all)
	MutationFilters  []string // include only mutation fields matching these prefixes (empty = all)
}

// AllServices returns the registry of GraphQL providers.
func AllServices() []ServiceConfig {
	return []ServiceConfig{
		{
			Name:             "linear",
			SchemaURL:        "https://raw.githubusercontent.com/linearapp/linear/refs/heads/master/packages/sdk/src/schema.graphql",
			IntrospectionURL: "https://api.linear.app/graphql",
			NangoProviders:   []string{"linear", "linear-mcp"},
		},
		{
			Name:             "monday",
			IntrospectionURL: "https://api.monday.com/v2",
			NangoProviders:   []string{"monday"},
		},
		{
			Name:             "shopify",
			IntrospectionURL: "https://shopify.dev/admin-graphql-direct-proxy/2025-04",
			NangoProviders:   []string{"shopify", "shopify-api-key"},
		},
		{
			Name:             "gitlab",
			IntrospectionURL: "https://gitlab.com/api/graphql",
			NangoProviders:   []string{"gitlab", "gitlab-pat"},
		},
		{
			Name:             "contentful",
			IntrospectionURL: "https://graphql.contentful.com",
			NangoProviders:   []string{"contentful"},
		},
		{
			Name:             "braintree",
			SchemaURL:        "https://raw.githubusercontent.com/braintree/graphql-api/refs/heads/master/schema.graphql",
			IntrospectionURL: "https://payments.braintree-api.com/graphql",
			NangoProviders:   []string{"braintree", "braintree-sandbox"},
		},
	}
}
