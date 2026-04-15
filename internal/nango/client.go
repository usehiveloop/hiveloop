package nango

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"time"
)

// Client wraps the Nango API for managing integrations.
// Authenticated via a secret key (UUID v4) with Bearer token auth.
type Client struct {
	endpoint   string
	secretKey  string
	httpClient *http.Client
	mu         sync.RWMutex
	providers  map[string]Provider        // cached provider catalog
	templates  map[string]map[string]any  // raw provider templates for config extraction
}

// Provider represents a Nango integration provider from the catalog.
type Provider struct {
	Name                    string `json:"name"`
	DisplayName             string `json:"display_name"`
	AuthMode                string `json:"auth_mode"`
	ClientRegistration      string `json:"client_registration,omitempty"` // for MCP_OAUTH2
	WebhookUserDefinedSecret bool  `json:"webhook_user_defined_secret,omitempty"`
}

// Credentials is a union type covering all Nango auth modes.
// Only fields relevant to the auth mode should be populated.
type Credentials struct {
	Type          string `json:"type"`
	ClientID      string `json:"client_id,omitempty"`
	ClientSecret  string `json:"client_secret,omitempty"`
	Scopes        string `json:"scopes,omitempty"`
	AppID         string `json:"app_id,omitempty"`
	AppLink       string `json:"app_link,omitempty"`
	PrivateKey    string `json:"private_key,omitempty"`
	WebhookSecret string `json:"webhook_secret,omitempty"`
	// MCP_OAUTH2_GENERIC fields
	ClientName    string `json:"client_name,omitempty"`
	ClientUri     string `json:"client_uri,omitempty"`
	ClientLogoUri string `json:"client_logo_uri,omitempty"`
	// INSTALL_PLUGIN fields
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

// ConnectSessionEndUser identifies the end user for a Nango connect session.
type ConnectSessionEndUser struct {
	ID string `json:"id"`
}

// CreateConnectSessionRequest is the payload for creating a connect session in Nango.
type CreateConnectSessionRequest struct {
	EndUser             ConnectSessionEndUser `json:"end_user"`
	AllowedIntegrations []string              `json:"allowed_integrations,omitempty"`
}

// ConnectSessionResponse represents the response from creating a connect session.
type ConnectSessionResponse struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
}

// CreateIntegrationRequest is the payload for creating an integration in Nango.
type CreateIntegrationRequest struct {
	UniqueKey   string       `json:"unique_key"`
	Provider    string       `json:"provider"`
	DisplayName string       `json:"display_name,omitempty"`
	Credentials *Credentials `json:"credentials,omitempty"`
}

// UpdateIntegrationRequest is the payload for updating an integration in Nango.
type UpdateIntegrationRequest struct {
	DisplayName string       `json:"display_name,omitempty"`
	Credentials *Credentials `json:"credentials,omitempty"`
}

// NewClient creates a Nango API client.
func NewClient(endpoint, secretKey string) *Client {
	return &Client{
		endpoint:   endpoint,
		secretKey:  secretKey,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		providers:  make(map[string]Provider),
		templates:  make(map[string]map[string]any),
	}
}

