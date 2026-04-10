package sandbox

import (
	"context"
	"fmt"
	"sync"
)

// mockProvider is an in-memory sandbox.Provider for testing.
type mockProvider struct {
	mu               sync.Mutex
	sandboxes        map[string]*mockSandbox
	endpoints        map[string]string // externalID → URL
	endpointOverride string            // if set, all GetEndpoint calls return this URL
	nextID           int
	executeCommandFn func(ctx context.Context, externalID, command string) (string, error)
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

func (m *mockProvider) CreateSandbox(_ context.Context, opts CreateSandboxOpts) (*SandboxInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.nextID++
	id := fmt.Sprintf("mock-sb-%d", m.nextID)
	m.sandboxes[id] = &mockSandbox{name: opts.Name, status: StatusRunning}
	m.endpoints[id] = fmt.Sprintf("https://mock-sandbox-%d.test:25434", m.nextID)

	return &SandboxInfo{ExternalID: id, Status: StatusRunning}, nil
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

	if m.endpointOverride != "" {
		return m.endpointOverride, nil
	}

	url, ok := m.endpoints[externalID]
	if !ok {
		return "", fmt.Errorf("sandbox not found: %s", externalID)
	}
	return url, nil
}

func (m *mockProvider) BuildSnapshot(_ context.Context, _ BuildSnapshotOpts) (string, error) {
	return "mock-snapshot-id", nil
}

func (m *mockProvider) BuildSnapshotWithLogs(_ context.Context, _ BuildSnapshotOpts, _ func(string)) (string, error) {
	return "mock-snapshot-id", nil
}

func (m *mockProvider) DeleteSnapshot(_ context.Context, _ string) error {
	return nil
}

func (m *mockProvider) SetAutoStop(_ context.Context, _ string, _ int) error {
	return nil
}

func (m *mockProvider) ExecuteCommand(ctx context.Context, externalID string, command string) (string, error) {
	if m.executeCommandFn != nil {
		return m.executeCommandFn(ctx, externalID, command)
	}
	return "", nil
}

// --- helpers for assertions ---

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

func (m *mockProvider) getStatus(externalID string) SandboxStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	sb, ok := m.sandboxes[externalID]
	if !ok {
		return StatusError
	}
	return sb.status
}
