package scheduler

import (
	"errors"
	"time"

	"github.com/hibiken/asynq"
)

// asynqUnique returns the asynq.Unique option with the given TTL. Wrapped
// here so the loop files don't take a direct dependency on asynq's
// option surface beyond what they actually need.
func asynqUnique(ttl time.Duration) asynq.Option {
	return asynq.Unique(ttl)
}

// isDuplicate returns true when err is asynq.ErrDuplicateTask — that is,
// asynq.Unique blocked the enqueue because an identical task is already
// in flight within the TTL window. For our scan contract that's the
// success case (we only need exactly one in-flight per source).
func isDuplicate(err error) bool {
	return errors.Is(err, asynq.ErrDuplicateTask)
}
