package enqueue

import (
	"context"
	"sync"
	"testing"

	"github.com/hibiken/asynq"
)

type MockClient struct {
	mu       sync.Mutex
	enqueued []EnqueuedTask
}

type EnqueuedTask struct {
	TypeName string
	Payload  []byte
}

func (m *MockClient) Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	return m.EnqueueContext(context.Background(), task, opts...)
}

func (m *MockClient) EnqueueContext(_ context.Context, task *asynq.Task, _ ...asynq.Option) (*asynq.TaskInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.enqueued = append(m.enqueued, EnqueuedTask{
		TypeName: task.Type(),
		Payload:  task.Payload(),
	})
	return &asynq.TaskInfo{}, nil
}

func (m *MockClient) Close() error { return nil }

func (m *MockClient) Tasks() []EnqueuedTask {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]EnqueuedTask, len(m.enqueued))
	copy(cp, m.enqueued)
	return cp
}

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

func (m *MockClient) Flush() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.enqueued = nil
}
