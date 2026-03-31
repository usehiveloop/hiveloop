package turso

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const baseURL = "https://api.turso.tech/v1"

// Client communicates with the Turso Platform API for database management.
type Client struct {
	apiToken string
	orgSlug  string
	baseURL  string
	http     *http.Client
}

// NewClient creates a Turso Platform API client.
func NewClient(apiToken, orgSlug string) *Client {
	return &Client{
		apiToken: apiToken,
		orgSlug:  orgSlug,
		baseURL:  baseURL,
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SetBaseURL overrides the base URL (for testing).
func (c *Client) SetBaseURL(url string) {
	c.baseURL = url
}

func (c *Client) do(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshaling request: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.http.Do(req)
}

// Database represents a Turso database.
type Database struct {
	Name     string `json:"Name"`
	DbID     string `json:"DbId"`
	Hostname string `json:"Hostname"`
}

// CreateDatabase creates a new Turso database in the configured org.
func (c *Client) CreateDatabase(ctx context.Context, name, group string) (*Database, error) {
	payload := struct {
		Name  string `json:"name"`
		Group string `json:"group"`
	}{Name: name, Group: group}

	resp, err := c.do(ctx, http.MethodPost, "/organizations/"+c.orgSlug+"/databases", payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("turso create database failed (status %d): %s", resp.StatusCode, b)
	}

	var result struct {
		Database Database `json:"database"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &result.Database, nil
}

// GetDatabase retrieves a database by name.
func (c *Client) GetDatabase(ctx context.Context, name string) (*Database, error) {
	resp, err := c.do(ctx, http.MethodGet, "/organizations/"+c.orgSlug+"/databases/"+name, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("turso get database failed (status %d): %s", resp.StatusCode, b)
	}

	var result struct {
		Database Database `json:"database"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &result.Database, nil
}

// DeleteDatabase deletes a database by name.
func (c *Client) DeleteDatabase(ctx context.Context, name string) error {
	resp, err := c.do(ctx, http.MethodDelete, "/organizations/"+c.orgSlug+"/databases/"+name, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("turso delete database failed (status %d): %s", resp.StatusCode, b)
	}
	return nil
}

// CreateToken mints an auth token for a specific database.
// The token is used by Bridge's BRIDGE_STORAGE_AUTH_TOKEN env var.
func (c *Client) CreateToken(ctx context.Context, dbName string) (string, error) {
	resp, err := c.do(ctx, http.MethodPost, "/organizations/"+c.orgSlug+"/databases/"+dbName+"/auth/tokens", nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("turso create token failed (status %d): %s", resp.StatusCode, b)
	}

	var result struct {
		JWT string `json:"jwt"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding response: %w", err)
	}
	return result.JWT, nil
}

// DatabaseURL returns the libsql URL for a database given its hostname.
func DatabaseURL(hostname string) string {
	return "libsql://" + hostname
}
