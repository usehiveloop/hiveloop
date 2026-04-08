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
	"github.com/daytonaio/daytona/libs/sdk-go/pkg/types" // used for snapshot creation

	"github.com/ziraloop/ziraloop/internal/sandbox"
)

const (
	// signedURLTTLSeconds is how long signed preview URLs are valid.
	// We request 3600s (1 hour) and refresh at 55 minutes in the orchestrator.
	signedURLTTLSeconds = 3600
)

// Config holds Daytona-specific configuration.
type Config struct {
	APIURL string
	APIKey string
	Target string
}

// Driver implements sandbox.Provider using Daytona.
type Driver struct {
	client *daytona.Client
	apiURL string
	apiKey string
}

// NewDriver creates a new Daytona sandbox provider.
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

// CreateSandbox creates a new Daytona sandbox via REST API and polls until started.
// Uses raw HTTP instead of SDK to handle self-hosted Daytona instances correctly.
func (d *Driver) CreateSandbox(ctx context.Context, opts sandbox.CreateSandboxOpts) (*sandbox.SandboxInfo, error) {
	// Build request body
	// We inject a startup script via env var that the entrypoint will pick up
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
		body["image"] = "ziraloop/bridge:latest"
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

	// Poll until started (max 3 minutes)
	if err := d.waitForStarted(ctx, created.ID, 3*time.Minute); err != nil {
		return nil, fmt.Errorf("waiting for sandbox: %w", err)
	}

	return &sandbox.SandboxInfo{
		ExternalID: created.ID,
		Status:     sandbox.StatusRunning,
	}, nil
}

// waitForStarted polls the sandbox status until it reaches "started".
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

// StartSandbox starts a stopped Daytona sandbox.
func (d *Driver) StartSandbox(ctx context.Context, externalID string) error {
	return d.sandboxAction(ctx, externalID, "start")
}

// StopSandbox stops a running Daytona sandbox.
func (d *Driver) StopSandbox(ctx context.Context, externalID string) error {
	return d.sandboxAction(ctx, externalID, "stop")
}

// DeleteSandbox permanently removes a Daytona sandbox.
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
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete sandbox failed (status %d): %s", resp.StatusCode, body)
	}
	return nil
}

// GetStatus returns the current status of a Daytona sandbox.
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
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s sandbox failed (status %d): %s", action, resp.StatusCode, body)
	}
	return nil
}

