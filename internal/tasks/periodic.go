package tasks

import (
	"fmt"
	"time"

	"github.com/hibiken/asynq"

	"github.com/usehiveloop/hiveloop/internal/config"
)

// PeriodicTaskConfigs returns the periodic task configurations for the Asynq scheduler.
func PeriodicTaskConfigs(cfg *config.Config) []*asynq.PeriodicTaskConfig {
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

	}

	return configs
}