// FetchProviders fetches the full provider catalog from Nango and caches it.
// Called on startup and can be called periodically to refresh.
func (c *Client) FetchProviders(ctx context.Context) error {
	resp, err := c.doJSON(ctx, http.MethodGet, "/providers", nil)
	if err != nil {
		return fmt.Errorf("fetching providers: %w", err)
	}

	// Nango returns {"data": [...]} for the providers list
	rawData, ok := resp["data"]
	if !ok {
		return fmt.Errorf("unexpected provider response: missing 'data' key")
	}

	// Re-marshal and unmarshal to get typed providers
	b, err := json.Marshal(rawData)
	if err != nil {
		return fmt.Errorf("marshaling provider data: %w", err)
	}

	var providers []Provider
	if err := json.Unmarshal(b, &providers); err != nil {
		return fmt.Errorf("unmarshaling providers: %w", err)
	}

	// Also store raw templates for config extraction (need these before building catalog)
	var rawProviders []map[string]any
	if err := json.Unmarshal(b, &rawProviders); err != nil {
		return fmt.Errorf("unmarshaling raw provider templates: %w", err)
	}
	templates := make(map[string]map[string]any, len(rawProviders))
	for _, rp := range rawProviders {
		if name, ok := rp["name"].(string); ok {
			templates[name] = rp
		}
	}

	// Build catalog with webhook_user_defined_secret populated from templates
	catalog := make(map[string]Provider, len(providers))
	for _, p := range providers {
		if tmpl, ok := templates[p.Name]; ok {
			if wuds, ok := tmpl["webhook_user_defined_secret"].(bool); ok {
				p.WebhookUserDefinedSecret = wuds
			}
		}
		catalog[p.Name] = p
	}

	c.mu.Lock()
	c.providers = catalog
	c.templates = templates
	c.mu.Unlock()

	slog.Info("nango providers fetched", "count", len(catalog))
	return nil
}

// GetProvider returns a cached provider by name. Thread-safe.
func (c *Client) GetProvider(name string) (Provider, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	p, ok := c.providers[name]
	return p, ok
}

// GetProviderTemplate returns the raw provider template for config extraction.
func (c *Client) GetProviderTemplate(name string) (map[string]any, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	t, ok := c.templates[name]
	return t, ok
}

// CallbackURL returns the Nango OAuth callback URL.
func (c *Client) CallbackURL() string {
	return c.endpoint + "/oauth/callback"
}

// GetProviders returns all cached providers as a slice.
func (c *Client) GetProviders() []Provider {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([]Provider, 0, len(c.providers))
	for _, p := range c.providers {
		result = append(result, p)
	}
	return result
}

// CreateIntegration creates an integration in Nango.
// POST /integrations
func (c *Client) CreateIntegration(ctx context.Context, req CreateIntegrationRequest) error {
	slog.Info("nango: creating integration", "unique_key", req.UniqueKey, "provider", req.Provider)
	_, err := c.doJSON(ctx, http.MethodPost, "/integrations", req)
	if err != nil {
		slog.Error("nango: create integration failed", "error", err, "unique_key", req.UniqueKey)
		return err
	}
	slog.Info("nango: integration created", "unique_key", req.UniqueKey, "provider", req.Provider)
	return nil
}

// UpdateIntegration updates an existing integration in Nango.
// PATCH /integrations/{uniqueKey}
func (c *Client) UpdateIntegration(ctx context.Context, uniqueKey string, req UpdateIntegrationRequest) error {
	slog.Info("nango: updating integration", "unique_key", uniqueKey)
	_, err := c.doJSON(ctx, http.MethodPatch, "/integrations/"+uniqueKey, req)
	if err != nil {
		slog.Error("nango: update integration failed", "error", err, "unique_key", uniqueKey)
		return err
	}
	slog.Info("nango: integration updated", "unique_key", uniqueKey)
	return nil
}

// GetIntegration fetches an integration by its unique key.
// GET /integrations/{uniqueKey}?include[]=webhook&include[]=credentials
func (c *Client) GetIntegration(ctx context.Context, uniqueKey string) (map[string]any, error) {
	return c.doJSON(ctx, http.MethodGet, "/integrations/"+uniqueKey+"?include[]=webhook&include[]=credentials", nil)
}

// DeleteIntegration removes an integration by its unique key.
// DELETE /integrations/{uniqueKey}
func (c *Client) DeleteIntegration(ctx context.Context, uniqueKey string) error {
	slog.Info("nango: deleting integration", "unique_key", uniqueKey)
	_, err := c.doJSON(ctx, http.MethodDelete, "/integrations/"+uniqueKey, nil)
	if err != nil {
		slog.Error("nango: delete integration failed", "error", err, "unique_key", uniqueKey)
		return err
	}
	slog.Info("nango: integration deleted", "unique_key", uniqueKey)
	return nil
}

