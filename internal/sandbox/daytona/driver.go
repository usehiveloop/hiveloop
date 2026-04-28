package daytona

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	daytona "github.com/daytonaio/daytona/libs/sdk-go/pkg/daytona"
	"github.com/daytonaio/daytona/libs/sdk-go/pkg/types"

	"github.com/usehiveloop/hiveloop/internal/sandbox"
)

const signedURLTTLSeconds = 3600

type Config struct {
	APIURL string
	APIKey string
	Target string
}

type Driver struct {
	client *daytona.Client
	apiURL string
	apiKey string
}

func NewDriver(cfg Config) (*Driver, error) {
	client, err := daytona.NewClientWithConfig(&types.DaytonaConfig{
		APIKey: cfg.APIKey,
		APIUrl: cfg.APIURL,
		Target: cfg.Target,
	})
	if err != nil {
		return nil, fmt.Errorf("creating daytona client: %w", err)
	}
	apiURL := cfg.APIURL
	if apiURL == "" {
		apiURL = "https://app.daytona.io/api"
	}
	return &Driver{client: client, apiURL: apiURL, apiKey: cfg.APIKey}, nil
}

func (d *Driver) CreateSandbox(ctx context.Context, opts sandbox.CreateSandboxOpts) (*sandbox.SandboxInfo, error) {
	envVars := make(map[string]string)
	for k, v := range opts.EnvVars {
		envVars[k] = v
	}

	body := map[string]any{
		"name":   opts.Name,
		"env":    envVars,
		"labels": opts.Labels,
		"public": false,
	}
	if opts.SnapshotID != "" {
		body["snapshot"] = opts.SnapshotID
	} else {
		body["image"] = "hiveloop/bridge:latest"
	}

	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.apiURL+"/sandbox", bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+d.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("creating sandbox: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("creating sandbox (status %d): %s", resp.StatusCode, respBody)
	}

	var created struct {
		ID    string `json:"id"`
		State string `json:"state"`
	}
	if err := json.Unmarshal(respBody, &created); err != nil {
		return nil, fmt.Errorf("parsing sandbox response: %w", err)
	}

	if err := d.waitForStarted(ctx, created.ID, 3*time.Minute); err != nil {
		return nil, fmt.Errorf("waiting for sandbox: %w", err)
	}

	return &sandbox.SandboxInfo{
		ExternalID: created.ID,
		Status:     sandbox.StatusRunning,
	}, nil
}

func (d *Driver) waitForStarted(ctx context.Context, sandboxID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	attempt := 0

	for time.Now().Before(deadline) {
		attempt++
		status, err := d.GetStatus(ctx, sandboxID)
		if err == nil && status == sandbox.StatusRunning {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}

	return fmt.Errorf("sandbox %s did not reach started state within %s (%d attempts)", sandboxID, timeout, attempt)
}

func (d *Driver) StartSandbox(ctx context.Context, externalID string) error {
	return d.sandboxAction(ctx, externalID, "start")
}

func (d *Driver) StopSandbox(ctx context.Context, externalID string) error {
	return d.sandboxAction(ctx, externalID, "stop")
}

func (d *Driver) ArchiveSandbox(ctx context.Context, externalID string) error {
	return d.sandboxAction(ctx, externalID, "archive")
}

func (d *Driver) DeleteSandbox(ctx context.Context, externalID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, d.apiURL+"/sandbox/"+externalID, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+d.apiKey)
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return fmt.Errorf("deleting sandbox: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return sandbox.ErrSandboxNotFound
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete sandbox failed (status %d): %s", resp.StatusCode, body)
	}
	return nil
}

func (d *Driver) GetStatus(ctx context.Context, externalID string) (sandbox.SandboxStatus, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.apiURL+"/sandbox/"+externalID, nil)
	if err != nil {
		return sandbox.StatusError, err
	}
	req.Header.Set("Authorization", "Bearer "+d.apiKey)
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return sandbox.StatusError, fmt.Errorf("getting sandbox status: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return sandbox.StatusError, fmt.Errorf("get sandbox status failed (status %d)", resp.StatusCode)
	}
	var result struct {
		State string `json:"state"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	return mapState(result.State), nil
}

func (d *Driver) sandboxAction(ctx context.Context, externalID, action string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.apiURL+"/sandbox/"+externalID+"/"+action, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+d.apiKey)
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return fmt.Errorf("%s sandbox: %w", action, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return sandbox.ErrSandboxNotFound
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s sandbox failed (status %d): %s", action, resp.StatusCode, body)
	}
	return nil
}
