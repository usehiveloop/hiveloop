package daytona

import (
	"context"
	"fmt"
	"io"
	"strings"

	daytonasdk "github.com/daytonaio/daytona/libs/sdk-go/pkg/daytona"
	sdktypes "github.com/daytonaio/daytona/libs/sdk-go/pkg/types"

	"github.com/usehivy/hivy/internal/sandbox"
)

func (d *Driver) BuildTemplate(ctx context.Context, opts sandbox.TemplateBuildRequest) (string, error) {
	return d.buildImage(ctx, opts, nil)
}

func (d *Driver) BuildTemplateWithLogs(ctx context.Context, opts sandbox.TemplateBuildRequest, onLog func(string)) (string, error) {
	return d.buildImage(ctx, opts, onLog)
}

func (d *Driver) buildImage(ctx context.Context, opts sandbox.TemplateBuildRequest, onLog func(string)) (string, error) {
	baseImage := opts.BaseImage
	if baseImage == "" {
		baseImage = "node:22-bookworm-slim"
	}

	tag := "v" + strings.TrimPrefix(d.cloudAgentsSandboxRuntimeVersion, "v")
	bridgeDownloadURL := fmt.Sprintf(
		"https://github.com/usehivy/hivy/releases/download/%s/bridge-%s-x86_64-unknown-linux-gnu.tar.gz",
		tag, tag,
	)

	image := daytonasdk.Base(baseImage)

	// Minimal runtime tools — the canonical fat image with rtk/uv/Go/Rust
	// lives in cmd/buildtemplates; user templates layer on top via BuildCommands.
	image = image.AptGet([]string{"ca-certificates", "curl", "git", "jq", "unzip", "openssh-client"})

	image = image.Run(
		"curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg | dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg && " +
			`echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" | tee /etc/apt/sources.list.d/github-cli.list > /dev/null && ` +
			"apt-get update && apt-get install -y --no-install-recommends gh && rm -rf /var/lib/apt/lists/*",
	)

	// ACP harnesses installed globally so bridge can spawn them as subprocesses.
	image = image.Run("npm install -g @agentclientprotocol/claude-agent-acp@0.31.4 opencode-ai@1.14.32")

	image = image.Run("mkdir -p /work/.claude /work/.opencode")

	image = image.Run(
		fmt.Sprintf(`curl -fsSL %q | tar -xzf - -C /usr/local/bin/ bridge && chmod +x /usr/local/bin/bridge`, bridgeDownloadURL),
	)

	// Image-level ENV mirrors orchestrator_types.baseEnvVars so a manual
	// `docker run` (without the orchestrator) lands in the same shape.
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

	image = image.Workdir("/work")
	image = image.Entrypoint([]string{"/bin/sh", "-c", "mkdir -p /work/.claude /work/.opencode && /usr/local/bin/bridge >> /tmp/bridge.log 2>&1"})

	params := &sdktypes.CreateSnapshotParams{
		Name:  opts.Name,
		Image: image,
	}
	if opts.CPU > 0 || opts.Memory > 0 || opts.Disk > 0 {
		params.Resources = &sdktypes.Resources{
			CPU:    opts.CPU,
			Memory: opts.Memory,
			Disk:   opts.Disk,
		}
	}

	snapshot, logChan, err := d.sdk.Snapshot.Create(ctx, params)
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

func (d *Driver) DeleteTemplate(ctx context.Context, externalID string) error {
	snapshot, err := d.sdk.Snapshot.Get(ctx, externalID)
	if err != nil {
		// Treat "not found" as success — delete is idempotent.
		return nil
	}
	if snapshot.State == "building" || snapshot.State == "pending" {
		return fmt.Errorf("cannot delete snapshot while in state: %s", snapshot.State)
	}
	if err := d.sdk.Snapshot.Delete(ctx, snapshot); err != nil {
		return fmt.Errorf("deleting snapshot %s: %w", externalID, err)
	}
	return nil
}

func (d *Driver) GetTemplateStatus(ctx context.Context, externalID string) (*sandbox.TemplateBuildStatus, error) {
	snapshot, err := d.sdk.Snapshot.Get(ctx, externalID)
	if err != nil {
		return nil, fmt.Errorf("getting snapshot %s: %w", externalID, err)
	}
	result := &sandbox.TemplateBuildStatus{State: snapshot.State}
	if snapshot.ErrorReason != nil {
		result.ErrorReason = *snapshot.ErrorReason
		result.ErrorMsg = *snapshot.ErrorReason
	}
	return result, nil
}

// GetSnapshotLogs fetches build logs for an existing snapshot. Used as a
// diagnostic when a snapshot build fails. The high-level pkg/daytona SDK
// only streams build logs as part of Snapshot.Create, so we drop down to
// api-client-go's GetSnapshotBuildLogs (raw response body) here.
func (d *Driver) GetTemplateLogs(ctx context.Context, externalID string) (string, error) {
	resp, err := d.apiClient.SnapshotsAPI.
		GetSnapshotBuildLogs(d.authCtx(ctx), externalID).
		Execute()
	if err != nil {
		return "", fmt.Errorf("getting snapshot logs %s: %w", externalID, err)
	}
	if resp == nil || resp.Body == nil {
		return "", nil
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return "", fmt.Errorf("reading snapshot logs %s: %w", externalID, readErr)
	}
	return string(body), nil
}
