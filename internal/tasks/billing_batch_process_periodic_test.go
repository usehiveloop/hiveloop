package tasks_test

import (
	"testing"

	"github.com/usehivy/hivy/internal/config"
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
