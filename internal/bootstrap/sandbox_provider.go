package bootstrap

import (
	"fmt"
	"strings"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/sandbox"
	"github.com/usehivy/hivy/internal/sandbox/daytona"
)

func newSandboxProvider(cfg *config.Config) (sandbox.Provider, error) {
	providerID := strings.TrimSpace(cfg.SandboxProviderID)
	if providerID == "" {
		providerID = sandbox.ProviderDaytona
	}

	switch providerID {
	case sandbox.ProviderDaytona:
		return daytona.NewDriver(daytona.Config{
			APIURL:              cfg.SandboxProviderURL,
			APIKey:              cfg.SandboxProviderKey,
			Target:              cfg.SandboxTarget,
			BridgeBinaryVersion: cfg.BridgeBinaryVersion,
		})
	default:
		return nil, fmt.Errorf("unsupported SANDBOX_PROVIDER_ID %q", providerID)
	}
}
