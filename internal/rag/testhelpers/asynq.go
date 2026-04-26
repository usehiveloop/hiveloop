package testhelpers

import (
	"testing"

	"github.com/hibiken/asynq"

	"github.com/usehiveloop/hiveloop/internal/enqueue"
)

// AsynqRedisOpt returns the asynq.RedisConnOpt the RAG tests use.
// Mirrors RedisAddr / RedisDB from redis.go.
func AsynqRedisOpt() asynq.RedisClientOpt {
	return asynq.RedisClientOpt{
		Addr: RedisAddr(),
		DB:   RedisDB(),
	}
}

// NewTestAsynqClient returns an asynq Client bound to the test Redis
// DB and registers t.Cleanup to close it. Tests use this where the
// scheduler scan functions take an enqueue.TaskEnqueuer argument.
func NewTestAsynqClient(t *testing.T) *enqueue.Client {
	t.Helper()
	cli := enqueue.NewClient(AsynqRedisOpt())
	t.Cleanup(func() { _ = cli.Close() })
	return cli
}

// NewTestAsynqInspector returns an Inspector bound to the test Redis
// DB. Tests use this to assert queue depth, list pending tasks, and
// inspect scheduler-enqueue events.
func NewTestAsynqInspector(t *testing.T) *asynq.Inspector {
	t.Helper()
	insp := asynq.NewInspector(AsynqRedisOpt())
	t.Cleanup(func() { _ = insp.Close() })
	return insp
}
