package main

// ServiceConfig defines how to introspect a GraphQL API and generate actions.
type ServiceConfig struct {
	Name             string            // service name (maps to metadata.json key)
	IntrospectionURL string            // GraphQL endpoint for live introspection (used if SchemaURL is empty)
	SchemaURL        string            // URL to a pre-published introspection JSON file (preferred over live introspection)
	NangoProviders   []string          // nango provider IDs that share this API
	QueryFilters     []string          // include only query fields matching these prefixes (empty = all)
	MutationFilters  []string          // include only mutation fields matching these prefixes (empty = all)
	ResourcePrefixes map[string]string // GraphQL field name prefix → resource_type (longest match wins)
	IncludeFields    []string          // exact GraphQL field names to include (empty = all); takes precedence over filters
}

// AllServices returns the registry of GraphQL providers.
func AllServices() []ServiceConfig {
	return []ServiceConfig{
		{
			Name:             "linear",
			SchemaURL:        "https://raw.githubusercontent.com/linearapp/linear/refs/heads/master/packages/sdk/src/schema.graphql",
			IntrospectionURL: "https://api.linear.app/graphql",
			NangoProviders:   []string{"linear"},
			ResourcePrefixes: map[string]string{
				"issue":         "issue",
				"issues":        "issue",
				"team":          "team",
				"teams":         "team",
				"project":       "project",
				"projects":      "project",
				"projectUpdate": "project",
				"cycle":         "cycle",
				"cycles":        "cycle",
				"comment":       "comment",
				"comments":      "comment",
				"document":      "document",
				"documents":     "document",
				"user":          "user",
				"users":         "user",
				"workflowState": "workflow_state",
				"workflowStates": "workflow_state",
				"issueLabel":    "label",
				"issueLabels":   "label",
				"attachment":    "attachment",
				"attachments":   "attachment",
				"notification":  "notification",
				"reaction":      "reaction",
				"webhook":       "webhook",
				"customView":    "custom_view",
			},
			IncludeFields: []string{
				// ── Issue ──
				"issue", "issuePriorityValues",
				"issueCreate", "issueUpdate", "issueArchive", "issueUnarchive", "issueDelete",
				"issueAddLabel", "issueRemoveLabel",
				"issueRelationCreate", "issueRelationDelete",
				"issueSubscribe", "issueUnsubscribe",
				"issueBatchUpdate",

				// ── Comment ──
				"comment",
				"commentCreate", "commentDelete", "commentResolve", "commentUnresolve",

				// ── Project ──
				"project", "projectUpdate", // projectUpdate: query reads a status‐update entity; mutation updates a project
				"projectArchive", "projectUnarchive",
				"projectAddLabel", "projectRemoveLabel",
				"projectMilestoneCreate", "projectMilestoneUpdate",
				"projectUpdateCreate", "projectRelationCreate",

				// ── Team ──
				"team", "teamMembership",
				"teamMembershipCreate", "teamUpdate",

				// ── Cycle ──
				"cycle",
				"cycleCreate", "cycleUpdate", "cycleArchive",

				// ── Label ──
				"issueLabel",
				"issueLabelDelete", "issueLabelRestore", "issueLabelRetire",

				// ── Document ──
				"document",
				"documentCreate", "documentUpdate", "documentDelete",

				// ── Workflow State ──
				"workflowState",
				"workflowStateCreate", "workflowStateUpdate", "workflowStateArchive",

				// ── User ──
				"user", "userSettings", "viewer",

				// ── Attachment ──
				"attachment",
				"attachmentCreate", "attachmentUpdate", "attachmentDelete",

				// ── Reactions ──
				"reactionCreate", "reactionDelete",

				// ── Cross‐cutting ──
				"organization",
				"notification", "notificationArchive", "notificationMarkReadAll",
				"customView", "customViewCreate",
				"emoji", "template", "templates",
				"webhookCreate",
			},
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
