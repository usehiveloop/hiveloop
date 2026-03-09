package zitadel

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Client wraps the ZITADEL Management and Admin APIs for programmatic
// organization and user management.
type Client struct {
	baseURL    string
	pat        string
	httpClient *http.Client
}

// NewClient creates a ZITADEL API client authenticated with a Personal Access Token.
func NewClient(baseURL, pat string) *Client {
	return &Client{
		baseURL:    baseURL,
		pat:        pat,
		httpClient: &http.Client{},
	}
}

// CreateOrganization creates a new organization in ZITADEL via the Management API.
func (c *Client) CreateOrganization(name string) (string, error) {
	body := map[string]string{"name": name}
	resp, err := c.doJSON("POST", "/management/v1/orgs", body)
	if err != nil {
		return "", fmt.Errorf("creating org: %w", err)
	}
	orgID, _ := resp["id"].(string)
	if orgID == "" {
		return "", fmt.Errorf("no org ID in response: %v", resp)
	}
	return orgID, nil
}

// CreateMachineUser creates a machine user in the specified organization.
func (c *Client) CreateMachineUser(orgID, username, name string) (string, error) {
	body := map[string]any{
		"userName": username,
		"name":     name,
	}
	resp, err := c.doJSONWithOrg("POST", "/management/v1/users/machine", orgID, body)
	if err != nil {
		return "", fmt.Errorf("creating machine user: %w", err)
	}
	userID, _ := resp["userId"].(string)
	if userID == "" {
		return "", fmt.Errorf("no user ID in response: %v", resp)
	}
	return userID, nil
}

// CreatePAT generates a Personal Access Token for a user.
func (c *Client) CreatePAT(orgID, userID string) (string, error) {
	body := map[string]string{
		"expirationDate": "2099-01-01T00:00:00Z",
	}
	resp, err := c.doJSONWithOrg("POST", fmt.Sprintf("/management/v1/users/%s/pats", userID), orgID, body)
	if err != nil {
		return "", fmt.Errorf("creating PAT: %w", err)
	}
	token, _ := resp["token"].(string)
	if token == "" {
		return "", fmt.Errorf("no token in response: %v", resp)
	}
	return token, nil
}

// GrantProjectRoles grants project roles to a user in a specific organization.
func (c *Client) GrantProjectRoles(orgID, userID, projectID string, roles []string) error {
	body := map[string]any{
		"projectId": projectID,
		"roleKeys":  roles,
	}
	_, err := c.doJSONWithOrg("POST", fmt.Sprintf("/management/v1/users/%s/grants", userID), orgID, body)
	if err != nil {
		return fmt.Errorf("granting roles: %w", err)
	}
	return nil
}

// GrantProjectToOrg grants a project to another organization so its users can receive roles.
func (c *Client) GrantProjectToOrg(projectID, grantedOrgID string, roleKeys []string) (string, error) {
	body := map[string]any{
		"grantedOrgId": grantedOrgID,
		"roleKeys":     roleKeys,
	}
	resp, err := c.doJSON("POST", fmt.Sprintf("/management/v1/projects/%s/grants", projectID), body)
	if err != nil {
		return "", fmt.Errorf("granting project to org: %w", err)
	}
	grantID, _ := resp["grantId"].(string)
	return grantID, nil
}

// AddOrgMember adds a user as a member of an organization with the given roles.
// Returns nil if the user is already a member (409 conflict).
func (c *Client) AddOrgMember(orgID, userID string, roles []string) error {
	body := map[string]any{
		"userId": userID,
		"roles":  roles,
	}
	_, err := c.doJSONWithOrg("POST", "/management/v1/orgs/me/members", orgID, body)
	if err != nil {
		// The org creator is automatically a member; ignore "already exists".
		if strings.Contains(err.Error(), "AlreadyExists") {
			return nil
		}
		return fmt.Errorf("adding org member: %w", err)
	}
	return nil
}

func (c *Client) doJSON(method, path string, body any) (map[string]any, error) {
	return c.doJSONWithOrg(method, path, "", body)
}

func (c *Client) doJSONWithOrg(method, path, orgID string, body any) (map[string]any, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(method, c.baseURL+path, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.pat)
	req.Host = "localhost"
	if orgID != "" {
		req.Header.Set("x-zitadel-orgid", orgID)
	}

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
		return nil, fmt.Errorf("ZITADEL API error %d: %s", resp.StatusCode, string(respBody))
	}

	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parsing response: %w (body: %s)", err, string(respBody))
	}
	return result, nil
}
