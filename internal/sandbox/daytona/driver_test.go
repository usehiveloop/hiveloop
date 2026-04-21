package daytona

import (
	"testing"

	"github.com/usehiveloop/hiveloop/internal/sandbox"
)

// TestDriverImplementsProvider verifies at compile time that Driver satisfies sandbox.Provider.
func TestDriverImplementsProvider(t *testing.T) {
	var _ sandbox.Provider = (*Driver)(nil)
}
