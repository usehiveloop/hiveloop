package daytona

import (
	"testing"

	"github.com/llmvault/llmvault/internal/sandbox"
)

// TestDriverImplementsProvider verifies at compile time that Driver satisfies sandbox.Provider.
func TestDriverImplementsProvider(t *testing.T) {
	var _ sandbox.Provider = (*Driver)(nil)
}
