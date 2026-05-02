package daytona

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	daytona "github.com/daytonaio/daytona/libs/sdk-go/pkg/daytona"
	"github.com/daytonaio/daytona/libs/sdk-go/pkg/types"

	"github.com/usehiveloop/hiveloop/internal/sandbox"
)

func (d *Driver) BuildSnapshot(ctx context.Context, opts sandbox.BuildSnapshotOpts) (string, error) {
	externalID, err := d.buildImage(ctx, opts, nil)
	return externalID, err
}

func (d *Driver) BuildSnapshotWithLogs(ctx context.Context, opts sandbox.BuildSnapshotOpts, onLog func(string)) (string, error) {
	return d.buildImage(ctx, opts, onLog)
}

// bridgeDownloadURL is the placeholder download URL for the new ACP-harness
// bridge binary. Once useportal.bridge@rip-harness ships GitHub releases,
// replace this constant with the real release asset URL (and prefer fetching
// a checksum/signature alongside).
//
// TODO(migration): replace with the real release URL for useportal.bridge@rip-harness.
// e.g. https://github.com/useportal/bridge/releases/download/${version}/bridge-linux-x86_64
const bridgeDownloadURL = "https://github.com/useportal/bridge/releases/download/TODO-MIGRATION-rip-harness/bridge-linux-x86_64"

func (d *Driver) buildImage(ctx context.Context, opts sandbox.BuildSnapshotOpts, onLog func(string)) (string, error) {
	// New ACP-harness runtime contract: single-stage from node:22-bookworm-slim
	// (the bridge binary is built upstream by useportal.bridge@rip-harness and
	// downloaded here — see bridgeDownloadURL above).
	baseImage := opts.BaseImage
	if baseImage == "" {
		baseImage = "node:22-bookworm-slim"
	}

	image := daytona.Base(baseImage)

	// Minimal runtime tools the agents need for git ops and self-introspection.
	// Dropped from the previous image: rtk, uv/uvx, postgresql, redis, nvm, Go,
	// Rust, sandbox-agent, python3, custom git-credential / gh-wrapper scripts.
	image = image.AptGet([]string{"ca-certificates", "curl", "git", "jq", "unzip", "openssh-client"})

	// gh CLI installed via the official apt repo — agents still need it for git ops.
	image = image.Run(
		"curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg | dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg && " +
			`echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" | tee /etc/apt/sources.list.d/github-cli.list > /dev/null && ` +
			"apt-get update && apt-get install -y --no-install-recommends gh && rm -rf /var/lib/apt/lists/*",
	)

	// ACP harnesses installed globally so bridge can spawn them as subprocesses.
	image = image.Run("npm install -g @agentclientprotocol/claude-agent-acp@0.31.4 opencode-ai@1.14.32")

	// Per-harness config dirs under HOME=/work.
	image = image.Run("mkdir -p /work/.claude /work/.opencode")

	// Download the bridge binary. The URL is a TODO placeholder; see bridgeDownloadURL.
	image = image.Run(
		fmt.Sprintf(`curl -fsSL %q -o /usr/local/bin/bridge && chmod +x /usr/local/bin/bridge`, bridgeDownloadURL),
	)

	// Image-level ENV mirrors what orchestrator_types.baseEnvVars sets per-sandbox,
	// so a snapshot booted without the orchestrator (manual debug) still has a
	// sane environment.
	image = image.Env("HOME", "/work")
	image = image.Env("CLAUDE_CONFIG_DIR", "/work/.claude")
	image = image.Env("OPENCODE_CONFIG_DIR", "/work/.opencode")
	image = image.Env("NO_BROWSER", "1")

	if len(opts.BuildCommands) > 0 {
		commands := make([]string, 0, len(opts.BuildCommands))
		for _, cmd := range opts.BuildCommands {
			trimmed := strings.TrimSpace(cmd)
			if trimmed != "" {
				commands = append(commands, trimmed)
			}
		}
		if len(commands) > 0 {
			image = image.Run(strings.Join(commands, " && "))
		}
	}

	// Workdir /work so the bridge SQLite DB at /work/bridge.db survives stop/start.
	image = image.Workdir("/work")
	image = image.Entrypoint([]string{"/bin/sh", "-c", "mkdir -p /work/.claude /work/.opencode && /usr/local/bin/bridge >> /tmp/bridge.log 2>&1"})

	params := &types.CreateSnapshotParams{
		Name:  opts.Name,
		Image: image,
	}
	if opts.CPU > 0 || opts.Memory > 0 || opts.Disk > 0 {
		params.Resources = &types.Resources{
			CPU:    opts.CPU,
			Memory: opts.Memory,
			Disk:   opts.Disk,
		}
	}

	snapshot, logChan, err := d.client.Snapshot.Create(ctx, params)
	if err != nil {
		return "", fmt.Errorf("creating snapshot: %w", err)
	}

	if logChan != nil {
		go func() {
			for line := range logChan {
				if onLog != nil {
					onLog(line)
				}
			}
		}()
	} else if onLog != nil {
		onLog("no log channel available from provider")
	}

	return snapshot.Name, nil
}

func (d *Driver) DeleteSnapshot(ctx context.Context, externalID string) error {
	status, err := d.GetSnapshotStatus(ctx, externalID)
	if err != nil {
		return nil
	}

	if status.State == "building" || status.State == "pending" {
		return fmt.Errorf("cannot delete snapshot while in state: %s", status.State)
	}

	snapshot, err := d.client.Snapshot.Get(ctx, externalID)
	if err != nil {
		return nil
	}
	return d.client.Snapshot.Delete(ctx, snapshot)
}

func (d *Driver) GetSnapshotStatus(ctx context.Context, externalID string) (*sandbox.SnapshotStatusResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.apiURL+"/snapshots/"+externalID, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+d.apiKey)
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("getting snapshot status: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get snapshot status failed (status %d): %s", resp.StatusCode, body)
	}

	var result struct {
		State       string `json:"state"`
		ErrorMsg    string `json:"error,omitempty"`
		ErrorReason string `json:"errorReason,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding snapshot status response: %w", err)
	}

	return &sandbox.SnapshotStatusResult{
		State:       result.State,
		ErrorMsg:    result.ErrorMsg,
		ErrorReason: result.ErrorReason,
	}, nil
}

func (d *Driver) GetSnapshotLogs(ctx context.Context, externalID string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.apiURL+"/snapshots/"+externalID+"/logs", nil)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+d.apiKey)
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return "", fmt.Errorf("getting snapshot logs: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("get snapshot logs failed (status %d): %s", resp.StatusCode, body)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading snapshot logs: %w", err)
	}
	return string(body), nil
}
