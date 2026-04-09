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

// githubResources returns the 10 GitHub resource definitions with action filtering.
func githubResources() map[string]ResourceFilterConfig {
	return map[string]ResourceFilterConfig{
		"repository": {
			DisplayName: "Repositories",
			Description: "GitHub repositories the AI can access",
			IDField:     "full_name",
			NameField:   "name",
			Icon:         "repo",
			RefBindings: map[string]string{
				"owner": "$refs.owner",
				"repo":  "$refs.repo",
			},
			ListAction:  "/installation/repositories",
			ListRequestConfig: &RequestConfig{
				Method:       "GET",
				QueryParams:  map[string]string{"per_page": "100"},
				ResponsePath: "repositories",
			},
			PathPrefixes: []string{
				"/repos/{owner}/{repo}/collaborators",
				"/repos/{owner}/{repo}/topics",
				"/repos/{owner}/{repo}/forks",
				"/repos/{owner}/{repo}/contributors",
				"/repos/{owner}/{repo}/languages",
				"/repos/{owner}/{repo}/readme",
				"/repos/{owner}/{repo}/contents",
				"/repos/{owner}/{repo}/git",
				"/repos/{owner}/{repo}/commits",
				"/repos/{owner}/{repo}/comments",
				"/repos/{owner}/{repo}/stargazers",
				"/installation/repositories",
				"/user/repos",
				"/search/repositories",
				"/search/code",
				"/search/commits",
			},
			ExactPaths: []string{
				"/repos/{owner}/{repo}",
			},
		},
		"issue": {
			DisplayName: "Issues",
			Description: "GitHub issues within repositories",
			IDField:     "number",
			NameField:   "title",
			Icon:         "issue-opened",
			RefBindings: map[string]string{
				"owner":        "$refs.owner",
				"repo":         "$refs.repo",
				"issue_number": "$refs.issue_number",
			},
			ListAction:  "/repos/{owner}/{repo}/issues",
			ListRequestConfig: &RequestConfig{
				Method: "GET",
				QueryParams: map[string]string{
					"per_page": "100",
					"state":    "all",
				},
			},
			PathPrefixes: []string{
				"/repos/{owner}/{repo}/issues",
				"/search/issues",
			},
		},
		"pull_request": {
			DisplayName: "Pull Requests",
			Description: "GitHub pull requests for code review and merging",
			IDField:     "number",
			NameField:   "title",
			Icon:         "git-pull-request",
			RefBindings: map[string]string{
				"owner":       "$refs.owner",
				"repo":        "$refs.repo",
				"pull_number": "$refs.pull_number",
			},
			ListAction:  "/repos/{owner}/{repo}/pulls",
			ListRequestConfig: &RequestConfig{
				Method: "GET",
				QueryParams: map[string]string{
					"per_page": "100",
					"state":    "all",
				},
			},
			PathPrefixes: []string{
				"/repos/{owner}/{repo}/pulls",
			},
		},
		"release": {
			DisplayName: "Releases",
			Description: "GitHub releases for versioning and distribution",
			IDField:     "id",
			NameField:   "tag_name",
			Icon:         "tag",
			RefBindings: map[string]string{
				"owner":      "$refs.owner",
				"repo":       "$refs.repo",
				"release_id": "$refs.release_id",
			},
			ListAction:  "/repos/{owner}/{repo}/releases",
			ListRequestConfig: &RequestConfig{
				Method:      "GET",
				QueryParams: map[string]string{"per_page": "100"},
			},
			PathPrefixes: []string{
				"/repos/{owner}/{repo}/releases",
			},
		},
		"workflow": {
			DisplayName: "Workflows",
			Description: "GitHub Actions CI/CD workflows and runs",
			IDField:     "id",
			NameField:   "name",
			Icon:         "play",
			RefBindings: map[string]string{
				"owner":  "$refs.owner",
				"repo":   "$refs.repo",
				"run_id": "$refs.run_id",
			},
			ListAction:  "/repos/{owner}/{repo}/actions/workflows",
			ListRequestConfig: &RequestConfig{
				Method:       "GET",
				QueryParams:  map[string]string{"per_page": "100"},
				ResponsePath: "workflows",
			},
			PathPrefixes: []string{
				"/repos/{owner}/{repo}/actions/workflows",
				"/repos/{owner}/{repo}/actions/runs",
				"/repos/{owner}/{repo}/actions/jobs",
				"/repos/{owner}/{repo}/actions/artifacts",
			},
		},
		"label": {
			DisplayName: "Labels",
			Description: "GitHub labels for categorizing issues and pull requests",
			IDField:     "id",
			NameField:   "name",
			Icon:         "tag",
			RefBindings: map[string]string{
				"owner": "$refs.owner",
				"repo":  "$refs.repo",
			},
			ListAction:  "/repos/{owner}/{repo}/labels",
			ListRequestConfig: &RequestConfig{
				Method:      "GET",
				QueryParams: map[string]string{"per_page": "100"},
			},
			PathPrefixes: []string{
				"/repos/{owner}/{repo}/labels",
			},
		},
		"milestone": {
			DisplayName: "Milestones",
			Description: "GitHub milestones for tracking progress",
			IDField:     "number",
			NameField:   "title",
			Icon:         "milestone",
			RefBindings: map[string]string{
				"owner":            "$refs.owner",
				"repo":             "$refs.repo",
				"milestone_number": "$refs.milestone_number",
			},
			ListAction:  "/repos/{owner}/{repo}/milestones",
			ListRequestConfig: &RequestConfig{
				Method:      "GET",
				QueryParams: map[string]string{"per_page": "100"},
			},
			PathPrefixes: []string{
				"/repos/{owner}/{repo}/milestones",
			},
		},
		"branch": {
			DisplayName: "Branches",
			Description: "GitHub repository branches",
			IDField:     "name",
			NameField:   "name",
			Icon:         "git-branch",
			RefBindings: map[string]string{
				"owner":  "$refs.owner",
				"repo":   "$refs.repo",
				"branch": "$refs.branch_name",
			},
			ListAction:  "/repos/{owner}/{repo}/branches",
			ListRequestConfig: &RequestConfig{
				Method:      "GET",
				QueryParams: map[string]string{"per_page": "100"},
			},
			PathPrefixes: []string{
				"/repos/{owner}/{repo}/branches",
			},
		},
		"organization": {
			DisplayName: "Organizations",
			Description: "GitHub organizations",
			IDField:     "login",
			NameField:   "login",
			Icon:         "organization",
			RefBindings: map[string]string{
				"org": "$refs.org",
			},
			ListAction:  "/user/orgs",
			ListRequestConfig: &RequestConfig{
				Method:      "GET",
				QueryParams: map[string]string{"per_page": "100"},
			},
			PathPrefixes: []string{
				"/orgs/{org}/members",
				"/orgs/{org}/invitations",
				"/orgs/{org}/hooks",
				"/user/orgs",
			},
			ExactPaths: []string{
				"/orgs/{org}",
			},
		},
		"team": {
			DisplayName: "Teams",
			Description: "GitHub organization teams",
			IDField:     "slug",
			NameField:   "name",
			Icon:         "people",
			RefBindings: map[string]string{
				"org":       "$refs.org",
				"team_slug": "$refs.team_slug",
			},
			ListAction:  "/orgs/{org}/teams",
			ListRequestConfig: &RequestConfig{
				Method:      "GET",
				QueryParams: map[string]string{"per_page": "100"},
			},
			PathPrefixes: []string{
				"/orgs/{org}/teams",
			},
		},
	}
}
