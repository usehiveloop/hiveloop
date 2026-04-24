package paystack

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain asserts that the Paystack adapter's tests don't leak goroutines.
// The adapter uses a net/http client but never spawns goroutines itself; the
// underlying http.Client idle connections are torn down at process exit.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
