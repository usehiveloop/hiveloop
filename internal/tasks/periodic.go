package tasks

import (
	"fmt"
	"time"

	"github.com/hibiken/asynq"

	"github.com/usehiveloop/hiveloop/internal/config"
	"github.com/usehiveloop/hiveloop/internal/rag/scheduler"
)

func PeriodicTaskConfigs(cfg *config.Config, ragSched *scheduler.Deps) []*asynq.PeriodicTaskConfig {
	configs := []*asynq.PeriodicTaskConfig{
		{
			Cronspec: "0 */6 * * *", // every 6 hours
			Task:     asynq.NewTask(TypeTokenCleanup, nil),
			Opts:     []asynq.Option{asynq.Queue(QueuePeriodic), asynq.MaxRetry(2), asynq.Timeout(2 * time.Minute)},
		},
		{
			Cronspec: "@every 5m",
			Task:     asynq.NewTask(TypeStreamCleanup, nil),
			Opts:     []asynq.Option{asynq.Queue(QueuePeriodic), asynq.MaxRetry(1), asynq.Timeout(2 * time.Minute)},
		},
		{
			Cronspec: "@every 30s",
			Task:     asynq.NewTask(TypeCronTriggerPoll, nil),
			Opts:     []asynq.Option{asynq.Queue(QueuePeriodic), asynq.MaxRetry(1), asynq.Timeout(25 * time.Second)},
		},
		{
			Cronspec: "@every 15m",
			Task:     asynq.NewTask(TypeCreditsExpire, nil),
			Opts:     []asynq.Option{asynq.Queue(QueuePeriodic), asynq.MaxRetry(2), asynq.Timeout(10 * time.Minute)},
		},
		{
			// Subscription renewal sweep. The handler enqueues per-sub
			// renewal tasks which own the attempt counter — at most one
			// attempt per subscription per RenewalRetryInterval is
			// dispatched because the sweep query filters on
			// last_renewal_attempt_at.
			Cronspec: "@every 1h",
			Task:     asynq.NewTask(TypeBillingRenewSweep, nil),
			Opts:     []asynq.Option{asynq.Queue(QueuePeriodic), asynq.MaxRetry(1), asynq.Timeout(5 * time.Minute)},
		},
	}

	// Sandbox tasks only if orchestrator is configured
	if cfg.SandboxProviderKey != "" && cfg.SandboxEncryptionKey != "" {
		configs = append(configs, &asynq.PeriodicTaskConfig{
			Cronspec: "@every 30s",
			Task:     asynq.NewTask(TypeSandboxHealthCheck, nil),
			Opts:     []asynq.Option{asynq.Queue(QueuePeriodic), asynq.MaxRetry(1), asynq.Timeout(time.Minute)},
		})

		interval := cfg.SandboxResourceCheckInterval
		if interval > 0 {
			configs = append(configs, &asynq.PeriodicTaskConfig{
				Cronspec: fmt.Sprintf("@every %s", interval),
				Task:     asynq.NewTask(TypeSandboxResourceCheck, nil),
				Opts:     []asynq.Option{asynq.Queue(QueuePeriodic), asynq.MaxRetry(1), asynq.Timeout(5 * time.Minute)},
			})
		}

		// Sandbox lifecycle policy: runs every 5 minutes. Stops sandboxes idle
		// for >10 min and archives sandboxes stopped for >24 h. Timeout
		// generous enough to stop/archive dozens of sandboxes in a single tick.
		configs = append(configs, &asynq.PeriodicTaskConfig{
			Cronspec: "@every 5m",
			Task:     asynq.NewTask(TypeSandboxLifecycle, nil),
			Opts:     []asynq.Option{asynq.Queue(QueuePeriodic), asynq.MaxRetry(1), asynq.Timeout(10 * time.Minute)},
		})
	}

	if ragSched != nil {
		configs = append(configs, ragSched.Configs()...)
	}
	return configs
}
