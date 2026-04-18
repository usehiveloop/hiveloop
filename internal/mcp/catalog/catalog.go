// Package catalog provides an embedded actions catalog for integration providers.
// Each provider's JSON is stored as a separate file under providers/ and embedded
// at build time via go:embed, giving O(1) provider/action lookups and zero network latency.
package catalog

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"sync"
)

//go:embed providers/*.actions.json
var providersFS embed.FS

//go:embed providers/*.triggers.json
var triggersFS embed.FS

//go:embed providers/*.resources.json
var resourcesFS embed.FS

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
	RefBindings   map[string]string `json:"ref_bindings,omitempty"`   // action_param_name → "$refs.ref_name" mapping for auto-filling context action params
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
	// resource exposes — if any $refs.x fails to resolve, the dispatcher treats
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
	BodyMapping      map[string]string `json:"body_mapping,omitempty"`      // Param name → body field mapping
	QueryMapping     map[string]string `json:"query_mapping,omitempty"`     // Param name → query param mapping
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
	Access         string           `json:"access"`                        // "read" or "write"
	ResourceType   string           `json:"resource_type"`                 // e.g. "channel", "repo", "" if none
	Parameters     json.RawMessage  `json:"parameters"`                    // JSON Schema
	Execution      *ExecutionConfig `json:"execution,omitempty"`           // How to execute this action via Nango proxy
	ResponseSchema string           `json:"response_schema,omitempty"`     // Ref into Schemas map
}

// SchemaDefinition is a flattened response/payload schema with top-level properties only.
type SchemaDefinition struct {
	Type       string                          `json:"type"`
	Properties map[string]SchemaPropertyDef    `json:"properties,omitempty"`
	Items      *SchemaRef                      `json:"items,omitempty"` // for array types
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
	Refs                map[string]string  `json:"refs,omitempty"`                  // ref_name → dot-path into webhook payload for entity extraction
	Enrichment          []EnrichmentAction `json:"enrichment,omitempty"`            // actions to run for pre-fetching context before dispatching to agent
	ResourceKeyTemplate string             `json:"resource_key_template,omitempty"` // canonical resource_key template with {ref_name} placeholders for subscription routing
}

