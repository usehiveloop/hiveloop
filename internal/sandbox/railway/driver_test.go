package railway

import (
	"testing"
	"time"
)

func TestRuntimeCommandHTTPTimeoutExceedsCommandTimeout(t *testing.T) {
	commandTimeout := 60 * time.Minute

	got := runtimeCommandHTTPTimeout(commandTimeout)
	want := commandTimeout + runtimeCommandHTTPTimeoutPadding

	if got != want {
		t.Fatalf("runtime command HTTP timeout = %s, want %s", got, want)
	}
}

func TestRuntimeCommandHTTPTimeoutUsesRuntimeDefault(t *testing.T) {
	got := runtimeCommandHTTPTimeout(0)
	want := 120*time.Second + runtimeCommandHTTPTimeoutPadding

	if got != want {
		t.Fatalf("runtime command HTTP timeout = %s, want %s", got, want)
	}
}
