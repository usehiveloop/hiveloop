package catalog

import "encoding/json"

// RequestConfig defines custom request configuration for resource discovery.
type RequestConfig struct {
	Method       string            `json:"method,omitempty"`        // HTTP method (GET, POST, etc.)
	Headers      map[string]string `json:"headers,omitempty"`       // Custom headers to add
	QueryParams  map[string]string `json:"query_params,omitempty"`  // Static query parameters
	BodyTemplate map[string]any    `json:"body_template,omitempty"` // Default body for POST requests
	ResponsePath string            `json:"response_path,omitempty"` // Dot-notation path to items (e.g., "data.items")
}

// ResourceDef describes a resource type that can be configured for a provider.
type ResourceDef struct {
	DisplayName   string            `json:"display_name"`
	Description   string            `json:"description"`
	IDField       string            `json:"id_field"`
	NameField     string            `json:"name_field"`
	Icon          string            `json:"icon,omitempty"`
	ListAction    string            `json:"list_action"`
	RequestConfig *RequestConfig    `json:"request_config,omitempty"` // Optional request customization
	RefBindings   map[string]string `json:"ref_bindings,omitempty"`   // action_param_name -> "$refs.ref_name" mapping for auto-filling context action params
	// ResourceKeyTemplate is a $refs.x template that produces a stable identifier
	// for a specific resource instance. Used by the trigger dispatcher to decide
	// whether a new event should continue an existing agent conversation or start
	// a new one. Empty means "always start a new conversation" (appropriate for
	// event families with no natural continuation, like push or release).
	//
	// Examples:
	//   issue:        "$refs.owner/$refs.repo#issue-$refs.issue_number"
	//   pull_request: "$refs.owner/$refs.repo#pr-$refs.pull_number"
	//   (intercom)    "$refs.conversation_id"
	//
	// The template MUST reference only ref names that every trigger feeding this
	// resource exposes. If any $refs.x fails to resolve, the dispatcher treats
	// the key as empty to avoid silently merging unrelated resources.
	ResourceKeyTemplate string `json:"resource_key_template,omitempty"`

	// Configurable marks this resource type as selectable for agent scoping.
	// When true, users can pick specific instances (e.g., specific repos) to
	// grant an agent access to.
	Configurable bool `json:"configurable,omitempty"`
}

// ProviderActions describes a provider and its available actions.
type ProviderActions struct {
	DisplayName string                      `json:"display_name"`
	PushToMCP   *bool                       `json:"push_to_mcp,omitempty"` // nil or true = expose via MCP; false = accessed via proxy instead
	Resources   map[string]ResourceDef      `json:"resources"`
	Actions     map[string]ActionDef        `json:"actions"`
	Schemas     map[string]SchemaDefinition `json:"schemas,omitempty"`
}

// ShouldPushToMCP returns whether this provider's actions should be exposed
// via the MCP server. Defaults to true when not explicitly set.
func (pa *ProviderActions) ShouldPushToMCP() bool {
	return pa.PushToMCP == nil || *pa.PushToMCP
}

// ExecutionConfig defines how to execute an action against a provider's API via Nango proxy.
type ExecutionConfig struct {
	Method           string            `json:"method"`                      // HTTP method (GET, POST, etc.)
	Path             string            `json:"path"`                        // Provider API path (via Nango proxy)
	BodyMapping      map[string]string `json:"body_mapping,omitempty"`      // Param name -> body field mapping
	QueryMapping     map[string]string `json:"query_mapping,omitempty"`     // Param name -> query param mapping
	Headers          map[string]string `json:"headers,omitempty"`           // Extra provider headers
	ResponsePath     string            `json:"response_path,omitempty"`     // Dot-path to extract data from response
	GraphQLOperation string            `json:"graphql_operation,omitempty"` // "query" or "mutation" (GraphQL providers only)
	GraphQLField     string            `json:"graphql_field,omitempty"`     // Top-level GraphQL field name (e.g. "issueCreate")
	GraphQLQuery     string            `json:"graphql_query,omitempty"`     // Full GraphQL query/mutation string with $variable placeholders
}

// Access type constants.
const (
	AccessRead  = "read"
	AccessWrite = "write"
)

// ActionDef describes a single action a provider supports.
type ActionDef struct {
	DisplayName    string           `json:"display_name"`
	Description    string           `json:"description"`
	Access         string           `json:"access"`                    // "read" or "write"
	ResourceType   string           `json:"resource_type"`             // e.g. "channel", "repo", "" if none
	Parameters     json.RawMessage  `json:"parameters"`                // JSON Schema
	Execution      *ExecutionConfig `json:"execution,omitempty"`       // How to execute this action via Nango proxy
	ResponseSchema string           `json:"response_schema,omitempty"` // Ref into Schemas map
}

// SchemaDefinition is a flattened response/payload schema with top-level properties only.
type SchemaDefinition struct {
	Type       string                       `json:"type"`
	Properties map[string]SchemaPropertyDef `json:"properties,omitempty"`
	Items      *SchemaRef                   `json:"items,omitempty"` // for array types
}

