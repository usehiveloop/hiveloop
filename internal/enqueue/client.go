package enqueue

import (
	"context"
	"errors"
	"fmt"

	"github.com/hibiken/asynq"

	sentryobs "github.com/usehiveloop/hiveloop/internal/observability/sentry"
)

type TaskEnqueuer interface {
	Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
	EnqueueContext(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
	Close() error
}

type TaskCleaner interface {
	DeleteTask(queue, id string) error
}

type Client struct {
	asynqClient *asynq.Client
	inspector   *asynq.Inspector
}

func NewClient(redisOpt asynq.RedisConnOpt) *Client {
	return &Client{
		asynqClient: asynq.NewClient(redisOpt),
		inspector:   asynq.NewInspector(redisOpt),
	}
}

func (c *Client) Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	return c.EnqueueContext(context.Background(), task, opts...)
}

func (c *Client) EnqueueContext(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	destinationQueue := destinationQueueFromOptions(opts)
	enqueueSpan := sentryobs.StartEnqueueSpan(ctx, task.Type(), destinationQueue)

	payloadWithTrace := sentryobs.WrapPayloadWithCurrentTrace(ctx, task.Payload())
	taskToEnqueue := task
	if len(payloadWithTrace) != len(task.Payload()) {
		taskToEnqueue = asynq.NewTask(task.Type(), payloadWithTrace)
	}

	info, err := c.asynqClient.EnqueueContext(ctx, taskToEnqueue, opts...)
	sentryobs.FinishEnqueueSpan(ctx, enqueueSpan, info, err)
	if err != nil {
		return nil, fmt.Errorf("enqueue %s: %w", task.Type(), err)
	}
	return info, nil
}

func (c *Client) DeleteTask(queue, id string) error {
	if err := c.inspector.DeleteTask(queue, id); err != nil {
		return fmt.Errorf("delete task %s/%s: %w", queue, id, err)
	}
	return nil
}

func (c *Client) Close() error {
	return errors.Join(c.asynqClient.Close(), c.inspector.Close())
}

func destinationQueueFromOptions(opts []asynq.Option) string {
	for _, opt := range opts {
		if opt.Type() != asynq.QueueOpt {
			continue
		}
		if name, ok := opt.Value().(string); ok {
			return name
		}
	}
	return ""
}