// GetEndpoint returns a signed, time-limited URL to reach a specific port inside the sandbox.
// Uses Daytona's signed preview URL API for private sandboxes.
func (d *Driver) GetEndpoint(ctx context.Context, externalID string, port int) (string, error) {
	url := fmt.Sprintf("%s/sandbox/%s/ports/%d/signed-preview-url?expiresInSeconds=%d",
		d.apiURL, externalID, port, signedURLTTLSeconds)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("creating signed URL request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+d.apiKey)

	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return "", fmt.Errorf("requesting signed URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("signed URL request failed (status %d): %s", resp.StatusCode, body)
	}

	var result struct {
		URL   string `json:"url"`
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding signed URL response: %w", err)
	}

	return result.URL, nil
}

// BuildSnapshot creates a new Daytona snapshot with Bridge + customer build commands.
// Builds a full Docker image: Ubuntu base → system packages → Bridge binary → customer commands → Bridge entrypoint.
// Returns the snapshot name. The build runs asynchronously — caller should poll until ready.
func (d *Driver) BuildSnapshot(ctx context.Context, opts sandbox.BuildSnapshotOpts) (string, error) {
	baseImage := opts.BaseImage
	if baseImage == "" {
		baseImage = "ubuntu:24.04"
	}

	// Build image with Bridge + customer commands
	image := daytona.Base(baseImage)

	// System packages
	image = image.AptGet([]string{"curl", "ca-certificates", "git", "jq", "unzip", "wget", "openssh-client"})

	// GitHub CLI
	image = image.Run(
		"curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg | dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg && " +
			`echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" | tee /etc/apt/sources.list.d/github-cli.list > /dev/null && ` +
			"apt-get update && apt-get install -y --no-install-recommends gh && rm -rf /var/lib/apt/lists/*",
	)

	// Bridge binary + storage directory
	image = image.Run("mkdir -p /home/daytona/.bridge")
	image = image.Run(
		`curl -fsSL "https://github.com/ziraloop/bridge/releases/download/v0.17.1/bridge-v0.17.1-x86_64-unknown-linux-gnu.tar.gz" | tar -xzf - -C /usr/local/bin && chmod +x /usr/local/bin/bridge`,
	)

	// CodeDB binary (code intelligence for agents)
	image = image.Run(
		`curl -fsSL -o /usr/local/bin/codedb "https://github.com/justrach/codedb/releases/download/v0.2.54/codedb-linux-x86_64" && chmod +x /usr/local/bin/codedb`,
	)

	// Customer build commands
	if opts.BuildCommands != "" {
		image = image.Run(opts.BuildCommands)
	}

	// Working directory + entrypoint
	image = image.Workdir("/home/daytona")
	image = image.Entrypoint([]string{"/bin/sh", "-c", "mkdir -p /home/daytona/.bridge && /usr/local/bin/bridge >> /tmp/bridge.log 2>&1"})

	snapshot, logChan, err := d.client.Snapshot.Create(ctx, &types.CreateSnapshotParams{
		Name:  opts.Name,
		Image: image,
	})
	if err != nil {
		return "", fmt.Errorf("creating snapshot: %w", err)
	}

	// Drain build logs (non-blocking — just consume them)
	if logChan != nil {
		go func() {
			for range logChan {
			}
		}()
	}

	return snapshot.Name, nil
}

// DeleteSnapshot removes a Daytona snapshot.
func (d *Driver) DeleteSnapshot(ctx context.Context, externalID string) error {
	snapshot, err := d.client.Snapshot.Get(ctx, externalID)
	if err != nil {
		return fmt.Errorf("getting snapshot %s: %w", externalID, err)
	}
	return d.client.Snapshot.Delete(ctx, snapshot)
}

// SetAutoStop configures auto-stop interval for a sandbox.
func (d *Driver) SetAutoStop(ctx context.Context, externalID string, intervalMinutes int) error {
	url := fmt.Sprintf("%s/sandbox/%s/autostop/%d", d.apiURL, externalID, intervalMinutes)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+d.apiKey)
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// ExecuteCommand runs a shell command inside the sandbox via the Daytona API
// server's toolbox proxy. This routes through the API server rather than the
// preview proxy domain, which may not be resolvable from all environments.
func (d *Driver) ExecuteCommand(ctx context.Context, externalID string, command string) (string, error) {
	cmdBody, _ := json.Marshal(map[string]string{"command": command})
	execURL := fmt.Sprintf("%s/toolbox/%s/toolbox/process/execute", d.apiURL, externalID)
	execReq, err := http.NewRequestWithContext(ctx, http.MethodPost, execURL, bytes.NewReader(cmdBody))
	if err != nil {
		return "", err
	}
	execReq.Header.Set("Authorization", "Bearer "+d.apiKey)
	execReq.Header.Set("Content-Type", "application/json")

	execResp, err := (&http.Client{Timeout: 120 * time.Second}).Do(execReq)
	if err != nil {
		return "", fmt.Errorf("executing command: %w", err)
	}
	defer execResp.Body.Close()

	respBody, _ := io.ReadAll(execResp.Body)
	if execResp.StatusCode >= 400 {
		return "", fmt.Errorf("execute command failed (status %d): %s", execResp.StatusCode, respBody)
	}

	var result struct {
		ExitCode int    `json:"exitCode"`
		Result   string `json:"result"`
	}
	json.Unmarshal(respBody, &result)

	if result.ExitCode != 0 {
		return result.Result, fmt.Errorf("command exited with code %d: %s", result.ExitCode, result.Result)
	}
	return result.Result, nil
}

// mapState converts Daytona's SandboxState to our SandboxStatus.
func mapState(state interface{}) sandbox.SandboxStatus {
	s := fmt.Sprintf("%v", state)
	switch s {
	case "started", "running":
		return sandbox.StatusRunning
	case "stopped":
		return sandbox.StatusStopped
	case "creating", "starting", "pending":
		return sandbox.StatusStarting
	case "error", "unknown":
		return sandbox.StatusError
	default:
		return sandbox.StatusError
	}
}