// SchemaPropertyDef describes a single property in a schema.
type SchemaPropertyDef struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	Nullable    bool   `json:"nullable,omitempty"`
	SchemaRef   string `json:"schema_ref,omitempty"` // references another schema by name for nested object resolution
}

// SchemaRef references another schema by name.
type SchemaRef struct {
	Ref string `json:"$ref,omitempty"`
}

// TriggerDef describes a single webhook event trigger a provider supports.
type TriggerDef struct {
	DisplayName         string             `json:"display_name"`
	Description         string             `json:"description"`
	ResourceType        string             `json:"resource_type"`                   // which resource this trigger relates to
	PayloadSchema       string             `json:"payload_schema,omitempty"`        // ref into ProviderTriggers.Schemas
	Refs                map[string]string  `json:"refs,omitempty"`                  // ref_name -> dot-path into webhook payload for entity extraction
	SummaryRefs         map[string]string  `json:"summary_refs,omitempty"`          // display_name -> dot-path, curated subset rendered into the agent's user message; full payload goes to bridge full_message
	Enrichment          []EnrichmentAction `json:"enrichment,omitempty"`            // actions to run for pre-fetching context before dispatching to agent
	ResourceKeyTemplate string             `json:"resource_key_template,omitempty"` // canonical resource_key template with {ref_name} placeholders for subscription routing
}

// EnrichmentAction defines a provider action to run during trigger enrichment.
// Params values starting with "$refs." are substituted from extracted refs.
type EnrichmentAction struct {
	Action string         `json:"action"`           // action key from the provider's actions.json
	As     string         `json:"as"`               // label for the result in the composed instructions
	Params map[string]any `json:"params,omitempty"` // action parameters - $refs.xxx values are substituted
}

// WebhookConfig describes manual webhook configuration requirements for
// providers that don't support automatic webhook registration (e.g. Railway).
// When present, the frontend should show a modal after connection setup with
// the webhook URL the user needs to paste into the provider's dashboard.
type WebhookConfig struct {
	// WebhookURLRequired indicates the user must manually configure a webhook
	// URL in the provider's dashboard for triggers to work.
	WebhookURLRequired bool `json:"webhook_url_required"`
	// ConfigurationNotes is markdown text shown to the user explaining how to
	// configure the webhook in the provider's settings.
	ConfigurationNotes string `json:"configuration_notes"`
}

// ProviderTriggers describes a provider's webhook event triggers.
type ProviderTriggers struct {
	DisplayName   string                      `json:"display_name"`
	WebhookConfig *WebhookConfig              `json:"webhook_config,omitempty"`
	Triggers      map[string]TriggerDef       `json:"triggers"`
	Schemas       map[string]SchemaDefinition `json:"schemas,omitempty"`
}

// SubscribableResource describes a class of external resource that provider
// webhook triggers can use for affinity. The server parses the resource id,
// substitutes the named groups into CanonicalTemplate, and uses that canonical
// key as the trigger resource key for employee runtime conversations.
//
// Example (github_pull_request):
//
//	id_pattern:         "^(?P<owner>[\\w.-]+)/(?P<repo>[\\w.-]+)#(?P<number>\\d+)$"
//	id_example:         "usehivy/hivy#99"
//	canonical_template: "github/{owner}/{repo}/pull/{number}"
//
// agent input "usehivy/hivy#99" becomes canonical key "github/usehivy/hivy/pull/99"
type SubscribableResource struct {
	DisplayName       string   `json:"display_name"`
	Description       string   `json:"description,omitempty"`
	IDPattern         string   `json:"id_pattern"`         // Named-group regex for validating resource_id.
	IDExample         string   `json:"id_example"`         // Shown to the agent in errors + documentation.
	CanonicalTemplate string   `json:"canonical_template"` // {name} placeholders substituted from IDPattern groups.
	Events            []string `json:"events,omitempty"`   // Trigger keys that emit events for this resource.
}

// ProviderSubscribableResources is the top-level shape of a *.resources.json
// file. Provider identifies which integration this catalog file describes -
// it must match the provider value stored on integrations.
type ProviderSubscribableResources struct {
	Provider    string                          `json:"provider"`
	DisplayName string                          `json:"display_name"`
	Description string                          `json:"description,omitempty"`
	Resources   map[string]SubscribableResource `json:"resources"`
}

// Catalog holds all providers and their actions/triggers, indexed for fast lookup.
type Catalog struct {
	providers          map[string]*ProviderActions
	triggers           map[string]*ProviderTriggers
	subscribableByType map[string]subscribableEntry // resource_type -> provider + def
	subscribableByProv map[string]map[string]SubscribableResource
}

// subscribableEntry holds a subscribable resource definition together with
// its owning provider so lookups by resource_type return everything the
// service layer needs without a second map traversal.
type subscribableEntry struct {
	Provider string
	Def      SubscribableResource
}
