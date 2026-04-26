package testhelpers

import (
	"testing"

	"github.com/hibiken/asynq"

	"github.com/usehiveloop/hiveloop/internal/enqueue"
)

func AsynqRedisOpt() asynq.RedisClientOpt {
	return asynq.RedisClientOpt{
		Addr: RedisAddr(),
		DB:   RedisDB(),
	}
}

func NewTestAsynqClient(t *testing.T) *enqueue.Client {
	t.Helper()
	cli := enqueue.NewClient(AsynqRedisOpt())
	t.Cleanup(func() { _ = cli.Close() })
	return cli
}

func NewTestAsynqInspector(t *testing.T) *asynq.Inspector {
	t.Helper()
	insp := asynq.NewInspector(AsynqRedisOpt())
	t.Cleanup(func() { _ = insp.Close() })
	return insp
}
