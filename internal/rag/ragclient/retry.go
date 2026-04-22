package ragclient

import (
	"context"
	"math/rand"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// rpcCall is the type-erased shape of a single RPC invocation that retry
// wraps. Retries are capped by Config.MaxRetries (attempts = retries+1)
// and use exponential backoff 100ms → 200ms → 400ms → ... with up to
// 25% jitter to avoid synchronized client retries.
type rpcCall func(ctx context.Context) error

// retryPolicy carries the per-RPC retry decision.
type retryPolicy struct {
	// maxAttempts is the TOTAL number of attempts including the first.
	// maxAttempts == 1 disables retry entirely.
	maxAttempts int
	// retryOnDeadlineExceeded lets idempotent RPCs retry when the
	// server didn't respond in time. Non-idempotent RPCs (Search)
	// must NOT retry DEADLINE_EXCEEDED: the request may have completed
	// server-side.
	retryOnDeadlineExceeded bool
}

// idempotentPolicy is used for IngestBatch, UpdateACL, DeleteByDocID,
// DeleteByOrg, Prune, CreateDataset, DropDataset — all of which carry
// idempotency keys or are naturally safe to replay.
func idempotentPolicy(maxRetries int) retryPolicy {
	return retryPolicy{
		maxAttempts:             maxRetries + 1,
		retryOnDeadlineExceeded: true,
	}
}

// nonIdempotentPolicy is used for Search and Health: retry only on
// UNAVAILABLE (nothing was sent to the app layer), never on timeouts.
func nonIdempotentPolicy(maxRetries int) retryPolicy {
	return retryPolicy{
		maxAttempts:             maxRetries + 1,
		retryOnDeadlineExceeded: false,
	}
}

// shouldRetry returns true iff the error is one the policy considers
// safe to replay.
func (p retryPolicy) shouldRetry(err error) bool {
	if err == nil {
		return false
	}
	st, ok := status.FromError(err)
	if !ok {
		// Non-gRPC errors (dial failure wrapped elsewhere, breaker open):
		// retry decision lives at the caller layer.
		return false
	}
	switch st.Code() {
	case codes.Unavailable:
		return true
	case codes.DeadlineExceeded:
		return p.retryOnDeadlineExceeded
	default:
		return false
	}
}

// backoff returns the wait duration for attempt N (1-indexed). Attempt 1
// returns 0 (the first call happens immediately). Uses 100ms base and
// doubling: 100, 200, 400, 800, ... plus up to 25% jitter.
func backoff(attempt int, rng *rand.Rand) time.Duration {
	if attempt <= 1 {
		return 0
	}
	base := 100 * time.Millisecond
	d := base << (attempt - 2) // attempt=2 → 100ms, attempt=3 → 200ms
	jitter := time.Duration(rng.Int63n(int64(d / 4)))
	return d + jitter
}

// runWithRetry invokes call up to policy.maxAttempts times, sleeping via
// backoff between attempts. The context is checked both before each
// attempt and during the backoff sleep so caller cancellation short-circuits.
// Returns the final error (from the last attempt) so the caller sees the
// real server status, not a synthetic retry-exhausted wrapper.
func runWithRetry(ctx context.Context, policy retryPolicy, rng *rand.Rand, call rpcCall) error {
	var lastErr error
	for attempt := 1; attempt <= policy.maxAttempts; attempt++ {
		if attempt > 1 {
			wait := backoff(attempt, rng)
			if wait > 0 {
				timer := time.NewTimer(wait)
				select {
				case <-ctx.Done():
					timer.Stop()
					return ctx.Err()
				case <-timer.C:
				}
			}
		}
		err := call(ctx)
		if err == nil {
			return nil
		}
		lastErr = err
		if !policy.shouldRetry(err) {
			return err
		}
	}
	return lastErr
}