// DeleteConnection removes a connection by its ID.
// DELETE /connection/{connectionId}?provider_config_key={providerConfigKey}
func (c *Client) DeleteConnection(ctx context.Context, connectionID, providerConfigKey string) error {
	slog.Info("nango: deleting connection", "connection_id", connectionID, "provider_config_key", providerConfigKey)
	path := fmt.Sprintf("/connection/%s?provider_config_key=%s", connectionID, providerConfigKey)
	_, err := c.doJSON(ctx, http.MethodDelete, path, nil)
	if err != nil {
		slog.Error("nango: delete connection failed", "error", err, "connection_id", connectionID)
		return err
	}
	slog.Info("nango: connection deleted", "connection_id", connectionID)
	return nil
}

// CreateConnectionRequest is the payload for creating a connection directly in Nango.
type CreateConnectionRequest struct {
	ProviderConfigKey string `json:"provider_config_key"`
	ConnectionID      string `json:"connection_id"`
	APIKey            string `json:"api_key,omitempty"`
}

// CreateConnection creates a connection directly in Nango (e.g. for API_KEY auth mode).
// POST /connection
func (c *Client) CreateConnection(ctx context.Context, req CreateConnectionRequest) error {
	slog.Info("nango: creating connection", "connection_id", req.ConnectionID, "provider_config_key", req.ProviderConfigKey)
	_, err := c.doJSON(ctx, http.MethodPost, "/connection", req)
	if err != nil {
		slog.Error("nango: create connection failed", "error", err, "connection_id", req.ConnectionID)
		return err
	}
	slog.Info("nango: connection created", "connection_id", req.ConnectionID)
	return nil
}

// GetConnection retrieves a connection from Nango.
// GET /connection/{connectionId}?provider_config_key={providerConfigKey}
func (c *Client) GetConnection(ctx context.Context, connectionID, providerConfigKey string) (map[string]any, error) {
	path := fmt.Sprintf("/connection/%s?provider_config_key=%s", connectionID, providerConfigKey)
	return c.doJSON(ctx, http.MethodGet, path, nil)
}

// CreateConnectSession creates a Nango connect session.
// POST /connect/sessions
func (c *Client) CreateConnectSession(ctx context.Context, req CreateConnectSessionRequest) (*ConnectSessionResponse, error) {
	slog.Info("nango: creating connect session", "allowed_integrations", req.AllowedIntegrations)
	resp, err := c.doJSON(ctx, http.MethodPost, "/connect/sessions", req)
	if err != nil {
		slog.Error("nango: create connect session failed", "error", err)
		return nil, err
	}

	// Nango returns {"data": {"token": "...", "expires_at": "..."}}
	data, ok := resp["data"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("unexpected connect session response: missing 'data' key")
	}

	token, _ := data["token"].(string)
	expiresAt, _ := data["expires_at"].(string)
	if token == "" {
		return nil, fmt.Errorf("unexpected connect session response: missing 'token'")
	}

	slog.Info("nango: connect session created")
	return &ConnectSessionResponse{Token: token, ExpiresAt: expiresAt}, nil
}

// CreateReconnectSessionRequest is the payload for reconnecting an existing connection.
type CreateReconnectSessionRequest struct {
	ConnectionID  string `json:"connection_id"`
	IntegrationID string `json:"integration_id"`
}

// CreateReconnectSession creates a Nango reconnect session for an existing connection.
// POST /connect/sessions/reconnect
func (c *Client) CreateReconnectSession(ctx context.Context, req CreateReconnectSessionRequest) (*ConnectSessionResponse, error) {
	slog.Info("nango: creating reconnect session", "connection_id", req.ConnectionID, "integration_id", req.IntegrationID)
	resp, err := c.doJSON(ctx, http.MethodPost, "/connect/sessions/reconnect", req)
	if err != nil {
		slog.Error("nango: create reconnect session failed", "error", err)
		return nil, err
	}

	data, ok := resp["data"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("unexpected reconnect session response: missing 'data' key")
	}

	token, _ := data["token"].(string)
	expiresAt, _ := data["expires_at"].(string)
	if token == "" {
		return nil, fmt.Errorf("unexpected reconnect session response: missing 'token'")
	}

	slog.Info("nango: reconnect session created")
	return &ConnectSessionResponse{Token: token, ExpiresAt: expiresAt}, nil
}

