package ragclient

import (
	"context"
	"errors"
	"math/rand"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// newTestRand returns a deterministic *rand.Rand for reproducible jitter
// timing in tests.
func newTestRand() *rand.Rand {
	return rand.New(rand.NewSource(1))
}

// --- shouldRetry ------------------------------------------------------

func TestShouldRetry_UnavailableAlwaysRetries(t *testing.T) {
	err := status.Error(codes.Unavailable, "nope")
	if !idempotentPolicy(2).shouldRetry(err) {
		t.Fatal("idempotent: UNAVAILABLE should retry")
	}
	if !nonIdempotentPolicy(2).shouldRetry(err) {
		t.Fatal("non-idempotent: UNAVAILABLE should retry")
	}
}

func TestShouldRetry_DeadlineExceededOnlyForIdempotent(t *testing.T) {
	err := status.Error(codes.DeadlineExceeded, "slow")
	if !idempotentPolicy(2).shouldRetry(err) {
		t.Fatal("idempotent policy must retry DEADLINE_EXCEEDED")
	}
	if nonIdempotentPolicy(2).shouldRetry(err) {
		t.Fatal("non-idempotent policy must NOT retry DEADLINE_EXCEEDED")
	}
}

func TestShouldRetry_NotRetriedOnInvalidArgument(t *testing.T) {
	err := status.Error(codes.InvalidArgument, "bad")
	if idempotentPolicy(3).shouldRetry(err) {
		t.Fatal("must not retry INVALID_ARGUMENT")
	}
}

func TestShouldRetry_NotRetriedOnUnauthenticated(t *testing.T) {
	err := status.Error(codes.Unauthenticated, "no")
	if idempotentPolicy(3).shouldRetry(err) {
		t.Fatal("must not retry UNAUTHENTICATED")
	}
}

func TestShouldRetry_NilError(t *testing.T) {
	if idempotentPolicy(3).shouldRetry(nil) {
		t.Fatal("nil must not retry")
	}
}

func TestShouldRetry_NonStatusError(t *testing.T) {
	// A raw error that's not a grpc status — breaker/caller deals with it.
	if idempotentPolicy(3).shouldRetry(errors.New("boom")) {
		t.Fatal("non-status errors must not retry inside the retry loop")
	}
}

// --- backoff ---------------------------------------------------------

func TestBackoff_FirstAttemptIsZero(t *testing.T) {
	if got := backoff(1, newTestRand()); got != 0 {
		t.Fatalf("attempt 1 backoff = %v, want 0", got)
	}
}

func TestBackoff_GrowsExponentially(t *testing.T) {
	// attempt 2 → 100ms base, attempt 3 → 200ms, attempt 4 → 400ms.
	tests := []struct {
		attempt int
		min     time.Duration
		max     time.Duration
	}{
		{2, 100 * time.Millisecond, 125 * time.Millisecond},
		{3, 200 * time.Millisecond, 250 * time.Millisecond},
		{4, 400 * time.Millisecond, 500 * time.Millisecond},
	}
	for _, tc := range tests {
		got := backoff(tc.attempt, newTestRand())
		if got < tc.min || got > tc.max {
			t.Fatalf("attempt %d backoff = %v, want in [%v,%v]", tc.attempt, got, tc.min, tc.max)
		}
	}
}

// --- runWithRetry ----------------------------------------------------

func TestRunWithRetry_SucceedsFirstAttempt(t *testing.T) {
	calls := 0
	err := runWithRetry(context.Background(), idempotentPolicy(3), newTestRand(), func(_ context.Context) error {
		calls++
		return nil
	})
	if err != nil || calls != 1 {
		t.Fatalf("want 1 call, no err. got calls=%d err=%v", calls, err)
	}
}

func TestRunWithRetry_RetriesOnUnavailableThenSucceeds(t *testing.T) {
	calls := 0
	err := runWithRetry(context.Background(), idempotentPolicy(3), newTestRand(), func(_ context.Context) error {
		calls++
		if calls < 3 {
			return status.Error(codes.Unavailable, "boom")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("final err = %v, want nil", err)
	}
	if calls != 3 {
		t.Fatalf("calls = %d, want 3", calls)
	}
}

func TestRunWithRetry_NoRetryOnNonRetryableCode(t *testing.T) {
	calls := 0
	err := runWithRetry(context.Background(), idempotentPolicy(5), newTestRand(), func(_ context.Context) error {
		calls++
		return status.Error(codes.InvalidArgument, "nope")
	})
	if calls != 1 {
		t.Fatalf("calls = %d, want 1 (no retry on INVALID_ARGUMENT)", calls)
	}
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("err code = %v, want InvalidArgument", status.Code(err))
	}
}

func TestRunWithRetry_ExhaustsAttemptsReturnsLastError(t *testing.T) {
	calls := 0
	err := runWithRetry(context.Background(), idempotentPolicy(2), newTestRand(), func(_ context.Context) error {
		calls++
		return status.Error(codes.Unavailable, "still down")
	})
	// 2 retries + initial = 3 attempts.
	if calls != 3 {
		t.Fatalf("calls = %d, want 3 (1 initial + 2 retries)", calls)
	}
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("err code = %v, want Unavailable", status.Code(err))
	}
}

func TestRunWithRetry_ContextCancelledDuringBackoff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	done := make(chan error, 1)
	go func() {
		done <- runWithRetry(ctx, idempotentPolicy(5), newTestRand(), func(_ context.Context) error {
			calls++
			return status.Error(codes.Unavailable, "down")
		})
	}()
	// Let the first attempt happen, then cancel while sleeping backoff.
	time.Sleep(20 * time.Millisecond)
	cancel()
	err := <-done
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	// Expect exactly 1 attempt to have landed before cancellation aborted the backoff.
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestRunWithRetry_ZeroMaxAttemptsMakesNoCalls(t *testing.T) {
	// Guard the "policy with maxAttempts=0 should be a no-op" edge case.
	calls := 0
	err := runWithRetry(context.Background(), retryPolicy{maxAttempts: 0}, newTestRand(), func(_ context.Context) error {
		calls++
		return nil
	})
	if err != nil {
		t.Fatalf("err = %v, want nil (no calls made)", err)
	}
	if calls != 0 {
		t.Fatalf("calls = %d, want 0", calls)
	}
}
