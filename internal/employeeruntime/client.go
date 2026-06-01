package employeeruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

const defaultHTTPTimeout = 2 * time.Minute

type SyncResponse struct {
	Applied          int      `json:"applied"`
	Deleted          int      `json:"deleted"`
	ReposCloned      int      `json:"repos_cloned"`
	RestartTriggered bool     `json:"restart_triggered"`
	Errors           []string `json:"errors,omitempty"`
}

type ConfigUpdateRequest struct {
	RuntimeSecret string            `json:"runtime_secret,omitempty"`
	RuntimeEnv    map[string]string `json:"runtime_env,omitempty"`
	Definition    *AgentDefinition  `json:"definition"`
}

type HTTPMessageRequest struct {
	Text            string         `json:"text"`
	ConversationID  string         `json:"conversation_id,omitempty"`
	User            string         `json:"user,omitempty"`
	UserDisplayName string         `json:"user_display_name,omitempty"`
	Attachments     []any          `json:"attachments,omitempty"`
	Raw             map[string]any `json:"raw,omitempty"`
}

type HTTPMessageResponse struct {
	SessionID string `json:"session_id"`
	StreamID  string `json:"stream_id"`
	StreamURL string `json:"stream_url"`
	TraceID   string `json:"trace_id"`
	TurnID    string `json:"turn_id"`
}

func NewClient(baseURL, apiKey string) *Client {
	return NewClientWithTimeout(baseURL, apiKey, defaultHTTPTimeout)
}

func NewClientWithTimeout(baseURL, apiKey string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = defaultHTTPTimeout
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		http:    &http.Client{Timeout: timeout},
	}
}

func (c *Client) Healthz(ctx context.Context) error {
	resp, err := c.doRuntimeRequest(ctx, http.MethodGet, "/healthz", nil, false)
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
	return c.PutRuntimeConfig(ctx, ConfigUpdateRequest{Definition: def})
}

func (c *Client) PutRuntimeConfig(ctx context.Context, body ConfigUpdateRequest) (*SyncResponse, error) {
	if body.RuntimeEnv == nil {
		body.RuntimeEnv = map[string]string{}
	}
	if os.Getenv("HIVY_DEBUG_RUNTIME_CONFIG_PAYLOAD") == "true" {
		payload, err := json.Marshal(body)
		if err != nil {
			slog.WarnContext(ctx, "runtime config debug payload marshal failed", "error", err)
		} else {
			slog.InfoContext(ctx, "runtime config debug payload", "base_url", c.baseURL, "payload", string(payload))
		}
	}
	if path := strings.TrimSpace(os.Getenv("HIVY_DEBUG_RUNTIME_CONFIG_PAYLOAD_FILE")); path != "" {
		payload, err := json.MarshalIndent(body, "", "  ")
		if err != nil {
			slog.WarnContext(ctx, "runtime config debug payload file marshal failed", "error", err)
		} else if err := os.WriteFile(path, payload, 0o600); err != nil {
			slog.WarnContext(ctx, "runtime config debug payload file write failed", "path", path, "error", err)
		} else {
			slog.InfoContext(ctx, "runtime config debug payload written", "path", path, "bytes", len(payload), "base_url", c.baseURL)
		}
	}
	resp, err := c.do(ctx, http.MethodPut, "/config", body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("put config: %s: %s", resp.Status, body)
	}
	var out struct {
		EnvKeyCount int `json:"env_key_count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode config response: %w", err)
	}
	return &SyncResponse{Applied: 1, RestartTriggered: true}, nil
}

func (c *Client) PostHTTPMessage(ctx context.Context, body HTTPMessageRequest) (*HTTPMessageResponse, error) {
	resp, err := c.do(ctx, http.MethodPost, "/gateway/http/messages", body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("post http message: %s: %s", resp.Status, raw)
	}
	var out HTTPMessageResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode http message response: %w", err)
	}
	return &out, nil
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
	return c.doRuntimeRequest(ctx, method, path, body, true)
}

func (c *Client) doRuntimeRequest(ctx context.Context, method, path string, body any, auth bool) (*http.Response, error) {
	var data []byte
	if body != nil {
		var err error
		data, err = json.Marshal(body)
		if err != nil {
			return nil, err
		}
	}
	req, err := c.newRequest(ctx, method, c.baseURL+path, data, auth)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err == nil {
		return resp, nil
	}
	fallbackBase, ok := localDockerHostBaseURL(c.baseURL)
	if !ok {
		return nil, err
	}
	fallbackReq, fallbackReqErr := c.newRequest(ctx, method, fallbackBase+path, data, auth)
	if fallbackReqErr != nil {
		return nil, err
	}
	fallbackResp, fallbackErr := c.http.Do(fallbackReq)
	if fallbackErr != nil {
		return nil, err
	}
	return fallbackResp, nil
}

func (c *Client) newRequest(ctx context.Context, method, rawURL string, data []byte, auth bool) (*http.Request, error) {
	var reader io.Reader
	if data != nil {
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, reader)
	if err != nil {
		return nil, err
	}
	if auth {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	if data != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

func localDockerHostBaseURL(raw string) (string, bool) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() != "host.docker.internal" {
		return "", false
	}
	if port := parsed.Port(); port != "" {
		parsed.Host = net.JoinHostPort("localhost", port)
	} else {
		parsed.Host = "localhost"
	}
	return strings.TrimRight(parsed.String(), "/"), true
}
