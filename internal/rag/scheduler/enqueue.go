package scheduler

import (
	"errors"
	"time"

	"github.com/hibiken/asynq"
)

func asynqUnique(ttl time.Duration) asynq.Option {
	return asynq.Unique(ttl)
}

func isDuplicate(err error) bool {
	return errors.Is(err, asynq.ErrDuplicateTask)
}
