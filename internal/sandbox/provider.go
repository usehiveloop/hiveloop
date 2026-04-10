package sandbox

import "context"

// SandboxStatus represents the state of a sandbox.
type SandboxStatus string

const (
	StatusCreating SandboxStatus = "creating"
	StatusRunning  SandboxStatus = "running"
	StatusStopped  SandboxStatus = "stopped"
	StatusStarting SandboxStatus = "starting"
	StatusError    SandboxStatus = "error"
)

// CreateSandboxOpts configures a new sandbox.
type CreateSandboxOpts struct {
	Name       string            // human-readable name
	SnapshotID string            // provider's snapshot/template ID (empty = base image)
	EnvVars    map[string]string // environment variables (e.g. BRIDGE_* config)
	Labels     map[string]string // metadata labels (org_id, sandbox_type, agent_id)
}

// SandboxInfo is returned after creating a sandbox.
type SandboxInfo struct {
	ExternalID string // provider's sandbox identifier
	Status     SandboxStatus
}

// BuildSnapshotOpts configures a snapshot (template) build.
type BuildSnapshotOpts struct {
	Name          string // snapshot name
	BuildCommands string // commands to run on the base image
	BaseImage     string // base image to build on top of
}

// SnapshotBuildStatus tracks snapshot build progress.
type SnapshotBuildStatus struct {
	ExternalID string
	Ready      bool
	Error      string
}

// SnapshotStatusResult holds the status check result.
type SnapshotStatusResult struct {
	State       string // "building", "ready", "error", "deleting"
	ErrorMsg    string
	ErrorReason string // detailed error reason from Daytona
}

// Provider is the interface that all sandbox providers must implement.
// This allows swapping Daytona for another provider (E2B, Fly.io, etc.)
// by implementing this interface.
type Provider interface {
	// Lifecycle
	CreateSandbox(ctx context.Context, opts CreateSandboxOpts) (*SandboxInfo, error)
	StartSandbox(ctx context.Context, externalID string) error
	StopSandbox(ctx context.Context, externalID string) error
	DeleteSandbox(ctx context.Context, externalID string) error
	GetStatus(ctx context.Context, externalID string) (SandboxStatus, error)

	// Networking — returns the URL to reach a port inside the sandbox.
	GetEndpoint(ctx context.Context, externalID string, port int) (string, error)

	// Snapshots (templates)
	BuildSnapshot(ctx context.Context, opts BuildSnapshotOpts) (externalID string, err error)
	BuildSnapshotWithLogs(ctx context.Context, opts BuildSnapshotOpts, onLog func(string)) (externalID string, err error)
	GetSnapshotStatus(ctx context.Context, externalID string) (*SnapshotStatusResult, error)
	GetSnapshotLogs(ctx context.Context, externalID string) (string, error)
	DeleteSnapshot(ctx context.Context, externalID string) error

	// Auto-management
	SetAutoStop(ctx context.Context, externalID string, intervalMinutes int) error

	// Execution — run a command inside the sandbox.
	ExecuteCommand(ctx context.Context, externalID string, command string) (string, error)
}
