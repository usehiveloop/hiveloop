package docker

import (
	"context"
	"fmt"
	"strings"

	"github.com/docker/docker/client"

	"github.com/usehivy/hivy/internal/sandbox"
)

type Config struct {
	Host                 string
	PublicHost           string
	ContainerLabelPrefix string
}

type Driver struct {
	cli         *client.Client
	publicHost  string
	labelPrefix string
}

func NewDriver(cfg Config) (*Driver, error) {
	opts := []client.Opt{client.FromEnv, client.WithAPIVersionNegotiation()}
	if strings.TrimSpace(cfg.Host) != "" {
		opts = append(opts, client.WithHost(strings.TrimSpace(cfg.Host)))
	}
	cli, err := client.NewClientWithOpts(opts...)
	if err != nil {
		return nil, fmt.Errorf("creating docker client: %w", err)
	}
	labelPrefix := strings.TrimSpace(cfg.ContainerLabelPrefix)
	if labelPrefix == "" {
		labelPrefix = "hivy"
	}
	return &Driver{
		cli:         cli,
		publicHost:  strings.TrimSpace(cfg.PublicHost),
		labelPrefix: labelPrefix,
	}, nil
}

func (d *Driver) ID() string { return sandbox.ProviderDocker }

func (d *Driver) Validate(ctx context.Context) error {
	if d.publicHost == "" {
		return fmt.Errorf("HIVY_SANDBOX_DOCKER_PUBLIC_HOST is required for docker sandbox provider")
	}
	if _, err := d.cli.Ping(ctx); err != nil {
		return fmt.Errorf("ping docker daemon: %w", err)
	}
	return nil
}

func (d *Driver) RuntimeLayout() sandbox.RuntimeLayout {
	return sandbox.RuntimeLayout{
		AgentRepoDir:    "/work/repos",
		EmployeeRepoDir: "/workspace/repos",
	}
}

func (d *Driver) labels(input map[string]string) map[string]string {
	labels := map[string]string{
		d.labelPrefix + ".provider": "docker",
	}
	for key, value := range input {
		labels[d.labelPrefix+"."+key] = value
	}
	return labels
}