// EnrichmentAction defines a provider action to run during trigger enrichment.
// Params values starting with "$refs." are substituted from extracted refs.
type EnrichmentAction struct {
	Action string         `json:"action"`           // action key from the provider's actions.json
	As     string         `json:"as"`               // label for the result in the composed instructions
	Params map[string]any `json:"params,omitempty"` // action parameters — $refs.xxx values are substituted
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

// SubscribableResource describes a class of external resource that an agent
// can subscribe to via the subscribe_to_events MCP tool. The agent supplies
// a resource_id in the format captured by IDPattern; the server parses it,
// substitutes the named groups into CanonicalTemplate, and writes that
// canonical key into conversation_subscriptions. Future webhook events whose
// dispatcher-computed resource key matches this canonical form will route
// into the subscribed conversation.
//
// Example (github_pull_request):
//   id_pattern:         "^(?P<owner>[\\w.-]+)/(?P<repo>[\\w.-]+)#(?P<number>\\d+)$"
//   id_example:         "ziraloop/ziraloop#99"
//   canonical_template: "github/{owner}/{repo}/pull/{number}"
//   → agent input "ziraloop/ziraloop#99" becomes canonical key "github/ziraloop/ziraloop/pull/99"
type SubscribableResource struct {
	DisplayName       string   `json:"display_name"`
	Description       string   `json:"description,omitempty"`
	IDPattern         string   `json:"id_pattern"`         // Named-group regex for validating resource_id.
	IDExample         string   `json:"id_example"`         // Shown to the agent in errors + documentation.
	CanonicalTemplate string   `json:"canonical_template"` // {name} placeholders substituted from IDPattern groups.
	Events            []string `json:"events,omitempty"`   // Trigger keys that emit events for this resource.
}

// ProviderSubscribableResources is the top-level shape of a *.resources.json
// file. Provider identifies which integration this catalog file describes —
// it must match the provider value stored on in_integrations.
type ProviderSubscribableResources struct {
	Provider    string                          `json:"provider"`
	DisplayName string                          `json:"display_name"`
	Description string                          `json:"description,omitempty"`
	Resources   map[string]SubscribableResource `json:"resources"`
}

// Catalog holds all providers and their actions/triggers, indexed for fast lookup.
type Catalog struct {
	providers            map[string]*ProviderActions
	triggers             map[string]*ProviderTriggers
	subscribableByType   map[string]subscribableEntry             // resource_type → provider + def
	subscribableByProv   map[string]map[string]SubscribableResource
}

// subscribableEntry holds a subscribable resource definition together with
// its owning provider so lookups by resource_type return everything the
// service layer needs without a second map traversal.
type subscribableEntry struct {
	Provider string
	Def      SubscribableResource
}

var (
	globalCatalog *Catalog
	initOnce      sync.Once
)

// Global returns the singleton catalog, parsing the embedded provider files on first call.
func Global() *Catalog {
	initOnce.Do(func() {
		globalCatalog = mustParse()
	})
	return globalCatalog
}

func mustParse() *Catalog {
	c := &Catalog{
		providers:          make(map[string]*ProviderActions),
		triggers:           make(map[string]*ProviderTriggers),
		subscribableByType: make(map[string]subscribableEntry),
		subscribableByProv: make(map[string]map[string]SubscribableResource),
	}

	// Parse *.actions.json files.
	entries, err := fs.ReadDir(providersFS, "providers")
	if err != nil {
		panic("catalog: failed to read embedded providers directory: " + err.Error())
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".actions.json") {
			continue
		}

		providerKey := strings.TrimSuffix(name, ".actions.json")

		data, err := fs.ReadFile(providersFS, "providers/"+name)
		if err != nil {
			panic("catalog: failed to read " + name + ": " + err.Error())
		}

		var pa ProviderActions
		if err := json.Unmarshal(data, &pa); err != nil {
			panic("catalog: failed to parse " + name + ": " + err.Error())
		}

		c.providers[providerKey] = &pa
	}

	// Parse *.triggers.json files.
	triggerEntries, err := fs.ReadDir(triggersFS, "providers")
	if err != nil {
		panic("catalog: failed to read embedded triggers directory: " + err.Error())
	}

	for _, entry := range triggerEntries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".triggers.json") {
			continue
		}

		providerKey := strings.TrimSuffix(name, ".triggers.json")

		data, err := fs.ReadFile(triggersFS, "providers/"+name)
		if err != nil {
			panic("catalog: failed to read " + name + ": " + err.Error())
		}

		var pt ProviderTriggers
		if err := json.Unmarshal(data, &pt); err != nil {
			panic("catalog: failed to parse " + name + ": " + err.Error())
		}

		c.triggers[providerKey] = &pt
	}

	// Parse *.resources.json files — subscribable-event catalog per provider.
	resourceEntries, err := fs.ReadDir(resourcesFS, "providers")
	if err != nil {
		panic("catalog: failed to read embedded resources directory: " + err.Error())
	}

	for _, entry := range resourceEntries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".resources.json") {
			continue
		}

		data, err := fs.ReadFile(resourcesFS, "providers/"+name)
		if err != nil {
			panic("catalog: failed to read " + name + ": " + err.Error())
		}

		var psr ProviderSubscribableResources
		if err := json.Unmarshal(data, &psr); err != nil {
			panic("catalog: failed to parse " + name + ": " + err.Error())
		}

		if psr.Provider == "" {
			panic("catalog: " + name + " is missing the top-level \"provider\" field")
		}

		// Per-provider map for fast listing.
		if _, exists := c.subscribableByProv[psr.Provider]; !exists {
			c.subscribableByProv[psr.Provider] = make(map[string]SubscribableResource)
		}

		for resourceType, def := range psr.Resources {
			if existing, clash := c.subscribableByType[resourceType]; clash {
				panic(fmt.Sprintf(
					"catalog: subscribable resource_type %q declared by both %q and %q — keys must be globally unique across providers",
					resourceType, existing.Provider, psr.Provider,
				))
			}
			c.subscribableByType[resourceType] = subscribableEntry{
				Provider: psr.Provider,
				Def:      def,
			}
			c.subscribableByProv[psr.Provider][resourceType] = def
		}
	}

	return c
}

// GetProvider returns a provider by its name (e.g. "slack", "github").
func (c *Catalog) GetProvider(name string) (*ProviderActions, bool) {
	p, ok := c.providers[name]
	return p, ok
}

// GetAction returns a specific action for a provider.
func (c *Catalog) GetAction(provider, actionKey string) (*ActionDef, bool) {
	p, ok := c.providers[provider]
	if !ok {
		return nil, false
	}
	a, ok := p.Actions[actionKey]
	if !ok {
		return nil, false
	}
	return &a, true
}