// ProxyRequest makes a request through Nango's proxy to the provider's API.
// This allows making authenticated requests to the provider's API using the stored credentials.
func (c *Client) ProxyRequest(ctx context.Context, method, providerConfigKey, connectionID, path string, queryParams map[string]string, body any) (map[string]any, error) {
	return c.ProxyRequestWithHeaders(ctx, method, providerConfigKey, connectionID, path, queryParams, body, nil)
}

// ProxyRequestWithHeaders makes a request through Nango's proxy to the provider's API with custom headers.
// This allows making authenticated requests with provider-specific headers (e.g., Notion-Version).
func (c *Client) ProxyRequestWithHeaders(ctx context.Context, method, providerConfigKey, connectionID, path string, queryParams map[string]string, body any, headers map[string]string) (map[string]any, error) {
	logger := slog.With(
		"component", "nango_client",
		"method", method,
		"provider_config_key", providerConfigKey,
		"connection_id", connectionID,
		"path", path,
		"query_param_count", len(queryParams),
		"custom_header_count", len(headers),
	)

	// Log query params at debug level (may contain sensitive data)
	if len(queryParams) > 0 {
		logger.Debug("proxy request with query params", "query_params", queryParams)
	}

	var bodyReader io.Reader
	var bodySize int
	var bodyContent string
	bodyType := fmt.Sprintf("%T", body)
	
	// Use reflection to check if body is effectively nil or empty
	// This handles typed nils (map[string]interface{}(nil)) and empty collections
	isEmptyBody := body == nil
	if !isEmptyBody {
		v := reflect.ValueOf(body)
		switch v.Kind() {
		case reflect.Ptr, reflect.Slice, reflect.Map:
			isEmptyBody = v.IsNil() || v.Len() == 0
		default:
			isEmptyBody = v.IsZero()
		}
	}
	
	logger.Debug("processing request body", "body_type", bodyType, "is_empty_body", isEmptyBody)
	
	if !isEmptyBody {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			logger.Error("failed to marshal request body", "error", err.Error(), "body_type", bodyType)
			return nil, fmt.Errorf("marshaling proxy request body: %w", err)
		}
		bodyReader = bytes.NewReader(jsonBody)
		bodySize = len(jsonBody)
		bodyContent = string(jsonBody)
		logger.Debug("request body prepared", 
			"body_size_bytes", bodySize, 
			"body_content", bodyContent,
			"body_type", bodyType,
		)
	} else {
		logger.Debug("body is nil or empty, not sending body")
	}

	// Build query string
	query := ""
	if len(queryParams) > 0 {
		q := make([]string, 0, len(queryParams))
		for k, v := range queryParams {
			q = append(q, fmt.Sprintf("%s=%s", k, v))
		}
		query = "?" + strings.Join(q, "&")
	}

	fullURL := c.endpoint + "/proxy" + path + query
	logger.Info("sending proxy request to provider API",
		"url", fullURL,
		"has_body", !isEmptyBody,
		"body_type", bodyType,
		"body_size", bodySize,
		"body_content_preview", truncate(bodyContent, 500),
	)

	req, err := http.NewRequestWithContext(ctx, method, fullURL, bodyReader)
	if err != nil {
		logger.Error("failed to create HTTP request", "error", err.Error())
		return nil, err
	}

	if !isEmptyBody {
		req.Header.Set("Content-Type", "application/json")
		logger.Debug("set Content-Type header to application/json")
	} else {
		logger.Debug("no body, not setting Content-Type")
	}

	// Set custom headers for the provider
	customHeaderKeys := make([]string, 0, len(headers))
	for k, v := range headers {
		req.Header.Set(k, v)
		customHeaderKeys = append(customHeaderKeys, k)
	}
	if len(customHeaderKeys) > 0 {
		logger.Debug("set custom provider headers", "header_keys", customHeaderKeys)
	}

	// Set Nango proxy headers
	req.Header.Set("Authorization", "Bearer "+c.secretKey)
	req.Header.Set("Provider-Config-Key", providerConfigKey)
	req.Header.Set("Connection-Id", connectionID)
	
	logger.Debug("request headers set",
		"authorization_set", c.secretKey != "",
		"provider_config_key_set", providerConfigKey != "",
		"connection_id_set", connectionID != "",
	)

	// Dump full outgoing request for debugging
	reqHeaders := make(map[string]string)
	for k, v := range req.Header {
		if k == "Authorization" {
			reqHeaders[k] = "Bearer [REDACTED]"
		} else {
			reqHeaders[k] = strings.Join(v, ", ")
		}
	}
	reqHeadersJSON, _ := json.Marshal(reqHeaders)
	logger.Info("full nango proxy request dump",
		"method", method,
		"url", fullURL,
		"headers", string(reqHeadersJSON),
		"body", bodyContent,
	)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		logger.Error("HTTP request failed", "error", err.Error())
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error("failed to read response body", "error", err.Error())
		return nil, err
	}

	logger = logger.With("status_code", resp.StatusCode, "response_size_bytes", len(respBody))

	// Log response headers at debug level
	responseContentType := resp.Header.Get("Content-Type")
	if responseContentType != "" {
		logger = logger.With("content_type", responseContentType)
	}

	if resp.StatusCode >= 400 {
		logger.Error("provider API returned error status",
			"response_body_preview", truncate(string(respBody), 500),
		)
		return nil, fmt.Errorf("nango proxy error %d: %s", resp.StatusCode, string(respBody))
	}

	logger.Info("provider API request successful",
		"response_body", truncate(string(respBody), 5000),
	)

	if len(respBody) == 0 {
		logger.Debug("empty response body")
		return nil, nil
	}

	// Try to parse as JSON
	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		logger.Info("response is not valid JSON, returning as raw string",
			"parse_error", err.Error(),
			"response_preview", truncate(string(respBody), 200),
		)
		// Return raw response if not JSON
		return map[string]any{"_raw": string(respBody)}, nil
	}

	// Log parsed response structure
	topLevelKeys := make([]string, 0, len(result))
	for k := range result {
		topLevelKeys = append(topLevelKeys, k)
	}
	logger.Debug("parsed JSON response", "top_level_keys", topLevelKeys)

	return result, nil
}

