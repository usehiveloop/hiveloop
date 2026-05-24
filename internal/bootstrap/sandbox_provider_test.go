package bootstrap

import (
	"testing"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/sandbox"
)

func TestNewSandboxProviderDefaultsToDaytona(t *testing.T) {
	provider, err := newSandboxProvider(&config.Config{
		DaytonaAPIKey:                   "test-key",
		SpecialistSandboxRuntimeVersion: "v1.0.0",
	})
	if err != nil {
		t.Fatalf("newSandboxProvider: %v", err)
	}
	if provider.ID() != sandbox.ProviderDaytona {
		t.Fatalf("provider ID = %q, want %q", provider.ID(), sandbox.ProviderDaytona)
	}
}

func TestNewSandboxProviderRejectsUnknownProvider(t *testing.T) {
	_, err := newSandboxProvider(&config.Config{
		SandboxProviderID:               "unknown",
		DaytonaAPIKey:                   "test-key",
		SpecialistSandboxRuntimeVersion: "v1.0.0",
	})
	if err == nil {
		t.Fatal("expected unsupported provider error")
	}
}
