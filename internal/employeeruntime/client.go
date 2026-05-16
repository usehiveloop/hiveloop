package employeeruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

type SyncResponse struct {
	Applied          int      `json:"applied"`
	Deleted          int      `json:"deleted"`
	ReposCloned      int      `json:"repos_cloned"`
	RestartTriggered bool     `json:"restart_triggered"`
	Errors           []string `json:"errors,omitempty"`
}

func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) Healthz(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/healthz", nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("healthz: %s", resp.Status)
	}
	return nil
}

func (c *Client) Readyz(ctx context.Context) error {
	return c.doVoid(ctx, http.MethodGet, "/readyz", nil)
}

func (c *Client) GetConfig(ctx context.Context) (*AgentDefinition, error) {
	resp, err := c.do(ctx, http.MethodGet, "/config", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get config: %s: %s", resp.Status, body)
	}
	var out AgentDefinition
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}
	return &out, nil
}

func (c *Client) PutConfig(ctx context.Context, def *AgentDefinition) (*SyncResponse, error) {
	resp, err := c.do(ctx, http.MethodPut, "/config", def)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("put config: %s: %s", resp.Status, body)
	}
	return &SyncResponse{Applied: 1, RestartTriggered: true}, nil
}

func (c *Client) UpdateRuntimeEnv(ctx context.Context, env map[string]string) (*SyncResponse, error) {
	body := map[string]string{}
	if env != nil {
		body = env
	}
	resp, err := c.do(ctx, http.MethodPut, "/config/env", body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("put runtime env: %s: %s", resp.Status, raw)
	}
	var out struct {
		KeyCount int `json:"key_count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode runtime env response: %w", err)
	}
	return &SyncResponse{Applied: out.KeyCount}, nil
}

func (c *Client) doVoid(ctx context.Context, method, path string, body any) error {
	resp, err := c.do(ctx, method, path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s %s: %s: %s", method, path, resp.Status, raw)
	}
	return nil
}

func (c *Client) do(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.http.Do(req)
}
