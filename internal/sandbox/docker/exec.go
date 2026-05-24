package docker

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/errdefs"
	"github.com/docker/docker/pkg/stdcopy"

	"github.com/usehivy/hivy/internal/sandbox"
)

const defaultExecTimeout = 2 * time.Minute

func (d *Driver) ExecuteCommand(ctx context.Context, externalID string, command string) (string, error) {
	return d.ExecuteCommandWithTimeout(ctx, externalID, command, defaultExecTimeout)
}

func (d *Driver) ExecuteCommandWithTimeout(ctx context.Context, externalID string, command string, timeout time.Duration) (string, error) {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	execID, err := d.cli.ContainerExecCreate(ctx, externalID, container.ExecOptions{
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          []string{"/bin/sh", "-lc", command},
	})
	if err != nil {
		if errdefs.IsNotFound(err) {
			return "", sandbox.ErrSandboxNotFound
		}
		return "", fmt.Errorf("creating docker exec in %s: %w", externalID, err)
	}

	attached, err := d.cli.ContainerExecAttach(ctx, execID.ID, container.ExecStartOptions{})
	if err != nil {
		if errdefs.IsNotFound(err) {
			return "", sandbox.ErrSandboxNotFound
		}
		return "", fmt.Errorf("attaching docker exec in %s: %w", externalID, err)
	}
	defer attached.Close()

	var output bytes.Buffer
	if _, err := stdcopy.StdCopy(&output, &output, attached.Reader); err != nil {
		return output.String(), fmt.Errorf("reading docker exec output in %s: %w", externalID, err)
	}

	inspect, err := d.cli.ContainerExecInspect(ctx, execID.ID)
	if err != nil {
		return output.String(), fmt.Errorf("inspecting docker exec in %s: %w", externalID, err)
	}
	if inspect.ExitCode != 0 {
		return output.String(), fmt.Errorf("docker exec in %s exited with code %d: %s", externalID, inspect.ExitCode, strings.TrimSpace(output.String()))
	}
	return output.String(), nil
}
