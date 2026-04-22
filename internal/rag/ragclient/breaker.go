package ragclient

import (
	"errors"
	"time"

	"github.com/sony/gobreaker/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// breakerConsecutiveFailThreshold — after this many consecutive failures,
// the breaker opens and short-circuits further calls.
const breakerConsecutiveFailThreshold uint32 = 5

// breakerOpenDuration — time to wait before transitioning from Open to
// Half-Open, at which point one probe call is allowed through.
const breakerOpenDuration = 30 * time.Second

// breakerCountingWindow — interval at which the breaker resets its
// internal counts in the Closed state. This matches "5 consecutive
// failures in 30s" — counts reset every 30s so a slow trickle of
// unrelated failures doesn't trip the breaker.
const breakerCountingWindow = 30 * time.Second

// newBreaker constructs the breaker used by a Client. The name is used
// only in gobreaker internal state changes (logged via OnStateChange).
func newBreaker(name string) *gobreaker.CircuitBreaker[any] {
	settings := gobreaker.Settings{
		Name:     name,
		Interval: breakerCountingWindow,
		Timeout:  breakerOpenDuration,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= breakerConsecutiveFailThreshold
		},
		// IsSuccessful is called with the invocation error. Returning
		// true treats the error as a success for breaker-accounting
		// purposes (it will NOT increment ConsecutiveFailures). We use
		// this to keep caller-bug errors (INVALID_ARGUMENT, NOT_FOUND,
		// UNAUTHENTICATED) from opening the breaker — those reflect a
		// broken CALLER, not a broken service.
		IsSuccessful: isBreakerSuccessful,
	}
	return gobreaker.NewCircuitBreaker[any](settings)
}

// isBreakerSuccessful returns true when err should NOT count against
// the breaker. Rule of thumb: only fail the breaker on codes that
// indicate the service itself is unhealthy (UNAVAILABLE, INTERNAL,
// DEADLINE_EXCEEDED). All other gRPC codes reflect caller-supplied bad
// input or authoritative server answers and should not open the breaker.
func isBreakerSuccessful(err error) bool {
	if err == nil {
		return true
	}
	st, ok := status.FromError(err)
	if !ok {
		// Non-gRPC errors (dial failures bubbling up, context cancellation).
		// A dial failure that manifests as a non-status error still deserves
		// to fail the breaker — the service is not reachable.
		return false
	}
	switch st.Code() {
	case codes.Unavailable, codes.Internal, codes.DeadlineExceeded:
		return false
	default:
		return true
	}
}

// ErrCircuitOpen is returned when the breaker is Open and rejects a call
// before it touches the network. Callers can assert on errors.Is.
var ErrCircuitOpen = errors.New("ragclient: circuit breaker is open")

// mapBreakerError normalizes gobreaker's two sentinel errors into our
// public ErrCircuitOpen so callers don't need to import gobreaker.
func mapBreakerError(err error) error {
	if errors.Is(err, gobreaker.ErrOpenState) ||
		errors.Is(err, gobreaker.ErrTooManyRequests) {
		return ErrCircuitOpen
	}
	return err
}
