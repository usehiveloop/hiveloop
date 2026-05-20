package catalog

import (
	"fmt"
	"sort"
	"strings"
)

// GetProviderTriggers returns all trigger definitions for a provider.
func (c *Catalog) GetProviderTriggers(provider string) (*ProviderTriggers, bool) {
	pt, ok := c.triggers[provider]
	return pt, ok
}

// GetProviderTriggersForVariant looks up triggers by stripping common suffixes
// from variant provider names (e.g., "github-app" -> "github", "jira-basic" -> "jira").
func (c *Catalog) GetProviderTriggersForVariant(variant string) (*ProviderTriggers, bool) {
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
