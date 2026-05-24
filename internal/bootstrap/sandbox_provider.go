package bootstrap

import (
	"fmt"
	"strings"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/sandbox"
	"github.com/usehivy/hivy/internal/sandbox/daytona"
	dockerprovider "github.com/usehivy/hivy/internal/sandbox/docker"
)

func newSandboxProvider(cfg *config.Config) (sandbox.Provider, error) {
	providerID := strings.TrimSpace(cfg.SandboxProviderID)
	if providerID == "" {
		providerID = sandbox.ProviderDaytona
	}

	switch providerID {
	case sandbox.ProviderDaytona:
		return daytona.NewDriver(daytona.Config{
			APIURL:                           cfg.DaytonaAPIURL,
			APIKey:                           cfg.DaytonaAPIKey,
			Target:                           cfg.DaytonaTarget,
			CloudAgentsSandboxRuntimeVersion: cfg.CloudAgentsSandboxRuntimeVersion,
		})
	case sandbox.ProviderDocker:
		return dockerprovider.NewDriver(dockerprovider.Config{
			Host:                 cfg.SandboxDockerHost,
			PublicHost:           cfg.SandboxDockerPublicHost,
			ContainerLabelPrefix: cfg.SandboxDockerContainerLabelPrefix,
		})
	default:
		return nil, fmt.Errorf("unsupported HIVY_SANDBOX_PROVIDER_ID %q", providerID)
	}
}
