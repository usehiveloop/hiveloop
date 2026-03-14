package logto

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// Client wraps the Logto Management API for programmatic organization
// and user management, authenticated via M2M client credentials.
type Client struct {
	endpoint   string // e.g. http://localhost:3301
	appID      string
	appSecret  string
	httpClient *http.Client

	mu          sync.Mutex
	accessToken string
	expiresAt   time.Time

	mgmtOnce     sync.Once
	mgmtResource string
}

// NewClient creates a Logto Management API client authenticated with
// M2M client credentials (client_credentials grant).
func NewClient(endpoint, appID, appSecret string) *Client {
	return &Client{
		endpoint:   endpoint,
		appID:      appID,
		appSecret:  appSecret,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// token returns a valid Management API access token, refreshing if needed.
func (c *Client) token() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.accessToken != "" && time.Now().Before(c.expiresAt) {
		return c.accessToken, nil
	}

	// Logto Management API uses the default tenant resource indicator.
	// We discover it by fetching the well-known resource list.
	mgmtResource, err := c.discoverManagementResource()
	if err != nil {
		return "", fmt.Errorf("discovering management API resource: %w", err)
	}

	data := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {c.appID},
		"client_secret": {c.appSecret},
		"resource":      {mgmtResource},
		"scope":         {"all"},
	}

	resp, err := c.httpClient.PostForm(c.endpoint+"/oidc/token", data)
	if err != nil {
		return "", fmt.Errorf("requesting M2M token: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("M2M token error %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("parsing token response: %w", err)
	}

	c.accessToken = tokenResp.AccessToken
	c.expiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn-60) * time.Second) // refresh 60s early
	return c.accessToken, nil
}

// CreateOrganization creates a new organization in Logto.
func (c *Client) CreateOrganization(name string) (string, error) {
	body := map[string]string{"name": name}
	resp, err := c.doJSON("POST", "/api/organizations", body)
	if err != nil {
		return "", fmt.Errorf("creating org: %w", err)
	}
	orgID, _ := resp["id"].(string)
	if orgID == "" {
		return "", fmt.Errorf("no org ID in response: %v", resp)
	}
	return orgID, nil
}

// AddOrgMember adds a user to an organization.
func (c *Client) AddOrgMember(orgID, userID string) error {
	body := map[string][]string{"userIds": {userID}}
	_, err := c.doJSON("POST", fmt.Sprintf("/api/organizations/%s/users", orgID), body)
	if err != nil {
		return fmt.Errorf("adding org member: %w", err)
	}
	return nil
}

// AddOrgMemberM2M adds an M2M application to an organization.
func (c *Client) AddOrgMemberM2M(orgID, appID string) error {
	body := map[string][]string{"applicationIds": {appID}}
	_, err := c.doJSON("POST", fmt.Sprintf("/api/organizations/%s/applications", orgID), body)
	if err != nil {
		return fmt.Errorf("adding M2M app to org: %w", err)
	}
	return nil
}

// AssignOrgRoleToUser assigns organization roles to a user within an org.
func (c *Client) AssignOrgRoleToUser(orgID, userID string, roleIDs []string) error {
	body := map[string][]string{"organizationRoleIds": roleIDs}
	_, err := c.doJSON("POST", fmt.Sprintf("/api/organizations/%s/users/%s/roles", orgID, userID), body)
	if err != nil {
		return fmt.Errorf("assigning org roles: %w", err)
	}
	return nil
}

// AssignOrgRoleToM2M assigns organization roles to an M2M app within an org.
func (c *Client) AssignOrgRoleToM2M(orgID, appID string, roleIDs []string) error {
	body := map[string][]string{"organizationRoleIds": roleIDs}
	_, err := c.doJSON("POST", fmt.Sprintf("/api/organizations/%s/applications/%s/roles", orgID, appID), body)
	if err != nil {
		return fmt.Errorf("assigning org roles to M2M: %w", err)
	}
	return nil
}

// GetOrgRoleByName looks up an organization role by name and returns its ID.
func (c *Client) GetOrgRoleByName(name string) (string, error) {
	tok, err := c.token()
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("GET", c.endpoint+"/api/organization-roles", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+tok)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("listing org roles: %d %s", resp.StatusCode, string(body))
	}

	var roles []map[string]any
	if err := json.Unmarshal(body, &roles); err != nil {
		return "", err
	}
	for _, r := range roles {
		if r["name"] == name {
			return r["id"].(string), nil
		}
	}
	return "", fmt.Errorf("org role %q not found", name)
}

// GetM2MOrgToken gets an organization-scoped access token for an M2M app.
func (c *Client) GetM2MOrgToken(appID, appSecret, orgID, resource string, scopes []string) (string, error) {
	scope := ""
	for i, s := range scopes {
		if i > 0 {
			scope += " "
		}
		scope += s
	}

	data := url.Values{
		"grant_type":      {"client_credentials"},
		"client_id":       {appID},
		"client_secret":   {appSecret},
		"resource":        {resource},
		"organization_id": {orgID},
		"scope":           {scope},
	}

	resp, err := c.httpClient.PostForm(c.endpoint+"/oidc/token", data)
	if err != nil {
		return "", fmt.Errorf("requesting org-scoped token: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("org token error %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("parsing org token response: %w", err)
	}
	return tokenResp.AccessToken, nil
}

// GetM2MToken gets an access token for an M2M app (non-org-scoped).
func (c *Client) GetM2MToken(appID, appSecret, resource string) (string, error) {
	data := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {appID},
		"client_secret": {appSecret},
		"resource":      {resource},
	}

	resp, err := c.httpClient.PostForm(c.endpoint+"/oidc/token", data)
	if err != nil {
		return "", fmt.Errorf("requesting M2M token: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("M2M token error %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("parsing token response: %w", err)
	}
	return tokenResp.AccessToken, nil
}

// discoverManagementResource finds the Logto Management API resource indicator
// by querying the well-known OIDC configuration. For self-hosted Logto, the
// management API resource is always "https://default.logto.app/api".
func (c *Client) discoverManagementResource() (string, error) {
	c.mgmtOnce.Do(func() {
		// Standard Logto management API resource for self-hosted instances
		c.mgmtResource = "https://default.logto.app/api"
	})
	return c.mgmtResource, nil
}

func (c *Client) doJSON(method, path string, payload any) (map[string]any, error) {
	tok, err := c.token()
	if err != nil {
		return nil, err
	}

	jsonBody, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(method, c.endpoint+path, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("Logto API error %d: %s", resp.StatusCode, string(respBody))
	}

	// Some endpoints return 201/204 with no body or non-JSON body
	if len(respBody) == 0 || respBody[0] != '{' {
		return nil, nil
	}

	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, nil // Non-JSON success response — treat as ok
	}
	return result, nil
}
