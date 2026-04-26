// Rate-limit detection + retry helper.
//
// GitHub signals primary-rate-limit exhaustion with HTTP 403 plus
// `X-RateLimit-Remaining: 0` and an `X-RateLimit-Reset` epoch-second
// header pinning when the quota refreshes. (Secondary rate-limits use
// 429; we treat both the same way.)
//
// Port of backend/onyx/connectors/github/rate_limit_utils.py:13-25 — the
// 60-second margin past `reset_at` matches Onyx's
// sleep_after_rate_limit_exception.
package github

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"
)

// rateLimitMargin matches Onyx's `+ 60` at rate_limit_utils.py:21 — sleep
// past the reset epoch to avoid racing GitHub's clock.
const rateLimitMargin = 60 * time.Second

// rateLimitMarginForTest is the margin actually used by withRateLimitRetry.
// In production this is rateLimitMargin (60s); tests override it via
// withRateLimitRetryWithMargin to keep retry timing in the millisecond
// range. Kept as a var rather than reading from the constant directly so
// production callers stay constant-time.
var rateLimitMarginForTest = rateLimitMargin

// rateLimitMaxAttempts caps how many times withRateLimitRetry will
// re-issue the same request before giving up. Five is generous: at the
// 60-min reset cadence that's a 5+ hour stuck-loop, way more than any
// realistic transient quota event.
const rateLimitMaxAttempts = 5

// errRateLimited is the sentinel returned by detectRateLimit + threaded
// through withRateLimitRetry. Callers that want to identify exhaustion
// after retries are spent can errors.Is it from the wrapped error.
var errRateLimited = errors.New("github: rate limit exhausted after retries")

// detectRateLimit returns (resetAt, true) when the response signals
// rate-limit exhaustion. Both 403 and 429 with `X-RateLimit-Remaining: 0`
// qualify; a plain 403 without those headers is a perms error and
// returns (zero, false).
//
// resetAt is the moment the quota refreshes (server-side epoch). The
// caller adds rateLimitMargin before sleeping.
func detectRateLimit(status int, headers http.Header) (time.Time, bool) {
	if status != http.StatusForbidden && status != http.StatusTooManyRequests {
		return time.Time{}, false
	}
	if headers.Get("X-RateLimit-Remaining") != "0" {
		return time.Time{}, false
	}
	resetSec, err := strconv.ParseInt(headers.Get("X-RateLimit-Reset"), 10, 64)
	if err != nil {
		// Header missing or malformed — bail; caller treats as a real
		// 403 rather than a quota event.
		return time.Time{}, false
	}
	return time.Unix(resetSec, 0), true
}

// withRateLimitRetry runs op up to rateLimitMaxAttempts times, sleeping
// past the reset epoch between attempts when op returns rateLimitError.
// Non-rate-limit errors propagate immediately on the first attempt — we
// don't retry generic 5xx here because Nango/GitHub already retry their
// own way and our transient-error budget is owned at a higher layer.
//
// The first attempt is unconditional; on rate-limit the helper sleeps
// until time.Now() ≥ resetAt + rateLimitMargin, then re-runs. ctx
// cancellation cancels the sleep + propagates as ctx.Err().
func withRateLimitRetry(ctx context.Context, op func() error) error {
	return withRateLimitRetryWithMargin(ctx, op, rateLimitMarginForTest)
}

// withRateLimitRetryWithMargin is the test-friendly form. Production
// always passes the package default; tests pass a small override.
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

// rateLimitError carries the resetAt timestamp from detectRateLimit
// through op's error so withRateLimitRetry can sleep precisely.
type rateLimitError struct {
	resetAt time.Time
}

func (e *rateLimitError) Error() string {
	return "github: rate limited until " + e.resetAt.UTC().Format(time.RFC3339)
}