// ListProviders returns all provider names sorted alphabetically.
func (c *Catalog) ListProviders() []string {
	names := make([]string, 0, len(c.providers))
	for name := range c.providers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ListActions returns all actions for a provider.
func (c *Catalog) ListActions(provider string) map[string]ActionDef {
	p, ok := c.providers[provider]
	if !ok {
		return nil
	}
	return p.Actions
}

// ValidateActions checks that every action key exists in the catalog for the
// given provider. Wildcard ["*"] is NOT allowed — all actions must be explicit.
func (c *Catalog) ValidateActions(provider string, actions []string) error {
	p, ok := c.providers[provider]
	if !ok {
		return fmt.Errorf("unknown provider %q in actions catalog", provider)
	}

	if len(p.Actions) == 0 {
		return fmt.Errorf("provider %q has no actions defined in the catalog", provider)
	}

	for _, action := range actions {
		if action == "*" {
			return fmt.Errorf("wildcard actions are not allowed; explicitly list each action")
		}
		if _, ok := p.Actions[action]; !ok {
			return fmt.Errorf("unknown action %q for provider %q", action, provider)
		}
	}

	return nil
}

// ListResourceTypes returns all resource types for a provider.
func (c *Catalog) ListResourceTypes(provider string) map[string]ResourceDef {
	p, ok := c.providers[provider]
	if !ok {
		return nil
	}
	return p.Resources
}

// GetResourceDef returns a specific resource definition for a provider.
func (c *Catalog) GetResourceDef(provider, resourceType string) (*ResourceDef, bool) {
	p, ok := c.providers[provider]
	if !ok {
		return nil, false
	}
	r, ok := p.Resources[resourceType]
	if !ok {
		return nil, false
	}
	return &r, true
}

// HasConfigurableResources returns true if the provider has at least one
// resource with configurable: true.
func (c *Catalog) HasConfigurableResources(provider string) bool {
	return len(c.GetConfigurableResources(provider)) > 0
}

// ConfigurableResourceSummary is a lightweight descriptor returned to frontends
// so they know which resource types can be scoped on an agent.
type ConfigurableResourceSummary struct {
	Key         string `json:"key"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
}

// GetConfigurableResources returns the resource types marked configurable: true
// for a provider.
func (c *Catalog) GetConfigurableResources(provider string) []ConfigurableResourceSummary {
	p, ok := c.providers[provider]
	if !ok {
		return nil
	}
	var result []ConfigurableResourceSummary
	for key, resDef := range p.Resources {
		if resDef.Configurable {
			result = append(result, ConfigurableResourceSummary{
				Key:         key,
				DisplayName: resDef.DisplayName,
				Description: resDef.Description,
			})
		}
	}
	return result
}

// GetExecution returns the execution config for a specific provider action.
func (c *Catalog) GetExecution(provider, actionKey string) (*ExecutionConfig, bool) {
	action, ok := c.GetAction(provider, actionKey)
	if !ok || action.Execution == nil {
		return nil, false
	}
	return action.Execution, true
}

// ValidateResources checks that every resource type key in the resources map
// matches the resource_type of at least one action in the given action list,
// and that each resource ID is in the allowed set from the connection.
func (c *Catalog) ValidateResources(provider string, actions []string, requestedResources, allowedResources map[string][]string) error {
	if len(requestedResources) == 0 {
		return nil
	}

	p, ok := c.providers[provider]
	if !ok {
		return fmt.Errorf("unknown provider %q in actions catalog", provider)
	}

	// Build set of valid resource types from the listed actions
	validResourceTypes := make(map[string]bool)
	for _, actionKey := range actions {
		if action, ok := p.Actions[actionKey]; ok && action.ResourceType != "" {
			validResourceTypes[action.ResourceType] = true
		}
	}

	for resourceType, requestedIDs := range requestedResources {
		// Check resource type is valid for these actions
		if !validResourceTypes[resourceType] {
			return fmt.Errorf("resource type %q does not match any listed action for provider %q", resourceType, provider)
		}

		// Check each requested ID is in the allowed set (nil means no restrictions)
		if allowedResources != nil {
			allowedIDs := allowedResources[resourceType]
			allowedSet := make(map[string]bool, len(allowedIDs))
			for _, id := range allowedIDs {
				allowedSet[id] = true
			}

			for _, reqID := range requestedIDs {
				if !allowedSet[reqID] {
					return fmt.Errorf("resource %q of type %q not configured for this connection", reqID, resourceType)
				}
			}
		}
	}

	return nil
}

// --- Trigger lookup methods ---

// GetProviderTriggers returns all trigger definitions for a provider.
func (c *Catalog) GetProviderTriggers(provider string) (*ProviderTriggers, bool) {
	pt, ok := c.triggers[provider]
	return pt, ok
}

// GetProviderTriggersForVariant looks up triggers by stripping common suffixes
// from variant provider names (e.g., "github-app" → "github", "jira-basic" → "jira").
func (c *Catalog) GetProviderTriggersForVariant(variant string) (*ProviderTriggers, bool) {
	// Try progressively shorter prefixes by stripping dash-separated suffixes.
	name := variant
	for {
		idx := strings.LastIndex(name, "-")
		if idx <= 0 {
			return nil, false
		}
		name = name[:idx]
		if pt, ok := c.triggers[name]; ok {
			return pt, ok
		}
	}
}

// GetTrigger returns a specific trigger definition for a provider.
func (c *Catalog) GetTrigger(provider, triggerKey string) (*TriggerDef, bool) {
	pt, ok := c.triggers[provider]
	if !ok {
		return nil, false
	}
	t, ok := pt.Triggers[triggerKey]
	if !ok {
		return nil, false
	}
	return &t, true
}

// ListTriggers returns all trigger keys for a provider sorted alphabetically.
func (c *Catalog) ListTriggers(provider string) []string {
	pt, ok := c.triggers[provider]
	if !ok {
		return nil
	}
	names := make([]string, 0, len(pt.Triggers))
	for name := range pt.Triggers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ListTriggersForResource returns all trigger keys that match a given resource type.
func (c *Catalog) ListTriggersForResource(provider, resourceType string) []string {
	pt, ok := c.triggers[provider]
	if !ok {
		return nil
	}
	var names []string
	for name, trigger := range pt.Triggers {
		if trigger.ResourceType == resourceType {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// ValidateTriggers checks that every trigger key exists in the catalog for the provider.
func (c *Catalog) ValidateTriggers(provider string, triggerKeys []string) error {
	pt, ok := c.triggers[provider]
	if !ok {
		return fmt.Errorf("provider %q has no triggers defined in the catalog", provider)
	}

	for _, key := range triggerKeys {
		if _, ok := pt.Triggers[key]; !ok {
			return fmt.Errorf("unknown trigger %q for provider %q", key, provider)
		}
	}

	return nil
}

// HasTriggers returns true if the provider has trigger definitions.
func (c *Catalog) HasTriggers(provider string) bool {
	pt, ok := c.triggers[provider]
	if !ok {
		return false
	}
	return len(pt.Triggers) > 0
}

// ListProvidersWithTriggers returns provider names that have triggers, sorted alphabetically.
func (c *Catalog) ListProvidersWithTriggers() []string {
	var names []string
	for name, pt := range c.triggers {
		if len(pt.Triggers) > 0 {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// --- Subscribable resource lookup methods ---

// GetSubscribableResource returns the subscribable-resource definition for a
// given resource_type (e.g. "github_pull_request") together with the
// provider that owns it. resource_type is globally unique across providers;
// the panic in mustParse enforces this invariant at load time.
func (c *Catalog) GetSubscribableResource(resourceType string) (provider string, def SubscribableResource, ok bool) {
	entry, ok := c.subscribableByType[resourceType]
	if !ok {
		return "", SubscribableResource{}, false
	}
	return entry.Provider, entry.Def, true
}

// ListSubscribableResourcesForProvider returns every subscribable resource
// declared for the given provider, keyed by resource_type. Returns nil if
// the provider has no resources file.
func (c *Catalog) ListSubscribableResourcesForProvider(provider string) map[string]SubscribableResource {
	return c.subscribableByProv[provider]
}

// ListSubscribableResourceTypes returns every known resource_type across all
// providers, sorted alphabetically. Useful for building system reminders that
// show the agent its available types.
func (c *Catalog) ListSubscribableResourceTypes() []string {
	names := make([]string, 0, len(c.subscribableByType))
	for name := range c.subscribableByType {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// GetTriggerPayloadSchema returns the schema definition for a trigger's payload.
func (c *Catalog) GetTriggerPayloadSchema(provider, triggerKey string) (*SchemaDefinition, bool) {
	pt, ok := c.triggers[provider]
	if !ok {
		return nil, false
	}
	trigger, ok := pt.Triggers[triggerKey]
	if !ok || trigger.PayloadSchema == "" {
		return nil, false
	}
	schema, ok := pt.Schemas[trigger.PayloadSchema]
	if !ok {
		return nil, false
	}
	return &schema, true
}
