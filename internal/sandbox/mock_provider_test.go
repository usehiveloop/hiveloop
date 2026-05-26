package sandbox

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// mockProvider is an in-memory sandbox.Provider for testing.
type mockProvider struct {
	mu                  sync.Mutex
	sandboxes           map[string]*mockSandbox
	endpoints           map[string]string // externalID → URL
	endpointOverride    string            // if set, all GetEndpoint calls return this URL
	nextID              int
	executeCommandFn    func(ctx context.Context, externalID, command string) (string, error)
	resourceUsageFn     func(ctx context.Context, externalID string) (*ResourceUsage, error)
	setAutoStopCalls    []autoPolicyCall
	setAutoArchiveCalls []autoPolicyCall
	archivedIDs         []string
	stoppedIDs          []string
	createCalls         []CreateSandboxOpts // captured for integration assertions
	endpointPorts       []int               // captured port arg of every GetEndpoint call
	warmEndpoint        string
	warmCreateCalls     []WarmSlotCreateOpts
}

// autoPolicyCall records one invocation of SetAutoStop / SetAutoArchive.
// Tests assert against these slices to verify sandbox creation paths pass
// intervalMinutes=0 (disabling provider-managed lifecycle).
type autoPolicyCall struct {
	externalID      string
	intervalMinutes int
}

type mockSandbox struct {
	name   string
	status SandboxStatus
}

func newMockProvider() *mockProvider {
	return &mockProvider{
		sandboxes: make(map[string]*mockSandbox),
		endpoints: make(map[string]string),
	}
}

func (m *mockProvider) ID() string { return ProviderDaytona }

func (m *mockProvider) Validate(context.Context) error { return nil }

func (m *mockProvider) RuntimeLayout() RuntimeLayout {
	return RuntimeLayout{
		AgentRepoDir:    "/home/daytona/repos",
		EmployeeRepoDir: "/workspace/repos",
	}
}

func (m *mockProvider) CreateSandbox(_ context.Context, opts CreateSandboxOpts) (*SandboxInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.nextID++
	id := fmt.Sprintf("mock-sb-%d", m.nextID)
	m.sandboxes[id] = &mockSandbox{name: opts.Name, status: StatusRunning}
	m.endpoints[id] = fmt.Sprintf("https://mock-sandbox-%d.test:%d", m.nextID, RuntimePort)
	m.createCalls = append(m.createCalls, opts)

	return &SandboxInfo{ExternalID: id, Status: StatusRunning}, nil
}

func (m *mockProvider) CreateWarmSlot(_ context.Context, opts WarmSlotCreateOpts) (*WarmSlotInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.nextID++
	id := fmt.Sprintf("mock-warm-%d", m.nextID)
	endpoint := m.warmEndpoint
	if endpoint == "" {
		endpoint = fmt.Sprintf("https://mock-warm-%d.test", m.nextID)
	}
	m.sandboxes[id] = &mockSandbox{name: opts.Name, status: StatusRunning}
	m.endpoints[id] = endpoint
	m.warmCreateCalls = append(m.warmCreateCalls, opts)
	return &WarmSlotInfo{ExternalID: id, EndpointURL: endpoint, RuntimePort: opts.RuntimePort}, nil
}

func (m *mockProvider) StartSandbox(_ context.Context, externalID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	sb, ok := m.sandboxes[externalID]
	if !ok {
		return fmt.Errorf("sandbox not found: %s", externalID)
	}
	sb.status = StatusRunning
	return nil
}

func (m *mockProvider) StopSandbox(_ context.Context, externalID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	sb, ok := m.sandboxes[externalID]
	if !ok {
		return fmt.Errorf("sandbox not found: %s", externalID)
	}
	sb.status = StatusStopped
	m.stoppedIDs = append(m.stoppedIDs, externalID)
	return nil
}

func (m *mockProvider) DeleteSandbox(_ context.Context, externalID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.sandboxes, externalID)
	delete(m.endpoints, externalID)
	return nil
}