// RawProxyResponse holds the raw HTTP response from a Nango proxy request.
type RawProxyResponse struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

// RawProxyRequest makes a request through Nango's proxy and returns the raw response
// without parsing JSON or treating 4xx as errors. Only transport failures return an error.
func (c *Client) RawProxyRequest(ctx context.Context, method, providerConfigKey, connectionID, path, rawQuery string, body io.Reader, contentType string) (*RawProxyResponse, error) {
	fullURL := c.endpoint + "/proxy" + path
	if rawQuery != "" {
		fullURL += "?" + rawQuery
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, body)
	if err != nil {
		return nil, err
	}

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("Authorization", "Bearer "+c.secretKey)
	req.Header.Set("Provider-Config-Key", providerConfigKey)
	req.Header.Set("Connection-Id", connectionID)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return &RawProxyResponse{
		StatusCode: resp.StatusCode,
		Header:     resp.Header,
		Body:       respBody,
	}, nil
}

// truncate truncates a string to maxLen characters, adding "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func (c *Client) doJSON(ctx context.Context, method, path string, payload any) (map[string]any, error) {
	slog.Debug("nango: request", "method", method, "path", path)

	var bodyReader io.Reader
	if payload != nil {
		jsonBody, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.endpoint+path, bodyReader)
	if err != nil {
		return nil, err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+c.secretKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	slog.Debug("nango: response", "method", method, "path", path, "status", resp.StatusCode)

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("nango API error %d: %s", resp.StatusCode, string(respBody))
	}

	if len(respBody) == 0 || (respBody[0] != '{' && respBody[0] != '[') {
		return nil, nil
	}

	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, nil
	}
	return result, nil
}
