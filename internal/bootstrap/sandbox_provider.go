package bootstrap

import (
	"errors"
	"fmt"
	"strings"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/sandbox"
	"github.com/usehivy/hivy/internal/sandbox/daytona"
	dockerprovider "github.com/usehivy/hivy/internal/sandbox/docker"
)

var errSandboxProviderNotConfigured = errors.New("sandbox provider not configured")

func newSandboxProvider(cfg *config.Config) (sandbox.Provider, error) {
	providerID := strings.TrimSpace(cfg.SandboxProviderID)
	if providerID == "" {
		return nil, errSandboxProviderNotConfigured
	}

	switch providerID {
	case sandbox.ProviderDaytona:
		if strings.TrimSpace(cfg.DaytonaAPIKey) == "" {
			return nil, fmt.Errorf("%w: HIVY_DAYTONA_API_KEY is empty", errSandboxProviderNotConfigured)
		}
		if strings.TrimSpace(cfg.SpecialistSandboxRuntimeVersion) == "" {
			return nil, fmt.Errorf("%w: HIVY_SPECIALIST_SANDBOX_RUNTIME_VERSION is empty", errSandboxProviderNotConfigured)
		}
		return daytona.NewDriver(daytona.Config{
			APIURL:                          cfg.DaytonaAPIURL,
			APIKey:                          cfg.DaytonaAPIKey,
			Target:                          cfg.DaytonaTarget,
			SpecialistSandboxRuntimeVersion: cfg.SpecialistSandboxRuntimeVersion,
		})
	case sandbox.ProviderDocker:
		if strings.TrimSpace(cfg.SandboxDockerPublicHost) == "" {
			return nil, fmt.Errorf("%w: HIVY_SANDBOX_DOCKER_PUBLIC_HOST is empty", errSandboxProviderNotConfigured)
		}
		return dockerprovider.NewDriver(dockerprovider.Config{
			Host:                 cfg.SandboxDockerHost,
			PublicHost:           cfg.SandboxDockerPublicHost,
			ContainerLabelPrefix: cfg.SandboxDockerContainerLabelPrefix,
		})
	default:
		return nil, fmt.Errorf("unsupported HIVY_SANDBOX_PROVIDER_ID %q", providerID)
	}
}
