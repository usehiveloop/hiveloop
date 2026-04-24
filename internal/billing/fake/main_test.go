package fake_test

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain asserts that the fake billing provider's tests don't leak
// goroutines. The fake has no goroutines of its own; a leak would be a bug in
// a test helper.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
