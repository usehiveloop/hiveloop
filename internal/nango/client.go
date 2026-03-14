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
	providers  map[string]Provider // cached provider catalog
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

	c.mu.Lock()
	c.providers = catalog
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
// GET /integrations/{uniqueKey}
func (c *Client) GetIntegration(ctx context.Context, uniqueKey string) (map[string]any, error) {
	return c.doJSON(ctx, http.MethodGet, "/integrations/"+uniqueKey, nil)
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
