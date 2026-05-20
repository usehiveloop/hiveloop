package catalog

import "fmt"

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

	validResourceTypes := make(map[string]bool)
	for _, actionKey := range actions {
		if action, ok := p.Actions[actionKey]; ok && action.ResourceType != "" {
			validResourceTypes[action.ResourceType] = true
		}
	}

	for resourceType, requestedIDs := range requestedResources {
		if !validResourceTypes[resourceType] {
			return fmt.Errorf("resource type %q does not match any listed action for provider %q", resourceType, provider)
		}

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
