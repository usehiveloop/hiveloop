// Package resources provides resource discovery for integration providers.
// It fetches available resources from provider APIs via Nango proxy.
package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/ziraloop/ziraloop/internal/mcp/catalog"
	"github.com/ziraloop/ziraloop/internal/nango"
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
	logger := slog.With(
		"component", "resource_discovery",
		"provider", provider,
		"resource_type", resourceType,
		"nango_connection_id", nangoConnectionID,
		"nango_provider_config_key", nangoProviderConfigKey,
	)
	logger.Info("starting resource discovery")

	// Get resource definition from catalog
	resDef, ok := d.catalog.GetResourceDef(provider, resourceType)
	if !ok {
		logger.Error("resource definition not found in catalog",
			"error", "resource type not configured for provider",
		)
		return nil, fmt.Errorf("resource type %q not configured for provider %q", resourceType, provider)
	}

	logger = logger.With(
		"id_field", resDef.IDField,
		"name_field", resDef.NameField,
		"list_action", resDef.ListAction,
	)
	logger.Info("resource definition found",
		"display_name", resDef.DisplayName,
		"has_request_config", resDef.RequestConfig != nil,
	)

	// Build the request configuration
	method := http.MethodGet
	queryParams := make(map[string]string)
	var body map[string]interface{}
	headers := make(map[string]string)

	if resDef.RequestConfig != nil {
		// Use configured method
		if resDef.RequestConfig.Method != "" {
			method = resDef.RequestConfig.Method
		}

		// Use configured headers
		if resDef.RequestConfig.Headers != nil {
			for k, v := range resDef.RequestConfig.Headers {
				headers[k] = v
			}
		}

		// Use configured query params
		if resDef.RequestConfig.QueryParams != nil {
			for k, v := range resDef.RequestConfig.QueryParams {
				queryParams[k] = v
			}
		}

		// Use configured body template
		if resDef.RequestConfig.BodyTemplate != nil {
			body = make(map[string]interface{})
			for k, v := range resDef.RequestConfig.BodyTemplate {
				body[k] = v
			}
		}

		logger = logger.With(
			"request_method", method,
			"response_path", resDef.RequestConfig.ResponsePath,
			"header_count", len(headers),
			"query_param_count", len(queryParams),
			"has_body", body != nil,
			"body_map_empty", body != nil && len(body) == 0,
		)
	} else {
		logger.Info("no request_config found, using defaults (GET, no body)")
	}

	// Log the exact body content for debugging
	if body != nil {
		bodyJSON, _ := json.Marshal(body)
		logger.Debug("request body content", "body_json", string(bodyJSON), "body_len", len(body))
	} else {
		logger.Debug("request body is nil")
	}

	// IMPORTANT: Never send body for GET requests
	if method == http.MethodGet && body != nil {
		logger.Warn("GET request with non-nil body detected, forcing body to nil", "original_body", body)
		body = nil
	}

	// Make the proxy request
	logger.Info("making nango proxy request",
		"action_path", resDef.ListAction,
		"final_method", method,
		"body_is_nil", body == nil,
		"body_type", fmt.Sprintf("%T", body),
	)

	resp, err := d.nango.ProxyRequestWithHeaders(ctx, method, nangoProviderConfigKey, nangoConnectionID,
		resDef.ListAction, queryParams, body, headers)
	if err != nil {
		logger.Error("nango proxy request failed",
			"error", err.Error(),
		)
		return nil, fmt.Errorf("discovery request failed: %w", err)
	}

	// Dump the full Nango response for debugging
	respJSON, _ := json.MarshalIndent(resp, "", "  ")
	logger.Info("nango proxy response dump",
		"response_keys", getMapKeys(resp),
		"response_body", string(respJSON),
	)

	// Extract the data array from response
	var data []interface{}

	// Use response path if configured
	if resDef.RequestConfig != nil && resDef.RequestConfig.ResponsePath != "" {
		logger.Debug("extracting data using response_path",
			"path", resDef.RequestConfig.ResponsePath,
		)
		data = extractPath(resp, resDef.RequestConfig.ResponsePath)
		if data == nil {
			logger.Error("failed to extract data using response_path",
				"path", resDef.RequestConfig.ResponsePath,
				"available_keys", getMapKeys(resp),
			)
		}
	} else {
		// Empty response_path means direct array response (e.g., GitHub)
		logger.Debug("no response_path configured, checking for direct array response")
		if arr, ok := resp["_raw"].([]interface{}); ok {
			data = arr
			logger.Info("extracted data from _raw array",
				"count", len(data),
			)
		} else {
			logger.Error("no _raw array found in response",
				"response_keys", getMapKeys(resp),
			)
		}
	}

	if data == nil {
		return nil, fmt.Errorf("could not extract data: response_path not configured or _raw array not found")
	}

	logger.Info("extracted raw data items",
		"total_count", len(data),
	)

	// Transform into standardized AvailableResource format using configured fields only
	resources := make([]AvailableResource, 0, len(data))
	skippedCount := 0
	for i, item := range data {
		obj, ok := item.(map[string]interface{})
		if !ok {
			logger.Warn("skipping non-object item",
				"index", i,
				"type", fmt.Sprintf("%T", item),
			)
			skippedCount++
			continue
		}

		resource := extractResource(obj, resourceType, resDef)

		if resource.ID == "" {
			logger.Warn("skipping item with empty ID",
				"index", i,
				"configured_id_field", resDef.IDField,
				"available_fields", getMapKeys(obj),
			)
			skippedCount++
			continue
		}

		if resource.Name == "" {
			logger.Warn("skipping item with empty name",
				"index", i,
				"id", resource.ID,
				"configured_name_field", resDef.NameField,
				"available_fields", getMapKeys(obj),
			)
			skippedCount++
			continue
		}

		resources = append(resources, resource)
	}

	logger.Info("resource discovery completed",
		"total_raw_items", len(data),
		"valid_resources", len(resources),
		"skipped_items", skippedCount,
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

// getMapKeys returns a slice of keys from a map for debugging.
func getMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// HasDiscovery returns true if the provider has resource discovery configured.
func (d *Discovery) HasDiscovery(provider string) bool {
	hasResources := d.catalog.HasConfigurableResources(provider)
	slog.Debug("checking discovery availability",
		"provider", provider,
		"has_discovery", hasResources,
	)
	return hasResources
}
