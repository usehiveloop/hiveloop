package tasks

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/usehivy/hivy/internal/sandbox"
)

type employeeUpgradeProvider struct {
	mu           sync.Mutex
	endpoint     string
	failCreate   bool
	failSync     bool
	created      []sandbox.CreateSandboxOpts
	started      []string
	stopped      []string
	deleted      []string
	commands     []string
	nextExternal int
}

func (p *employeeUpgradeProvider) ID() string { return sandbox.ProviderDaytona }

func (p *employeeUpgradeProvider) Validate(context.Context) error { return nil }

func (p *employeeUpgradeProvider) RuntimeLayout() sandbox.RuntimeLayout {
	return sandbox.RuntimeLayout{
		AgentRepoDir:    "/home/daytona/repos",
		EmployeeRepoDir: "/workspace/repos",
	}
}

func (p *employeeUpgradeProvider) CreateSandbox(_ context.Context, opts sandbox.CreateSandboxOpts) (*sandbox.SandboxInfo, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.failCreate {
		return nil, errors.New("create failed")
	}
	p.nextExternal++
	p.created = append(p.created, opts)
	return &sandbox.SandboxInfo{ExternalID: fmt.Sprintf("new-external-%d", p.nextExternal), Status: sandbox.StatusRunning}, nil
}

func (p *employeeUpgradeProvider) StartSandbox(_ context.Context, externalID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.started = append(p.started, externalID)
	return nil
}

func (p *employeeUpgradeProvider) StopSandbox(_ context.Context, externalID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopped = append(p.stopped, externalID)
	return nil
}

func (p *employeeUpgradeProvider) ArchiveSandbox(context.Context, string) error { return nil }

func (p *employeeUpgradeProvider) DeleteSandbox(_ context.Context, externalID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.deleted = append(p.deleted, externalID)
	return nil
}

func (p *employeeUpgradeProvider) GetStatus(context.Context, string) (sandbox.SandboxStatus, error) {
	return sandbox.StatusRunning, nil
}

func (p *employeeUpgradeProvider) GetEndpoint(context.Context, string, int) (string, error) {
	return p.endpoint, nil
}

func (p *employeeUpgradeProvider) BuildTemplate(context.Context, sandbox.TemplateBuildRequest) (string, error) {
	return "", nil
}

func (p *employeeUpgradeProvider) BuildTemplateWithLogs(context.Context, sandbox.TemplateBuildRequest, func(string)) (string, error) {
	return "", nil
}

func (p *employeeUpgradeProvider) GetTemplateStatus(context.Context, string) (*sandbox.TemplateBuildStatus, error) {
	return &sandbox.TemplateBuildStatus{State: "ready"}, nil
}

func (p *employeeUpgradeProvider) GetTemplateLogs(context.Context, string) (string, error) {
	return "", nil
}

func (p *employeeUpgradeProvider) DeleteTemplate(context.Context, string) error { return nil }

func (p *employeeUpgradeProvider) SetAutoStop(context.Context, string, int) error {
	return nil
}

func (p *employeeUpgradeProvider) SetAutoArchive(context.Context, string, int) error {
	return nil
}

func (p *employeeUpgradeProvider) ExecuteCommand(ctx context.Context, externalID string, command string) (string, error) {
	return p.ExecuteCommandWithTimeout(ctx, externalID, command, 0)
}

func (p *employeeUpgradeProvider) ExecuteCommandWithTimeout(_ context.Context, _ string, command string, _ time.Duration) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.commands = append(p.commands, command)
	return "", nil
}

func (p *employeeUpgradeProvider) GetResourceUsage(context.Context, string) (*sandbox.ResourceUsage, error) {
	return &sandbox.ResourceUsage{}, nil
}
