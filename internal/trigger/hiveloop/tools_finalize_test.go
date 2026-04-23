package hiveloop

import (
	"context"
	"strings"
	"testing"
)

func TestFinalize(t *testing.T) {
	handler := NewFinalizeHandler()
	result, done, err := handler(context.Background(), "call-1", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !done {
		t.Error("finalize should signal done")
	}
	if !strings.Contains(result, "✓") {
		t.Errorf("expected success marker: %q", result)
	}
}
