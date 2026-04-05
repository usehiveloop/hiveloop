package enqueue

import (
	"fmt"

	"github.com/hibiken/asynq"
)

// TaskEnqueuer is the interface for enqueuing Asynq tasks. HTTP handlers and
// middleware depend on this interface so that tests can inject a mock.
type TaskEnqueuer interface {
	Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
	Close() error
}

// Client wraps asynq.Client and implements TaskEnqueuer.
type Client struct {
	inner *asynq.Client
}

// NewClient creates a new enqueue client from Asynq Redis connection options.
func NewClient(redisOpt asynq.RedisConnOpt) *Client {
	return &Client{inner: asynq.NewClient(redisOpt)}
}

// Enqueue submits a task to the Asynq queue.
func (c *Client) Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	info, err := c.inner.Enqueue(task, opts...)
	if err != nil {
		return nil, fmt.Errorf("enqueue %s: %w", task.Type(), err)
	}
	return info, nil
}

// Close flushes and closes the underlying Asynq client.
func (c *Client) Close() error {
	return c.inner.Close()
}
