package docker

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/docker/docker/errdefs"
	"github.com/docker/go-connections/nat"

	"github.com/usehivy/hivy/internal/sandbox"
)

func (d *Driver) GetEndpoint(ctx context.Context, externalID string, port int) (string, error) {
	info, err := d.cli.ContainerInspect(ctx, externalID)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return "", sandbox.ErrSandboxNotFound
		}
		return "", fmt.Errorf("inspecting docker container %s: %w", externalID, err)
	}
	if info.NetworkSettings == nil {
		return "", fmt.Errorf("docker container %s has no network settings", externalID)
	}

	key := nat.Port(strconv.Itoa(port) + "/tcp")
	bindings := info.NetworkSettings.Ports[key]
	if len(bindings) == 0 || strings.TrimSpace(bindings[0].HostPort) == "" {
		return "", fmt.Errorf("docker container %s has no host binding for port %d", externalID, port)
	}
	return fmt.Sprintf("http://%s:%s", d.publicHost, bindings[0].HostPort), nil
}
