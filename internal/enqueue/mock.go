package enqueue

import (
	"context"
	"sync"
	"testing"

	"github.com/hibiken/asynq"
)

type MockClient struct {
	mu        sync.Mutex
	enqueued  []EnqueuedTask
	deleted   []DeletedTask
	DeleteErr error
}

type EnqueuedTask struct {
	TypeName string
	Payload  []byte
	Options  []asynq.Option
}

type DeletedTask struct {
	Queue string
	ID    string
}

func (m *MockClient) Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	return m.EnqueueContext(context.Background(), task, opts...)
}

func (m *MockClient) EnqueueContext(_ context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.enqueued = append(m.enqueued, EnqueuedTask{
		TypeName: task.Type(),
		Payload:  task.Payload(),
		Options:  append([]asynq.Option(nil), opts...),
	})
	return &asynq.TaskInfo{}, nil
}

func (m *MockClient) Close() error { return nil }

func (m *MockClient) DeleteTask(queue, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deleted = append(m.deleted, DeletedTask{Queue: queue, ID: id})
	return m.DeleteErr
}

func (m *MockClient) Tasks() []EnqueuedTask {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]EnqueuedTask, len(m.enqueued))
	copy(cp, m.enqueued)
	return cp
}

func (m *MockClient) DeletedTasks() []DeletedTask {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]DeletedTask, len(m.deleted))
	copy(cp, m.deleted)
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
	m.deleted = nil
}
