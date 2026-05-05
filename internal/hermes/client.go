// Package hermes wraps the upstream Hermes sidecar SDK so the rest of the
// codebase consumes a stable internal type. The sidecar runs as PID 1 inside
// every Hermes-harness sandbox and exposes its control surface at
// /v1/* on the sandbox's published port. The control plane talks to it via
// this client to push agent config, manage skills/memory/hooks, restart the
// supervised hermes process, and so on.
package hermes

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	hsdk "github.com/usehiveloop/hermes/pkg/sdk"
)

type Client struct{ inner *hsdk.ClientWithResponses }

func New(sandboxURL, apiKey string) (*Client, error) {
	c, err := hsdk.New(sandboxURL, apiKey, hsdk.Options{
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
	})
	if err != nil {
		return nil, fmt.Errorf("hermes sdk init: %w", err)
	}
	return &Client{inner: c}, nil
}

func (c *Client) Raw() *hsdk.ClientWithResponses { return c.inner }

func (c *Client) Healthz(ctx context.Context) error {
	resp, err := c.inner.HealthzWithResponse(ctx)
	if err != nil {
		return fmt.Errorf("healthz: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return fmt.Errorf("healthz: %s", resp.Status())
	}
	return nil
}

func (c *Client) HermesStatus(ctx context.Context) (*hsdk.HermesStatus, error) {
	resp, err := c.inner.HermesStatusWithResponse(ctx)
	if err != nil {
		return nil, fmt.Errorf("hermes status: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, fmt.Errorf("hermes status: %s", resp.Status())
	}
	return resp.JSON200, nil
}

func (c *Client) SyncConfig(ctx context.Context, req hsdk.SyncRequest) (*hsdk.SyncResponse, error) {
	resp, err := c.inner.SyncConfigWithResponse(ctx, hsdk.SyncConfigJSONRequestBody(req))
	if err != nil {
		return nil, fmt.Errorf("sync config: %w", err)
	}
	if resp.JSON200 != nil {
		return resp.JSON200, nil
	}
	if resp.HTTPResponse != nil && resp.HTTPResponse.StatusCode == http.StatusOK {
		var out hsdk.SyncResponse
		if jerr := json.Unmarshal(resp.Body, &out); jerr == nil {
			return &out, nil
		}
	}
	body := ""
	if len(resp.Body) > 0 {
		body = ": " + string(resp.Body)
	}
	return nil, fmt.Errorf("sync config: %s%s", resp.Status(), body)
}
