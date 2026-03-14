// Package catalog provides an embedded actions catalog for integration providers.
// The JSON is embedded at build time via go:embed and parsed once at init,
// giving O(1) provider/action lookups and zero network latency.
package catalog

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
)

//go:embed actions.json
var actionsJSON []byte

// ProviderActions describes a provider and its available actions.
type ProviderActions struct {
	DisplayName string               `json:"display_name"`
	Actions     map[string]ActionDef `json:"actions"`
}

// ActionDef describes a single action a provider supports.
type ActionDef struct {
	DisplayName  string          `json:"display_name"`
	Description  string          `json:"description"`
	ResourceType string          `json:"resource_type"` // e.g. "channel", "repo", "" if none
	Parameters   json.RawMessage `json:"parameters"`    // JSON Schema
}

// Catalog holds all providers and their actions, indexed for fast lookup.
type Catalog struct {
	providers map[string]*ProviderActions
}

var (
	globalCatalog *Catalog
	initOnce      sync.Once
)

// Global returns the singleton catalog, parsing the embedded JSON on first call.
func Global() *Catalog {
	initOnce.Do(func() {
		globalCatalog = mustParse(actionsJSON)
	})
	return globalCatalog
}

func mustParse(data []byte) *Catalog {
	var providers map[string]ProviderActions
	if err := json.Unmarshal(data, &providers); err != nil {
		panic("catalog: failed to parse embedded actions.json: " + err.Error())
	}

	c := &Catalog{
		providers: make(map[string]*ProviderActions, len(providers)),
	}
	for name := range providers {
		p := providers[name]
		c.providers[name] = &p
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

// ValidateResources checks that every resource type key in the resources map
// matches the resource_type of at least one action in the given action list.
func (c *Catalog) ValidateResources(provider string, actions []string, resources map[string][]string) error {
	if len(resources) == 0 {
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

	for resourceType := range resources {
		if !validResourceTypes[resourceType] {
			return fmt.Errorf("resource type %q does not match any listed action for provider %q", resourceType, provider)
		}
	}

	return nil
}
