package billing_test

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain asserts that the package's tests don't leak goroutines.
//
// The billing package is pure logic — no goroutines are spawned in production
// code, so any leak flagged here is a real bug in a test.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
