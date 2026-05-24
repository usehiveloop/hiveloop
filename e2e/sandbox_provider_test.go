package e2e

import (
	"context"
	"fmt"
	"time"

	"github.com/usehivy/hivy/internal/sandbox"
)

type e2eSandboxProvider struct {
	endpoint string
}

func (p *e2eSandboxProvider) ID() string { return sandbox.ProviderDaytona }

func (p *e2eSandboxProvider) Validate(context.Context) error { return nil }

func (p *e2eSandboxProvider) RuntimeLayout() sandbox.RuntimeLayout { return sandbox.RuntimeLayout{} }

func (p *e2eSandboxProvider) CreateSandbox(context.Context, sandbox.CreateSandboxOpts) (*sandbox.SandboxInfo, error) {
	return &sandbox.SandboxInfo{ExternalID: "e2e-sandbox", Status: sandbox.StatusRunning}, nil
}

func (p *e2eSandboxProvider) StartSandbox(context.Context, string) error { return nil }

func (p *e2eSandboxProvider) StopSandbox(context.Context, string) error { return nil }

func (p *e2eSandboxProvider) ArchiveSandbox(context.Context, string) error { return nil }

func (p *e2eSandboxProvider) DeleteSandbox(context.Context, string) error { return nil }

func (p *e2eSandboxProvider) GetStatus(context.Context, string) (sandbox.SandboxStatus, error) {
	return sandbox.StatusRunning, nil
}

func (p *e2eSandboxProvider) GetEndpoint(context.Context, string, int) (string, error) {
	if p.endpoint == "" {
		return "", fmt.Errorf("e2e sandbox endpoint not configured")
	}
	return p.endpoint, nil
}

func (p *e2eSandboxProvider) BuildTemplate(context.Context, sandbox.TemplateBuildRequest) (string, error) {
	return "e2e-template", nil
}

func (p *e2eSandboxProvider) BuildTemplateWithLogs(_ context.Context, _ sandbox.TemplateBuildRequest, onLog func(string)) (string, error) {
	if onLog != nil {
		onLog("e2e template ready")
	}
	return "e2e-template", nil
}

func (p *e2eSandboxProvider) GetTemplateStatus(context.Context, string) (*sandbox.TemplateBuildStatus, error) {
	return &sandbox.TemplateBuildStatus{State: "ready"}, nil
}

func (p *e2eSandboxProvider) GetTemplateLogs(context.Context, string) (string, error) { return "", nil }

func (p *e2eSandboxProvider) DeleteTemplate(context.Context, string) error { return nil }

func (p *e2eSandboxProvider) SetAutoStop(context.Context, string, int) error { return nil }

func (p *e2eSandboxProvider) SetAutoArchive(context.Context, string, int) error { return nil }

func (p *e2eSandboxProvider) ExecuteCommand(context.Context, string, string) (string, error) {
	return "", nil
}

func (p *e2eSandboxProvider) ExecuteCommandWithTimeout(context.Context, string, string, time.Duration) (string, error) {
	return "", nil
}

func (p *e2eSandboxProvider) GetResourceUsage(context.Context, string) (*sandbox.ResourceUsage, error) {
	return &sandbox.ResourceUsage{}, nil
}
