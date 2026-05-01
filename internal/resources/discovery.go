// Package resources provides resource discovery for integration providers.
// It fetches available resources from provider APIs via Nango proxy.
package resources

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/mcp/catalog"
	"github.com/usehiveloop/hiveloop/internal/nango"
)

// Discovery handles resource discovery for providers.
type Discovery struct {
	catalog *catalog.Catalog
	nango   *nango.Client
}

// NewDiscovery creates a new resource discovery handler.
func NewDiscovery(cat *catalog.Catalog, nangoClient *nango.Client) *Discovery {
	return &Discovery{
		catalog: cat,
		nango:   nangoClient,
	}
}

// AvailableResource represents a resource that can be selected.
type AvailableResource struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// DiscoveryResult holds the result of a resource discovery request.
type DiscoveryResult struct {
	Resources []AvailableResource `json:"resources"`
}

// Discover fetches available resources of a specific type for a provider.
// This is a fully generic implementation that uses actions.json configuration.
func (d *Discovery) Discover(
	ctx context.Context,
	provider, resourceType, nangoProviderConfigKey, nangoConnectionID string,
) (*DiscoveryResult, error) {
	ctx = logging.WithAttrs(ctx,
		"component", "resource_discovery",
		"provider", provider,
		"resource_type", resourceType,
		"nango_connection_id", nangoConnectionID,
	)

	// Get resource definition from catalog
	resDef, ok := d.catalog.GetResourceDef(provider, resourceType)
	if !ok {
		return nil, fmt.Errorf("resource type %q not configured for provider %q", resourceType, provider)
	}

	// Build the request configuration
	method := http.MethodGet
	queryParams := make(map[string]string)
	var body map[string]interface{}
	headers := make(map[string]string)

	if resDef.RequestConfig != nil {
		if resDef.RequestConfig.Method != "" {
			method = resDef.RequestConfig.Method
		}
		for k, v := range resDef.RequestConfig.Headers {
			headers[k] = v
		}
		for k, v := range resDef.RequestConfig.QueryParams {
			queryParams[k] = v
		}
		if resDef.RequestConfig.BodyTemplate != nil {
			body = make(map[string]interface{}, len(resDef.RequestConfig.BodyTemplate))
			for k, v := range resDef.RequestConfig.BodyTemplate {
				body[k] = v
			}
		}
	}

	// IMPORTANT: Never send body for GET requests
	if method == http.MethodGet {
		body = nil
	}

	resp, err := d.nango.ProxyRequestWithHeaders(ctx, method, nangoProviderConfigKey, nangoConnectionID,
		resDef.ListAction, queryParams, body, headers)
	if err != nil {
		return nil, fmt.Errorf("discovery request failed: %w", err)
	}

	// Extract the data array from response
	var data []interface{}
	if resDef.RequestConfig != nil && resDef.RequestConfig.ResponsePath != "" {
		data = extractPath(resp, resDef.RequestConfig.ResponsePath)
	} else if arr, ok := resp["_raw"].([]interface{}); ok {
		// Empty response_path means direct array response (e.g., GitHub)
		data = arr
	}

	if data == nil {
		return nil, fmt.Errorf("could not extract data: response_path not configured or _raw array not found")
	}

	// Transform into standardized AvailableResource format using configured fields only
	resources := make([]AvailableResource, 0, len(data))
	for _, item := range data {
		obj, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		resource := extractResource(obj, resourceType, resDef)
		if resource.ID == "" || resource.Name == "" {
			continue
		}
		resources = append(resources, resource)
	}

	logging.FromContext(ctx).Info("resource discovery completed",
		"total_raw_items", len(data),
		"valid_resources", len(resources),
	)

	return &DiscoveryResult{
		Resources: resources,
	}, nil
}

// extractResource extracts a standardized AvailableResource using configured fields only.
func extractResource(obj map[string]interface{}, resourceType string, resDef *catalog.ResourceDef) AvailableResource {
	// Extract ID using configured IDField only
	id := ""
	if resDef.IDField != "" {
		id = extractString(obj, resDef.IDField)
	}

	// Extract name using configured NameField only
	name := ""
	if resDef.NameField != "" {
		name = extractString(obj, resDef.NameField)
	}

	return AvailableResource{
		ID:   id,
		Name: name,
		Type: resourceType,
	}
}

// extractString safely extracts a string value from a map.
func extractString(obj map[string]interface{}, key string) string {
	if val, ok := obj[key].(string); ok {
		return val
	}
	return ""
}

// extractPath extracts data from a nested path like "data.teams.nodes".
func extractPath(data map[string]interface{}, path string) []interface{} {
	parts := strings.Split(path, ".")
	var current interface{} = data

	for _, part := range parts {
		if current == nil {
			return nil
		}

		switch v := current.(type) {
		case map[string]interface{}:
			current = v[part]
		default:
			return nil
		}
	}

	if arr, ok := current.([]interface{}); ok {
		return arr
	}
	return nil
}

// HasDiscovery returns true if the provider has resource discovery configured.
func (d *Discovery) HasDiscovery(provider string) bool {
	return d.catalog.HasConfigurableResources(provider)
}
