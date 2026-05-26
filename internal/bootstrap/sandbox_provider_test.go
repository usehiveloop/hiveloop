package bootstrap

import (
	"errors"
	"testing"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/sandbox"
)

func TestNewSandboxProviderEmptyProviderDisablesSandbox(t *testing.T) {
	_, err := newSandboxProvider(&config.Config{})
	if !errors.Is(err, errSandboxProviderNotConfigured) {
		t.Fatalf("newSandboxProvider error = %v, want errSandboxProviderNotConfigured", err)
	}
}

func TestNewSandboxProviderCreatesDaytonaWhenConfigured(t *testing.T) {
	provider, err := newSandboxProvider(&config.Config{
		SandboxProviderID:               sandbox.ProviderDaytona,
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

func TestNewSandboxProviderDaytonaWithoutCredentialsDisablesSandbox(t *testing.T) {
	_, err := newSandboxProvider(&config.Config{
		SandboxProviderID:               sandbox.ProviderDaytona,
		SpecialistSandboxRuntimeVersion: "v1.0.0",
	})
	if !errors.Is(err, errSandboxProviderNotConfigured) {
		t.Fatalf("newSandboxProvider error = %v, want errSandboxProviderNotConfigured", err)
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
