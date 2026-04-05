package enqueue

import (
	"sync"
	"testing"

	"github.com/hibiken/asynq"
)

// MockClient implements TaskEnqueuer for tests. It captures all enqueued tasks
// without needing a Redis connection.
type MockClient struct {
	mu       sync.Mutex
	enqueued []EnqueuedTask
}

// EnqueuedTask records a single enqueue call.
type EnqueuedTask struct {
	TypeName string
	Payload  []byte
}

// Enqueue captures the task for later assertions.
func (m *MockClient) Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.enqueued = append(m.enqueued, EnqueuedTask{
		TypeName: task.Type(),
		Payload:  task.Payload(),
	})
	return &asynq.TaskInfo{}, nil
}

// Close is a no-op for the mock.
func (m *MockClient) Close() error { return nil }

// Tasks returns all captured enqueued tasks.
func (m *MockClient) Tasks() []EnqueuedTask {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]EnqueuedTask, len(m.enqueued))
	copy(cp, m.enqueued)
	return cp
}

// AssertEnqueued asserts that at least one task of the given type was enqueued.
func (m *MockClient) AssertEnqueued(t *testing.T, taskType string) {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.enqueued {
		if e.TypeName == taskType {
			return
		}
	}
	t.Errorf("expected task %q to be enqueued, but it was not", taskType)
}

// Flush clears all captured tasks.
func (m *MockClient) Flush() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.enqueued = nil
}
