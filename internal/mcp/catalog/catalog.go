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
	DisplayName   string         `json:"display_name"`
	Description   string         `json:"description"`
	IDField       string         `json:"id_field"`
	NameField     string         `json:"name_field"`
	Icon          string         `json:"icon,omitempty"`
	ListAction    string         `json:"list_action"`
	RequestConfig *RequestConfig `json:"request_config,omitempty"` // Optional request customization
}

// ProviderActions describes a provider and its available actions.
type ProviderActions struct {
	DisplayName string                 `json:"display_name"`
	Resources   map[string]ResourceDef `json:"resources"`
	Actions     map[string]ActionDef   `json:"actions"`
}

// ExecutionConfig defines how to execute an action against a provider's API via Nango proxy.
type ExecutionConfig struct {
	Method       string            `json:"method"`                  // HTTP method (GET, POST, etc.)
	Path         string            `json:"path"`                    // Provider API path (via Nango proxy)
	BodyMapping  map[string]string `json:"body_mapping,omitempty"`  // Param name → body field mapping
	QueryMapping map[string]string `json:"query_mapping,omitempty"` // Param name → query param mapping
	Headers      map[string]string `json:"headers,omitempty"`       // Extra provider headers
	ResponsePath string            `json:"response_path,omitempty"` // Dot-path to extract data from response
}

// Access type constants.
const (
	AccessRead  = "read"
	AccessWrite = "write"
)

// ActionDef describes a single action a provider supports.
type ActionDef struct {
	DisplayName  string           `json:"display_name"`
	Description  string           `json:"description"`
	Access       string           `json:"access"`              // "read" or "write"
	ResourceType string           `json:"resource_type"`       // e.g. "channel", "repo", "" if none
	Parameters   json.RawMessage  `json:"parameters"`          // JSON Schema
	Execution    *ExecutionConfig `json:"execution,omitempty"` // How to execute this action via Nango proxy
}

// Catalog holds all providers and their actions, indexed for fast lookup.
type Catalog struct {
	providers map[string]*ProviderActions
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
		providers: make(map[string]*ProviderActions),
	}

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

// HasConfigurableResources returns true if the provider has resource configuration.
func (c *Catalog) HasConfigurableResources(provider string) bool {
	p, ok := c.providers[provider]
	if !ok {
		return false
	}
	return len(p.Resources) > 0
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
