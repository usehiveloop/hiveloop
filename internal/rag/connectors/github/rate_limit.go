package github

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"
)

// rateLimitMargin sleeps past the reset epoch to avoid racing GitHub's clock.
const rateLimitMargin = 60 * time.Second

// rateLimitMarginForTest is a var (not a constant) so tests can shrink
// the sleep into the millisecond range.
var rateLimitMarginForTest = rateLimitMargin

const rateLimitMaxAttempts = 5

var errRateLimited = errors.New("github: rate limit exhausted after retries")

// detectRateLimit returns (zero, false) for a plain 403 without the
// rate-limit headers — that's a perms error, not a quota event.
func detectRateLimit(status int, headers http.Header) (time.Time, bool) {
	if status != http.StatusForbidden && status != http.StatusTooManyRequests {
		return time.Time{}, false
	}
	if headers.Get("X-RateLimit-Remaining") != "0" {
		return time.Time{}, false
	}
	resetSec, err := strconv.ParseInt(headers.Get("X-RateLimit-Reset"), 10, 64)
	if err != nil {
		return time.Time{}, false
	}
	return time.Unix(resetSec, 0), true
}

func withRateLimitRetry(ctx context.Context, op func() error) error {
	return withRateLimitRetryWithMargin(ctx, op, rateLimitMarginForTest)
}

func withRateLimitRetryWithMargin(ctx context.Context, op func() error, margin time.Duration) error {
	var lastErr error
	for attempt := 0; attempt < rateLimitMaxAttempts; attempt++ {
		err := op()
		if err == nil {
			return nil
		}
		var rl *rateLimitError
		if !errors.As(err, &rl) {
			return err
		}
		lastErr = err

		wakeAt := rl.resetAt.Add(margin)
		sleep := time.Until(wakeAt)
		if sleep <= 0 {
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(sleep):
		}
	}
	if lastErr == nil {
		return errRateLimited
	}
	return errors.Join(errRateLimited, lastErr)
}

type rateLimitError struct {
	resetAt time.Time
}

func (e *rateLimitError) Error() string {
	return "github: rate limited until " + e.resetAt.UTC().Format(time.RFC3339)
}
