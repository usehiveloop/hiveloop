package railway

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/usehivy/hivy/internal/employeeruntime"
	railwayapi "github.com/usehivy/hivy/internal/railway"
	"github.com/usehivy/hivy/internal/sandbox"
)

const defaultRuntimePort = 7080
const runtimeCommandHTTPTimeoutPadding = 30 * time.Second

type Config struct {
	APIToken      string
	ProjectID     string
	EnvironmentID string
	Region        string
	RuntimePort   int
}

type Driver struct {
	client        *railwayapi.Client
	projectID     string
	environmentID string
	region        string
	runtimePort   int
}

func NewDriver(cfg Config) (*Driver, error) {
	client, err := railwayapi.NewClient(railwayapi.Config{Token: cfg.APIToken})
	if err != nil {
		return nil, err
	}
	port := cfg.RuntimePort
	if port == 0 {
		port = defaultRuntimePort
	}
	return &Driver{
		client:        client,
		projectID:     strings.TrimSpace(cfg.ProjectID),
		environmentID: strings.TrimSpace(cfg.EnvironmentID),
		region:        strings.TrimSpace(cfg.Region),
		runtimePort:   port,
	}, nil
}

func (d *Driver) ID() string { return sandbox.ProviderRailway }

func (d *Driver) Validate(ctx context.Context) error {
	if d.projectID == "" {
		return fmt.Errorf("HIVY_RAILWAY_PROJECT_ID is required")
	}
	if d.environmentID == "" {
		return fmt.Errorf("HIVY_RAILWAY_ENVIRONMENT_ID is required")
	}
	return nil
}

func (d *Driver) RuntimeLayout() sandbox.RuntimeLayout {
	return sandbox.RuntimeLayout{
		AgentRepoDir:    "/workspace/repos",
		EmployeeRepoDir: "/workspace/repos",
	}
}

func (d *Driver) CreateWarmSlot(ctx context.Context, opts sandbox.WarmSlotCreateOpts) (*sandbox.WarmSlotInfo, error) {
	image := strings.TrimSpace(opts.RuntimeImage)
	if image == "" {
		return nil, fmt.Errorf("railway: RuntimeImage is required")
	}
	port := opts.RuntimePort
	if port == 0 {
		port = d.runtimePort
	}
	vars := map[string]string{
		"HIVY_RUNTIME_SECRET":    opts.RuntimeSecret,
		"HIVY_RUNTIME_BIND_ADDR": fmt.Sprintf("0.0.0.0:%d", port),
		"HIVY_WORKSPACE_ROOT":    "/workspace",
		"HIVY_DB_PATH":           "/app/data/hivy-sandboxes-runtime.db",
		"PORT":                   fmt.Sprintf("%d", port),
	}
	for key, value := range opts.EnvVars {
		vars[key] = value
	}
	service, err := d.client.CreateServiceFromImage(ctx, railwayapi.CreateServiceInput{
		Name:          opts.Name,
		ProjectID:     d.projectID,
		EnvironmentID: d.environmentID,
		Image:         image,
		Variables:     vars,
	})
	if err != nil {
		return nil, fmt.Errorf("create railway service: %w", err)
	}
	domain, err := d.client.CreateServiceDomain(ctx, d.projectID, d.environmentID, service.ID)
	if err != nil {
		_ = d.client.DeleteService(context.WithoutCancel(ctx), d.environmentID, service.ID)
		return nil, fmt.Errorf("create railway domain: %w", err)
	}
	return &sandbox.WarmSlotInfo{
		ExternalID:  service.ID,
		EndpointURL: "https://" + strings.TrimSpace(domain.Domain),
		RuntimePort: port,
	}, nil
}

func (d *Driver) CreateSandbox(context.Context, sandbox.CreateSandboxOpts) (*sandbox.SandboxInfo, error) {
	return nil, fmt.Errorf("railway sandboxes must be created through the warm pool")
}

func (d *Driver) StartSandbox(context.Context, string) error { return nil }

func (d *Driver) StopSandbox(context.Context, string) error { return nil }

func (d *Driver) ArchiveSandbox(context.Context, string) error { return nil }

func (d *Driver) DeleteSandbox(ctx context.Context, externalID string) error {
	if err := d.client.DeleteService(ctx, d.environmentID, externalID); err != nil {
		return fmt.Errorf("delete railway service %s: %w", externalID, err)
	}
	return nil
}

