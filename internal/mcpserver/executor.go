// Package mcpserver provides dynamic MCP server construction from token scopes.
package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/llmvault/llmvault/internal/mcp/catalog"
	"github.com/llmvault/llmvault/internal/nango"
)

// ExecuteAction runs a catalog action against a provider via Nango proxy.
// It maps user-supplied params to the correct HTTP method, path, query/body
// using the action's ExecutionConfig, then proxies through Nango.
func ExecuteAction(
	ctx context.Context,
	nangoClient *nango.Client,
	provider string,
	providerCfgKey string,
	nangoConnID string,
	action *catalog.ActionDef,
	params map[string]any,
	allowedResources map[string][]string,
) (map[string]any, error) {
	exec := action.Execution
	if exec == nil {
		return nil, fmt.Errorf("action has no execution config")
	}

	// Validate resource access if action has a resource_type
	if action.ResourceType != "" && allowedResources != nil {
		allowed, ok := allowedResources[action.ResourceType]
		if ok && len(allowed) > 0 {
			// Find the resource param — it's the param matching the resource_type
			resourceParam := findResourceParam(action.ResourceType, params)
			if resourceParam != "" && !contains(allowed, resourceParam) {
				return nil, fmt.Errorf("access denied: resource %q not in allowed set for type %q", resourceParam, action.ResourceType)
			}
		}
	}

	// Build the API path, substituting {param} placeholders
	path := exec.Path
	for paramName, paramVal := range params {
		placeholder := "{" + paramName + "}"
		if strings.Contains(path, placeholder) {
			path = strings.ReplaceAll(path, placeholder, fmt.Sprintf("%v", paramVal))
		}
	}

	// Build query params from QueryMapping
	var queryParams map[string]string
	if len(exec.QueryMapping) > 0 {
		queryParams = make(map[string]string)
		for queryKey, paramName := range exec.QueryMapping {
			if val, ok := params[paramName]; ok {
				queryParams[queryKey] = fmt.Sprintf("%v", val)
			}
		}
	}

	// Build body from BodyMapping
	var body map[string]any
	if len(exec.BodyMapping) > 0 {
		body = make(map[string]any)
		for bodyKey, paramName := range exec.BodyMapping {
			if val, ok := params[paramName]; ok {
				body[bodyKey] = val
			}
		}
	}

	// Execute via Nango proxy
	result, err := nangoClient.ProxyRequestWithHeaders(
		ctx,
		exec.Method,
		providerCfgKey,
		nangoConnID,
		path,
		queryParams,
		body,
		exec.Headers,
	)
	if err != nil {
		return nil, fmt.Errorf("provider API error: %w", err)
	}

	// Extract nested data if ResponsePath is set
	if exec.ResponsePath != "" && result != nil {
		extracted := extractPath(result, exec.ResponsePath)
		if extracted != nil {
			return map[string]any{"data": extracted}, nil
		}
	}

	return result, nil
}

// findResourceParam tries to find the resource ID from the params.
// Heuristic: look for a param name matching the resource type or common patterns.
func findResourceParam(resourceType string, params map[string]any) string {
	// Direct match (e.g., "channel" param for "channel" resource type)
	if val, ok := params[resourceType]; ok {
		return fmt.Sprintf("%v", val)
	}
	// Try common suffixes
	for _, suffix := range []string{"_id", "Id"} {
		if val, ok := params[resourceType+suffix]; ok {
			return fmt.Sprintf("%v", val)
		}
	}
	return ""
}

// extractPath navigates a dot-separated path in a nested map.
func extractPath(data map[string]any, path string) any {
	parts := strings.Split(path, ".")
	var current any = data
	for _, part := range parts {
		m, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current, ok = m[part]
		if !ok {
			return nil
		}
	}
	return current
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
