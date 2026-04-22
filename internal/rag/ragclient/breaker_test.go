package ragclient

import (
	"errors"
	"testing"

	"github.com/sony/gobreaker/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestIsBreakerSuccessful_NilSucceeds(t *testing.T) {
	if !isBreakerSuccessful(nil) {
		t.Fatal("nil error must count as success")
	}
}

func TestIsBreakerSuccessful_UnhealthyCodesFail(t *testing.T) {
	for _, code := range []codes.Code{codes.Unavailable, codes.Internal, codes.DeadlineExceeded} {
		err := status.Error(code, "boom")
		if isBreakerSuccessful(err) {
			t.Fatalf("code %v must fail the breaker", code)
		}
	}
}

func TestIsBreakerSuccessful_CallerBugsDoNotTrip(t *testing.T) {
	// INVALID_ARGUMENT / NOT_FOUND / UNAUTHENTICATED are caller bugs,
	// not service health signals. They must not trip the breaker.
	for _, code := range []codes.Code{
		codes.InvalidArgument,
		codes.NotFound,
		codes.Unauthenticated,
		codes.PermissionDenied,
		codes.AlreadyExists,
	} {
		err := status.Error(code, "x")
		if !isBreakerSuccessful(err) {
			t.Fatalf("code %v must NOT fail the breaker", code)
		}
	}
}

func TestIsBreakerSuccessful_NonStatusErrorFails(t *testing.T) {
	// Dial / transport errors that aren't gRPC statuses indicate
	// the service is unreachable and should trip the breaker.
	if isBreakerSuccessful(errors.New("dial: connection refused")) {
		t.Fatal("non-status error must fail the breaker")
	}
}

func TestMapBreakerError_PassesThroughRealErrors(t *testing.T) {
	err := status.Error(codes.Internal, "boom")
	got := mapBreakerError(err)
	if got != err {
		t.Fatalf("mapBreakerError mutated non-breaker error: got %v want %v", got, err)
	}
	// Nil passes through unchanged.
	if mapBreakerError(nil) != nil {
		t.Fatal("nil should stay nil")
	}
}

func TestMapBreakerError_MapsOpenAndTooManyRequests(t *testing.T) {
	if got := mapBreakerError(gobreaker.ErrOpenState); !errors.Is(got, ErrCircuitOpen) {
		t.Fatalf("ErrOpenState → %v, want ErrCircuitOpen", got)
	}
	if got := mapBreakerError(gobreaker.ErrTooManyRequests); !errors.Is(got, ErrCircuitOpen) {
		t.Fatalf("ErrTooManyRequests → %v, want ErrCircuitOpen", got)
	}
}

func TestNewBreaker_TripsAfterThresholdConsecutiveFailures(t *testing.T) {
	// Drive the breaker directly to exercise the ReadyToTrip closure
	// bundled into newBreaker. We feed the classified-failure path
	// (UNAVAILABLE) so IsSuccessful=false and the counter increments.
	br := newBreaker("test")
	fail := func() (any, error) { return nil, status.Error(codes.Unavailable, "down") }

	// Feed N-1 failures; breaker stays closed.
	for i := uint32(0); i < breakerConsecutiveFailThreshold-1; i++ {
		_, _ = br.Execute(fail)
	}
	if br.State() != gobreaker.StateClosed {
		t.Fatalf("state = %v after %d failures, want Closed", br.State(), breakerConsecutiveFailThreshold-1)
	}
	// Nth failure flips the breaker open.
	_, _ = br.Execute(fail)
	if br.State() != gobreaker.StateOpen {
		t.Fatalf("state = %v after %d failures, want Open", br.State(), breakerConsecutiveFailThreshold)
	}

	// Subsequent calls short-circuit.
	_, err := br.Execute(fail)
	if !errors.Is(err, gobreaker.ErrOpenState) {
		t.Fatalf("open-state call returned %v, want ErrOpenState", err)
	}
	if mapped := mapBreakerError(err); !errors.Is(mapped, ErrCircuitOpen) {
		t.Fatalf("mapped err = %v, want ErrCircuitOpen", mapped)
	}
}

func TestNewBreaker_CallerBugsDoNotOpen(t *testing.T) {
	br := newBreaker("test-caller-bug")
	fail := func() (any, error) { return nil, status.Error(codes.InvalidArgument, "bad") }
	for i := uint32(0); i < breakerConsecutiveFailThreshold*2; i++ {
		_, _ = br.Execute(fail)
	}
	if br.State() != gobreaker.StateClosed {
		t.Fatalf("state = %v, want Closed — caller bugs must not trip", br.State())
	}
}