func (d *Driver) GetStatus(ctx context.Context, externalID string) (sandbox.SandboxStatus, error) {
	deployments, err := d.client.Deployments(ctx, railwayapi.DeploymentListInput{
		ProjectID:     d.projectID,
		EnvironmentID: d.environmentID,
		ServiceID:     externalID,
		First:         1,
	})
	if err != nil {
		return sandbox.StatusError, err
	}
	if len(deployments) == 0 {
		return sandbox.StatusCreating, nil
	}
	switch strings.ToUpper(deployments[0].Status) {
	case "SUCCESS", "DEPLOYED":
		return sandbox.StatusRunning, nil
	case "BUILDING", "DEPLOYING", "INITIALIZING", "QUEUED":
		return sandbox.StatusCreating, nil
	case "REMOVED", "STOPPED":
		return sandbox.StatusStopped, nil
	default:
		return sandbox.StatusError, nil
	}
}

func (d *Driver) GetEndpoint(ctx context.Context, externalID string, _ int) (string, error) {
	domains, err := d.client.Domains(ctx, d.projectID, d.environmentID, externalID)
	if err != nil {
		return "", err
	}
	if len(domains) == 0 {
		return "", fmt.Errorf("railway service %s has no service domain", externalID)
	}
	return "https://" + strings.TrimSpace(domains[0].Domain), nil
}

func (d *Driver) BuildTemplate(context.Context, sandbox.TemplateBuildRequest) (string, error) {
	return "", fmt.Errorf("railway provider does not build templates")
}

func (d *Driver) BuildTemplateWithLogs(context.Context, sandbox.TemplateBuildRequest, func(string)) (string, error) {
	return "", fmt.Errorf("railway provider does not build templates")
}

func (d *Driver) GetTemplateStatus(context.Context, string) (*sandbox.TemplateBuildStatus, error) {
	return nil, fmt.Errorf("railway provider does not build templates")
}

func (d *Driver) GetTemplateLogs(context.Context, string) (string, error) {
	return "", fmt.Errorf("railway provider does not build templates")
}

func (d *Driver) DeleteTemplate(context.Context, string) error { return nil }

func (d *Driver) SetAutoStop(context.Context, string, int) error { return nil }

func (d *Driver) SetAutoArchive(context.Context, string, int) error { return nil }

func (d *Driver) ExecuteCommand(context.Context, string, string) (string, error) {
	return "", fmt.Errorf("railway command execution is handled through runtime HTTP")
}

func (d *Driver) ExecuteCommandWithTimeout(context.Context, string, string, time.Duration) (string, error) {
	return "", fmt.Errorf("railway command execution is handled through runtime HTTP")
}

func (d *Driver) GetResourceUsage(context.Context, string) (*sandbox.ResourceUsage, error) {
	return &sandbox.ResourceUsage{}, nil
}

func (d *Driver) UsesWarmPool() bool { return true }

func (d *Driver) ExecuteCommandViaRuntime(ctx context.Context, cmdCtx sandbox.RuntimeCommandContext, command string, timeout time.Duration) (string, error) {
	seconds := int(timeout.Seconds())
	if seconds <= 0 {
		seconds = 120
	}
	client := employeeruntime.NewClientWithTimeout(cmdCtx.RuntimeURL, cmdCtx.RuntimeSecret, runtimeCommandHTTPTimeout(time.Duration(seconds)*time.Second))
	resp, err := client.RunCommands(ctx, employeeruntime.ControlCommandsRequest{
		Commands:       []string{command},
		TimeoutSeconds: seconds,
		StopOnError:    true,
	})
	if err != nil {
		return "", err
	}
	if len(resp.Results) == 0 {
		return "", fmt.Errorf("runtime command returned no results")
	}
	result := resp.Results[0]
	if !resp.OK {
		return result.Output, fmt.Errorf("command failed in runtime: exit_code=%v timed_out=%v", result.ExitCode, result.TimedOut)
	}
	return result.Output, nil
}

func runtimeCommandHTTPTimeout(commandTimeout time.Duration) time.Duration {
	if commandTimeout <= 0 {
		commandTimeout = 120 * time.Second
	}
	return commandTimeout + runtimeCommandHTTPTimeoutPadding
}