func (m *mockProvider) GetStatus(_ context.Context, externalID string) (SandboxStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sb, ok := m.sandboxes[externalID]
	if !ok {
		return StatusError, fmt.Errorf("sandbox not found: %s", externalID)
	}
	return sb.status, nil
}

func (m *mockProvider) GetEndpoint(_ context.Context, externalID string, port int) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.sandboxes[externalID]; !exists {
		return "", fmt.Errorf("sandbox not found: %s", externalID)
	}

	m.endpointPorts = append(m.endpointPorts, port)

	if m.endpointOverride != "" {
		return m.endpointOverride, nil
	}

	url, ok := m.endpoints[externalID]
	if !ok {
		return "", fmt.Errorf("sandbox not found: %s", externalID)
	}
	return url, nil
}

func (m *mockProvider) BuildTemplate(_ context.Context, _ TemplateBuildRequest) (string, error) {
	return "mock-snapshot-id", nil
}

func (m *mockProvider) BuildTemplateWithLogs(_ context.Context, _ TemplateBuildRequest, _ func(string)) (string, error) {
	return "mock-snapshot-id", nil
}

func (m *mockProvider) GetTemplateStatus(_ context.Context, _ string) (*TemplateBuildStatus, error) {
	return &TemplateBuildStatus{State: "ready"}, nil
}

func (m *mockProvider) GetTemplateLogs(_ context.Context, _ string) (string, error) {
	return "", nil
}

func (m *mockProvider) DeleteTemplate(_ context.Context, _ string) error {
	return nil
}

func (m *mockProvider) SetAutoStop(_ context.Context, externalID string, intervalMinutes int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.setAutoStopCalls = append(m.setAutoStopCalls, autoPolicyCall{
		externalID:      externalID,
		intervalMinutes: intervalMinutes,
	})
	return nil
}

func (m *mockProvider) SetAutoArchive(_ context.Context, externalID string, intervalMinutes int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.setAutoArchiveCalls = append(m.setAutoArchiveCalls, autoPolicyCall{
		externalID:      externalID,
		intervalMinutes: intervalMinutes,
	})
	return nil
}

func (m *mockProvider) ArchiveSandbox(_ context.Context, externalID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	sb, ok := m.sandboxes[externalID]
	if !ok {
		return fmt.Errorf("sandbox not found: %s", externalID)
	}
	sb.status = StatusArchived
	m.archivedIDs = append(m.archivedIDs, externalID)
	return nil
}

func (m *mockProvider) ExecuteCommand(ctx context.Context, externalID string, command string) (string, error) {
	if m.executeCommandFn != nil {
		return m.executeCommandFn(ctx, externalID, command)
	}
	return "", nil
}

func (m *mockProvider) ExecuteCommandWithTimeout(ctx context.Context, externalID string, command string, _ time.Duration) (string, error) {
	return m.ExecuteCommand(ctx, externalID, command)
}

func (m *mockProvider) GetResourceUsage(ctx context.Context, externalID string) (*ResourceUsage, error) {
	if m.resourceUsageFn != nil {
		return m.resourceUsageFn(ctx, externalID)
	}
	if m.executeCommandFn != nil {
		output, err := m.executeCommandFn(ctx, externalID, "")
		if err != nil {
			return nil, err
		}
		return parseCgroupOutput(output)
	}
	return &ResourceUsage{
		MemoryLimitBytes:  1000,
		MemoryUsedBytes:   200,
		MemoryPeakBytes:   300,
		CPUQuota:          "100000 100000",
		CPUUsageUsec:      400,
		CPUThrottledCount: 5,
		PIDCount:          6,
	}, nil
}

func (m *mockProvider) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sandboxes)
}

// registerSandbox adds a sandbox to the mock so GetStatus/StopSandbox work on seeded DB records.
func (m *mockProvider) registerSandbox(externalID string, status SandboxStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sandboxes[externalID] = &mockSandbox{name: externalID, status: status}
}
