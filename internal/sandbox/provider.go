package sandbox

import (
	"context"
	"errors"
	"time"
)

var ErrSandboxNotFound = errors.New("sandbox not found upstream")

const ProviderDaytona = "daytona"

// SandboxStatus represents the state of a sandbox.
type SandboxStatus string

const (
	StatusCreating  SandboxStatus = "creating"
	StatusRunning   SandboxStatus = "running"
	StatusStopped   SandboxStatus = "stopped"
	StatusStarting  SandboxStatus = "starting"
	StatusArchived  SandboxStatus = "archived"
	StatusArchiving SandboxStatus = "archiving"
	StatusError     SandboxStatus = "error"
)

// CreateSandboxOpts configures a new sandbox.
type CreateSandboxOpts struct {
	Name        string            // human-readable name
	TemplateRef string            // provider template/image reference
	EnvVars     map[string]string // environment variables (e.g. BRIDGE_* config)
	Labels      map[string]string // metadata labels (org_id, sandbox_id, agent_id)
}

// SandboxInfo is returned after creating a sandbox.
type SandboxInfo struct {
	ExternalID string // provider's sandbox identifier
	Status     SandboxStatus
}

// TemplateBuildRequest configures a provider template/image build.
type TemplateBuildRequest struct {
	Name          string   // provider template name
	BuildCommands []string // commands to run on the base image
	BaseImage     string   // base image to build on top of
	CPU           int      // CPU cores (0 = provider default)
	Memory        int      // memory in GB (0 = provider default)
	Disk          int      // disk in GB (0 = provider default)
}

// TemplateBuildResult tracks template build progress.
type TemplateBuildResult struct {
	ExternalID string
	Ready      bool
	Error      string
}

// TemplateBuildStatus holds the status check result.
type TemplateBuildStatus struct {
	State       string // "building", "ready", "error", "deleting"
	ErrorMsg    string
	ErrorReason string // provider-specific detailed error reason
}

// ResourceUsage is the provider-normalized runtime resource usage for a sandbox.
type ResourceUsage struct {
	MemoryLimitBytes  int64
	MemoryUsedBytes   int64
	MemoryPeakBytes   int64
	CPUQuota          string
	CPUUsageUsec      int64
	CPUThrottledCount int64
	PIDCount          int64
}

type RuntimeLayout struct {
	AgentRepoDir    string
	EmployeeRepoDir string
}

// Provider is the complete startup-time contract a sandbox backend must satisfy.
type Provider interface {
	ID() string
	Validate(ctx context.Context) error
	RuntimeLayout() RuntimeLayout

	// Lifecycle
	CreateSandbox(ctx context.Context, opts CreateSandboxOpts) (*SandboxInfo, error)
	StartSandbox(ctx context.Context, externalID string) error
	StopSandbox(ctx context.Context, externalID string) error
	// ArchiveSandbox moves a stopped sandbox into cold storage. The provider must
	// be able to restore it via StartSandbox. The sandbox must be stopped first.
	ArchiveSandbox(ctx context.Context, externalID string) error
	DeleteSandbox(ctx context.Context, externalID string) error
	GetStatus(ctx context.Context, externalID string) (SandboxStatus, error)

	// Networking — returns the URL to reach a port inside the sandbox.
	GetEndpoint(ctx context.Context, externalID string, port int) (string, error)

	// Templates
	BuildTemplate(ctx context.Context, opts TemplateBuildRequest) (externalID string, err error)
	BuildTemplateWithLogs(ctx context.Context, opts TemplateBuildRequest, onLog func(string)) (externalID string, err error)
	GetTemplateStatus(ctx context.Context, externalID string) (*TemplateBuildStatus, error)
	GetTemplateLogs(ctx context.Context, externalID string) (string, error)
	DeleteTemplate(ctx context.Context, externalID string) error

	// Auto-management. intervalMinutes=0 disables the policy.
	SetAutoStop(ctx context.Context, externalID string, intervalMinutes int) error
	SetAutoArchive(ctx context.Context, externalID string, intervalMinutes int) error

	// Execution — run a command inside the sandbox.
	ExecuteCommand(ctx context.Context, externalID string, command string) (string, error)
	ExecuteCommandWithTimeout(ctx context.Context, externalID string, command string, timeout time.Duration) (string, error)

	GetResourceUsage(ctx context.Context, externalID string) (*ResourceUsage, error)
}
