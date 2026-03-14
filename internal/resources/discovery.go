// Package resources provides resource discovery for integration providers.
// It fetches available resources from provider APIs via Nango proxy.
package resources

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/llmvault/llmvault/internal/mcp/catalog"
	"github.com/llmvault/llmvault/internal/nango"
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
	ID       string                 `json:"id"`
	Name     string                 `json:"name"`
	Type     string                 `json:"type"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// DiscoveryResult holds the result of a resource discovery request.
type DiscoveryResult struct {
	Resources  []AvailableResource `json:"resources"`
	HasMore    bool                `json:"has_more"`
	NextCursor string              `json:"next_cursor,omitempty"`
}

// Discover fetches available resources of a specific type for a provider.
func (d *Discovery) Discover(
	ctx context.Context,
	provider, resourceType, nangoProviderConfigKey, nangoConnectionID string,
) (*DiscoveryResult, error) {
	// Get resource definition from catalog
	resDef, ok := d.catalog.GetResourceDef(provider, resourceType)
	if !ok {
		return nil, fmt.Errorf("resource type %q not configured for provider %q", resourceType, provider)
	}

	// Verify the list action exists (validates configuration)
	_, ok = d.catalog.GetAction(provider, resDef.ListAction)
	if !ok {
		return nil, fmt.Errorf("list action %q not found for provider %q", resDef.ListAction, provider)
	}

	slog.Debug("resource discovery",
		"provider", provider,
		"resource_type", resourceType,
		"list_action", resDef.ListAction,
	)

	// If RequestConfig is provided, use generic discovery
	if resDef.RequestConfig != nil {
		return d.discoverWithConfig(ctx, nangoProviderConfigKey, nangoConnectionID, resourceType, resDef)
	}

	// Route to provider-specific discovery logic for backward compatibility
	switch provider {
	case "slack":
		return d.discoverSlackChannels(ctx, nangoProviderConfigKey, nangoConnectionID, resDef)
	case "github":
		return d.discoverGitHubRepos(ctx, nangoProviderConfigKey, nangoConnectionID, resDef)
	case "google_drive":
		return d.discoverGoogleDriveFolders(ctx, nangoProviderConfigKey, nangoConnectionID, resDef)
	case "notion":
		return d.discoverNotionResources(ctx, nangoProviderConfigKey, nangoConnectionID, resourceType, resDef)
	case "linear":
		return d.discoverLinearTeams(ctx, nangoProviderConfigKey, nangoConnectionID, resDef)
	default:
		return nil, fmt.Errorf("resource discovery not implemented for provider %q", provider)
	}
}

// discoverWithConfig performs generic discovery using RequestConfig.
func (d *Discovery) discoverWithConfig(
	ctx context.Context,
	providerConfigKey, connectionID, resourceType string,
	resDef *catalog.ResourceDef,
) (*DiscoveryResult, error) {
	config := resDef.RequestConfig

	// Determine HTTP method (default to GET)
	method := http.MethodGet
	if config.Method != "" {
		method = config.Method
	}

	// Build query params from config
	queryParams := make(map[string]string)
	if config.QueryParams != nil {
		for k, v := range config.QueryParams {
			queryParams[k] = v
		}
	}

	// Build body from config
	var body map[string]interface{}
	if config.BodyTemplate != nil {
		body = make(map[string]interface{})
		for k, v := range config.BodyTemplate {
			body[k] = v
		}
	}

	// Build headers from config
	headers := make(map[string]string)
	if config.Headers != nil {
		for k, v := range config.Headers {
			headers[k] = v
		}
	}

	// Make the proxy request with headers
	resp, err := d.nango.ProxyRequestWithHeaders(ctx, method, providerConfigKey, connectionID,
		resDef.ListAction, queryParams, body, headers)
	if err != nil {
		return nil, fmt.Errorf("discovery request failed: %w", err)
	}

	// Extract data using response path if specified
	var data []interface{}
	if config.ResponsePath != "" {
		data = extractPath(resp, config.ResponsePath)
	} else {
		// Try common patterns
		if items, ok := resp["results"].([]interface{}); ok {
			data = items
		} else if items, ok := resp["items"].([]interface{}); ok {
			data = items
		} else if items, ok := resp["data"].([]interface{}); ok {
			data = items
		} else if items, ok := resp["channels"].([]interface{}); ok {
			data = items
		} else if items, ok := resp["files"].([]interface{}); ok {
			data = items
		} else if items, ok := resp["_raw"].([]interface{}); ok {
			// Direct array response
			data = items
		}
	}

	if data == nil {
		return nil, fmt.Errorf("could not extract data from response")
	}

	// Transform data into AvailableResource
	resources := make([]AvailableResource, 0, len(data))
	for _, item := range data {
		obj, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		resource := d.transformResource(obj, resourceType, resDef)
		if resource.ID != "" {
			resources = append(resources, resource)
		}
	}

	return &DiscoveryResult{
		Resources: resources,
		HasMore:   false,
	}, nil
}

// transformResource converts a provider object to AvailableResource.
func (d *Discovery) transformResource(obj map[string]interface{}, resourceType string, resDef *catalog.ResourceDef) AvailableResource {
	var id, name string

	// Extract ID using IDField or common patterns
	if resDef.IDField != "" {
		if v, ok := obj[resDef.IDField].(string); ok {
			id = v
		}
	}
	if id == "" {
		// Fallback patterns
		if v, ok := obj["id"].(string); ok {
			id = v
		} else if v, ok := obj["key"].(string); ok {
			id = v
		} else if v, ok := obj["full_name"].(string); ok {
			id = v
		}
	}

	// Extract name using NameField or common patterns
	if resDef.NameField != "" {
		if v, ok := obj[resDef.NameField].(string); ok {
			name = v
		}
	}
	if name == "" {
		// Try to extract name from various patterns
		name = extractName(obj, resourceType)
	}

	if name == "" {
		name = "Untitled"
	}

	// Build metadata from remaining fields
	metadata := make(map[string]interface{})
	for k, v := range obj {
		if k == resDef.IDField || k == resDef.NameField || k == "id" || k == "name" {
			continue
		}
		// Skip complex nested objects for now
		if _, isMap := v.(map[string]interface{}); !isMap {
			metadata[k] = v
		}
	}

	return AvailableResource{
		ID:       id,
		Name:     name,
		Type:     resourceType,
		Metadata: metadata,
	}
}

// extractName tries to extract a human-readable name from various provider formats.
func extractName(obj map[string]interface{}, resourceType string) string {
	// Try common name fields
	if v, ok := obj["name"].(string); ok && v != "" {
		return v
	}
	if v, ok := obj["title"].(string); ok && v != "" {
		return v
	}
	if v, ok := obj["name_normalized"].(string); ok && v != "" {
		return "#" + v
	}

	// Provider-specific extractions
	switch resourceType {
	case "page", "database":
		// Notion: extract from title array
		if title, ok := obj["title"].([]interface{}); ok && len(title) > 0 {
			if first, ok := title[0].(map[string]interface{}); ok {
				if text, ok := first["plain_text"].(string); ok {
					return text
				}
			}
		}
		// Try properties.Name.title for pages
		if properties, ok := obj["properties"].(map[string]interface{}); ok {
			if nameProp, ok := properties["Name"].(map[string]interface{}); ok {
				if title, ok := nameProp["title"].([]interface{}); ok && len(title) > 0 {
					if first, ok := title[0].(map[string]interface{}); ok {
						if text, ok := first["plain_text"].(string); ok {
							return text
						}
					}
				}
			}
		}
	case "team":
		// Linear: use key as suffix if available
		if name, ok := obj["name"].(string); ok {
			if key, ok := obj["key"].(string); ok && key != "" {
				return fmt.Sprintf("%s (%s)", name, key)
			}
			return name
		}
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
		case []interface{}:
			// If we hit an array before the end, we can't continue
			return nil
		default:
			return nil
		}
	}

	if arr, ok := current.([]interface{}); ok {
		return arr
	}
	return nil
}

// discoverSlackChannels discovers Slack channels using conversations.list API.
func (d *Discovery) discoverSlackChannels(ctx context.Context, providerConfigKey, connectionID string, resDef *catalog.ResourceDef) (*DiscoveryResult, error) {
	// Slack API: conversations.list
	// https://api.slack.com/methods/conversations.list
	queryParams := map[string]string{
		"types":            "public_channel,private_channel",
		"exclude_archived": "true",
		"limit":            "1000",
	}

	resp, err := d.nango.ProxyRequest(ctx, http.MethodGet, providerConfigKey, connectionID,
		"/conversations.list", queryParams, nil)
	if err != nil {
		return nil, fmt.Errorf("slack conversations.list failed: %w", err)
	}

	// Check for Slack API error
	if ok, _ := resp["ok"].(bool); !ok {
		errorMsg := "unknown error"
		if msg, ok := resp["error"].(string); ok {
			errorMsg = msg
		}
		return nil, fmt.Errorf("slack API error: %s", errorMsg)
	}

	channels, ok := resp["channels"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected slack response: missing channels array")
	}

	resources := make([]AvailableResource, 0, len(channels))
	for _, ch := range channels {
		channel, ok := ch.(map[string]interface{})
		if !ok {
			continue
		}

		id, _ := channel["id"].(string)
		name, _ := channel["name_normalized"].(string)
		if name == "" {
			name, _ = channel["name"].(string)
		}

		if id == "" || name == "" {
			continue
		}

		metadata := make(map[string]interface{})
		if isPrivate, ok := channel["is_private"].(bool); ok {
			metadata["is_private"] = isPrivate
		}
		if numMembers, ok := channel["num_members"].(float64); ok {
			metadata["num_members"] = int(numMembers)
		}
		if topic, ok := channel["topic"].(map[string]interface{}); ok {
			if value, ok := topic["value"].(string); ok && value != "" {
				metadata["topic"] = value
			}
		}
		if purpose, ok := channel["purpose"].(map[string]interface{}); ok {
			if value, ok := purpose["value"].(string); ok && value != "" {
				metadata["purpose"] = value
			}
		}

		resources = append(resources, AvailableResource{
			ID:       id,
			Name:     "#" + name,
			Type:     "channel",
			Metadata: metadata,
		})
	}

	return &DiscoveryResult{
		Resources: resources,
		HasMore:   false,
	}, nil
}

// discoverGitHubRepos discovers GitHub repositories using /user/repos API.
func (d *Discovery) discoverGitHubRepos(ctx context.Context, providerConfigKey, connectionID string, resDef *catalog.ResourceDef) (*DiscoveryResult, error) {
	// GitHub API: /user/repos (lists repos for authenticated user including private ones)
	// https://docs.github.com/en/rest/repos/repos#list-repositories-for-the-authenticated-user
	queryParams := map[string]string{
		"sort":      "updated",
		"direction": "desc",
		"per_page":  "100",
	}

	resp, err := d.nango.ProxyRequest(ctx, http.MethodGet, providerConfigKey, connectionID,
		"/user/repos", queryParams, nil)
	if err != nil {
		return nil, fmt.Errorf("github repos list failed: %w", err)
	}

	// GitHub returns an array directly, not an object
	repos, ok := resp["_raw"].([]interface{})
	if !ok {
		// Try to parse as object with data
		if data, ok := resp["data"].([]interface{}); ok {
			repos = data
		} else {
			// Response might be wrapped differently
			slog.Debug("github response format", "resp", resp)
			return nil, fmt.Errorf("unexpected github response format")
		}
	}

	resources := make([]AvailableResource, 0, len(repos))
	for _, r := range repos {
		repo, ok := r.(map[string]interface{})
		if !ok {
			continue
		}

		fullName, _ := repo["full_name"].(string)
		name, _ := repo["name"].(string)

		if fullName == "" {
			continue
		}

		metadata := make(map[string]interface{})
		if desc, ok := repo["description"].(string); ok && desc != "" {
			metadata["description"] = desc
		}
		if isPrivate, ok := repo["private"].(bool); ok {
			metadata["private"] = isPrivate
		}
		if language, ok := repo["language"].(string); ok && language != "" {
			metadata["language"] = language
		}
		if stars, ok := repo["stargazers_count"].(float64); ok {
			metadata["stars"] = int(stars)
		}
		if owner, ok := repo["owner"].(map[string]interface{}); ok {
			if login, ok := owner["login"].(string); ok {
				metadata["owner"] = login
			}
		}

		resources = append(resources, AvailableResource{
			ID:       fullName,
			Name:     name,
			Type:     "repo",
			Metadata: metadata,
		})
	}

	return &DiscoveryResult{
		Resources: resources,
		HasMore:   false,
	}, nil
}

// discoverGoogleDriveFolders discovers Google Drive folders using files.list API.
func (d *Discovery) discoverGoogleDriveFolders(ctx context.Context, providerConfigKey, connectionID string, resDef *catalog.ResourceDef) (*DiscoveryResult, error) {
	// Google Drive API: files.list with q parameter for folders only
	// https://developers.google.com/drive/api/v3/reference/files/list
	// mimeType for folders: application/vnd.google-apps.folder
	queryParams := map[string]string{
		"q":        "mimeType='application/vnd.google-apps.folder' and trashed=false",
		"spaces":   "drive",
		"fields":   "files(id,name,createdTime,modifiedTime,webViewLink)",
		"pageSize": "100",
	}

	resp, err := d.nango.ProxyRequest(ctx, http.MethodGet, providerConfigKey, connectionID,
		"/drive/v3/files", queryParams, nil)
	if err != nil {
		return nil, fmt.Errorf("google drive files.list failed: %w", err)
	}

	files, ok := resp["files"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected google drive response: missing files array")
	}

	resources := make([]AvailableResource, 0, len(files))
	for _, f := range files {
		file, ok := f.(map[string]interface{})
		if !ok {
			continue
		}

		id, _ := file["id"].(string)
		name, _ := file["name"].(string)

		if id == "" || name == "" {
			continue
		}

		metadata := make(map[string]interface{})
		if createdTime, ok := file["createdTime"].(string); ok {
			metadata["created_time"] = createdTime
		}
		if modifiedTime, ok := file["modifiedTime"].(string); ok {
			metadata["modified_time"] = modifiedTime
		}
		if webViewLink, ok := file["webViewLink"].(string); ok {
			metadata["web_view_link"] = webViewLink
		}

		resources = append(resources, AvailableResource{
			ID:       id,
			Name:     name,
			Type:     "folder",
			Metadata: metadata,
		})
	}

	return &DiscoveryResult{
		Resources: resources,
		HasMore:   false,
	}, nil
}

// discoverNotionResources discovers Notion pages or databases using the search API.
func (d *Discovery) discoverNotionResources(ctx context.Context, providerConfigKey, connectionID string, resourceType string, resDef *catalog.ResourceDef) (*DiscoveryResult, error) {
	// Notion API: POST /v1/search
	// https://developers.notion.com/reference/post-search

	var filterValue string
	if resourceType == "page" {
		filterValue = "page"
	} else if resourceType == "database" {
		filterValue = "database"
	} else {
		return nil, fmt.Errorf("unsupported notion resource type: %s", resourceType)
	}

	body := map[string]interface{}{
		"filter": map[string]string{
			"value":    filterValue,
			"property": "object",
		},
		"page_size": 100,
	}

	// Add required Notion-Version header
	headers := map[string]string{
		"Notion-Version": "2022-06-28",
	}

	resp, err := d.nango.ProxyRequestWithHeaders(ctx, http.MethodPost, providerConfigKey, connectionID,
		"/v1/search", nil, body, headers)
	if err != nil {
		return nil, fmt.Errorf("notion search failed: %w", err)
	}

	results, ok := resp["results"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected notion response: missing results array")
	}

	hasMore, _ := resp["has_more"].(bool)
	nextCursor, _ := resp["next_cursor"].(string)

	resources := make([]AvailableResource, 0, len(results))
	for _, r := range results {
		item, ok := r.(map[string]interface{})
		if !ok {
			continue
		}

		id, _ := item["id"].(string)
		if id == "" {
			continue
		}

		// Extract name/title based on resource type
		var name string
		var metadata = make(map[string]interface{})

		if resourceType == "database" {
			// Database title is in "title" array
			if title, ok := item["title"].([]interface{}); ok && len(title) > 0 {
				if firstTitle, ok := title[0].(map[string]interface{}); ok {
					if plainText, ok := firstTitle["plain_text"].(string); ok {
						name = plainText
					}
				}
			}
			if url, ok := item["url"].(string); ok {
				metadata["url"] = url
			}
		} else {
			// Page title is in properties.Name.title
			if properties, ok := item["properties"].(map[string]interface{}); ok {
				if nameProp, ok := properties["Name"].(map[string]interface{}); ok {
					if title, ok := nameProp["title"].([]interface{}); ok && len(title) > 0 {
						if firstTitle, ok := title[0].(map[string]interface{}); ok {
							if plainText, ok := firstTitle["plain_text"].(string); ok {
								name = plainText
							}
						}
					}
				}
			}
			if url, ok := item["url"].(string); ok {
				metadata["url"] = url
			}
		}

		if name == "" {
			name = "Untitled"
		}

		if createdTime, ok := item["created_time"].(string); ok {
			metadata["created_time"] = createdTime
		}
		if lastEditedTime, ok := item["last_edited_time"].(string); ok {
			metadata["last_edited_time"] = lastEditedTime
		}

		resources = append(resources, AvailableResource{
			ID:       id,
			Name:     name,
			Type:     resourceType,
			Metadata: metadata,
		})
	}

	return &DiscoveryResult{
		Resources:  resources,
		HasMore:    hasMore,
		NextCursor: nextCursor,
	}, nil
}

// discoverLinearTeams discovers Linear teams using GraphQL API.
func (d *Discovery) discoverLinearTeams(ctx context.Context, providerConfigKey, connectionID string, resDef *catalog.ResourceDef) (*DiscoveryResult, error) {
	// Linear API: GraphQL query to list teams
	// https://studio.apollographql.com/public/Linear-API/variant/current/schema/reference/objects/Team

	body := map[string]interface{}{
		"query": "{ teams { nodes { id name key description } } }",
	}

	resp, err := d.nango.ProxyRequest(ctx, http.MethodPost, providerConfigKey, connectionID,
		"/graphql", nil, body)
	if err != nil {
		return nil, fmt.Errorf("linear graphql query failed: %w", err)
	}

	// Linear returns data nested under "data" key
	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		// Check for errors
		if errors, ok := resp["errors"].([]interface{}); ok && len(errors) > 0 {
			if firstError, ok := errors[0].(map[string]interface{}); ok {
				if msg, ok := firstError["message"].(string); ok {
					return nil, fmt.Errorf("linear API error: %s", msg)
				}
			}
		}
		return nil, fmt.Errorf("unexpected linear response: missing data")
	}

	teamsData, ok := data["teams"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected linear response: missing teams")
	}

	nodes, ok := teamsData["nodes"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected linear response: missing teams.nodes")
	}

	resources := make([]AvailableResource, 0, len(nodes))
	for _, n := range nodes {
		team, ok := n.(map[string]interface{})
		if !ok {
			continue
		}

		id, _ := team["id"].(string)
		name, _ := team["name"].(string)
		key, _ := team["key"].(string)

		if id == "" || name == "" {
			continue
		}

		metadata := make(map[string]interface{})
		if key != "" {
			metadata["key"] = key
		}
		if description, ok := team["description"].(string); ok && description != "" {
			metadata["description"] = description
		}

		displayName := name
		if key != "" {
			displayName = fmt.Sprintf("%s (%s)", name, key)
		}

		resources = append(resources, AvailableResource{
			ID:       id,
			Name:     displayName,
			Type:     "team",
			Metadata: metadata,
		})
	}

	return &DiscoveryResult{
		Resources: resources,
		HasMore:   false,
	}, nil
}

// HasDiscovery returns true if the provider has resource discovery implemented.
func (d *Discovery) HasDiscovery(provider string) bool {
	switch provider {
	case "slack", "github", "google_drive", "notion", "linear":
		return true
	default:
		return false
	}
}

// ListDiscoverableProviders returns a list of providers that support resource discovery.
func (d *Discovery) ListDiscoverableProviders() []string {
	return []string{"slack", "github", "google_drive", "notion", "linear"}
}

// ExtractResourceID extracts the resource ID from various formats.
// This is a helper for handling different ID formats across providers.
func ExtractResourceID(input string) string {
	// For most providers, the ID is used directly.
	// This function can be extended to handle special cases.
	return strings.TrimSpace(input)
}
