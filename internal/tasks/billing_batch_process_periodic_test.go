package tasks_test

import (
	"testing"
	"time"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/sandbox"
	"github.com/usehivy/hivy/internal/tasks"
)

func TestBatch_RegisteredAsPeriodicTask(t *testing.T) {
	configs := tasks.PeriodicTaskConfigs(&config.Config{}, nil)
	for _, c := range configs {
		if c.Task.Type() == tasks.TypeBillingBatchProcess {
			if c.Cronspec != "@every 30s" {
				t.Errorf("billing batch cronspec = %q, want @every 30s", c.Cronspec)
			}
			return
		}
	}
	t.Fatal("billing batch process not registered as a periodic task")
}

func TestPeriodicTaskConfigs_SkipsSandboxTasksWhenProviderIncomplete(t *testing.T) {
	configs := tasks.PeriodicTaskConfigs(&config.Config{
		SandboxProviderID:    sandbox.ProviderDaytona,
		SandboxEncryptionKey: "present",
	}, nil)

	for _, c := range configs {
		switch c.Task.Type() {
		case tasks.TypeSandboxHealthCheck, tasks.TypeSandboxResourceCheck, tasks.TypeSandboxLifecycle:
			t.Fatalf("sandbox task %q should not be registered without complete provider config", c.Task.Type())
		}
	}
}

func TestPeriodicTaskConfigs_RegistersSandboxTasksWhenProviderComplete(t *testing.T) {
	configs := tasks.PeriodicTaskConfigs(&config.Config{
		SandboxProviderID:                 sandbox.ProviderDocker,
		SandboxEncryptionKey:              "present",
		SandboxDockerPublicHost:           "host.docker.internal",
		SandboxResourceCheckInterval:      30 * time.Minute,
		SpecialistSandboxRuntimeVersion:   "v1.0.0",
		SandboxesRuntimeBaseImagePrefix:   "runtime",
		SandboxDockerContainerLabelPrefix: "hivy",
	}, nil)

	seen := map[string]bool{}
	for _, c := range configs {
		seen[c.Task.Type()] = true
	}
	if !seen[tasks.TypeSandboxHealthCheck] || !seen[tasks.TypeSandboxResourceCheck] || !seen[tasks.TypeSandboxLifecycle] {
		t.Fatalf("sandbox periodic tasks missing: %#v", seen)
	}
}
