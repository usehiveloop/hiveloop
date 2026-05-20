package catalog

import (
	"fmt"
	"sort"
)

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
// given provider. Wildcard ["*"] is NOT allowed - all actions must be explicit.
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

// GetExecution returns the execution config for a specific provider action.
func (c *Catalog) GetExecution(provider, actionKey string) (*ExecutionConfig, bool) {
	action, ok := c.GetAction(provider, actionKey)
	if !ok || action.Execution == nil {
		return nil, false
	}
	return action.Execution, true
}
