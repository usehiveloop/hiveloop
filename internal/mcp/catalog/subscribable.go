package catalog

import "sort"

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
