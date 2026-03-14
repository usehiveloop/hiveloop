package nango

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
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
	Name               string `json:"name"`
	DisplayName        string `json:"display_name"`
	AuthMode           string `json:"auth_mode"`
	ClientRegistration string `json:"client_registration,omitempty"` // for MCP_OAUTH2
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

	catalog := make(map[string]Provider, len(providers))
	for _, p := range providers {
		catalog[p.Name] = p
	}

	// Also store raw templates for config extraction
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
// GET /integrations/{uniqueKey}?include=webhook
func (c *Client) GetIntegration(ctx context.Context, uniqueKey string) (map[string]any, error) {
	return c.doJSON(ctx, http.MethodGet, "/integrations/"+uniqueKey+"?include=webhook", nil)
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
