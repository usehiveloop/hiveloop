package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/hibiken/asynq"

	"github.com/ziraloop/ziraloop/internal/bootstrap"
	"github.com/ziraloop/ziraloop/internal/email"
	"github.com/ziraloop/ziraloop/internal/forge"
	"github.com/ziraloop/ziraloop/internal/goroutine"
	"github.com/ziraloop/ziraloop/internal/mcp/catalog"
	"github.com/ziraloop/ziraloop/internal/registry"
	"github.com/ziraloop/ziraloop/internal/tasks"
	systemagents "github.com/ziraloop/ziraloop/internal/system-agents"
)

func runWork(ctx context.Context, deps *bootstrap.Deps) error {
	cfg := deps.Config

	// Seed system agents (idempotent, runs once at startup)
	goroutine.Go(func() {
		if err := systemagents.Seed(deps.DB); err != nil {
			slog.Error("failed to seed system agents", "error", err)
		} else {
			slog.Info("system agents seeded")
		}
	})

	// Start long-running stream consumers as goroutines
	// (sub-second ticks, not suitable for Asynq periodic tasks)
	goroutine.Go(func() { deps.Flusher.Run(ctx) })

	if deps.Retainer != nil {
		goroutine.Go(func() { deps.Retainer.Run(ctx) })
		slog.Info("hindsight memory retainer started")
	}

	// Stream cleanup, sandbox health/resource checks, and token cleanup
	// are now Asynq periodic tasks — no more goroutines here.

	// Asynq server
	redisOpt := cfg.AsynqRedisOpt()
	// Email sender for the worker (LogSender for now — replace with real SMTP later)
	logSender := &email.LogSender{}
	workerDeps := &tasks.WorkerDeps{
		DB:           deps.DB,
		Cleanup:      deps.Cleanup,
		Orchestrator: deps.Orchestrator,
		Pusher:       deps.AgentPusher,
		EncKey:       deps.SandboxEncKey,
		EmailSend: func(ctx context.Context, to, subject, body string) error {
			return logSender.Send(ctx, email.Message{To: to, Subject: subject, Body: body})
		},
		PolarClient: deps.PolarClient,
	}

	// Create forge controller for the worker (if sandbox is configured)
	if deps.Orchestrator != nil && deps.AgentPusher != nil {
		forgeCtrl := forge.NewForgeController(
			deps.DB, deps.Orchestrator, deps.AgentPusher,
			deps.SigningKey, deps.Config, deps.EventBus, catalog.Global(),
			registry.Global(), redisOpt,
		)
		workerDeps.ForgeExecute = forgeCtrl.Execute
		workerDeps.ForgeDesignEvals = forgeCtrl.DesignEvals
		slog.Info("forge controller ready (worker)")
	}
	mux := tasks.NewServeMux(workerDeps)

	srv := asynq.NewServer(redisOpt, asynq.Config{
		Concurrency: cfg.AsynqConcurrency,
		Queues: map[string]int{
			tasks.QueueCritical: 6,
			tasks.QueueDefault:  3,
			tasks.QueuePeriodic: 2,
			tasks.QueueBulk:     1,
		},
		Logger:          newAsynqLogger(),
		ShutdownTimeout: cfg.AsynqShutdownTimeout,
	})

	// Start Asynq server in background
	errCh := make(chan error, 1)
	goroutine.Go(func() {
		slog.Info("asynq worker starting", "concurrency", cfg.AsynqConcurrency)
		if err := srv.Run(mux); err != nil {
			slog.Error("asynq server error", "error", err)
			errCh <- err
		}
	})

	// Asynq periodic task scheduler
	periodicConfigs := tasks.PeriodicTaskConfigs(cfg)
	if len(periodicConfigs) > 0 {
		scheduler := asynq.NewScheduler(redisOpt, nil)
		for _, pc := range periodicConfigs {
			if _, err := scheduler.Register(pc.Cronspec, pc.Task, pc.Opts...); err != nil {
				return fmt.Errorf("registering periodic task %s: %w", pc.Task.Type(), err)
			}
			slog.Info("registered periodic task", "type", pc.Task.Type(), "cron", pc.Cronspec)
		}
		goroutine.Go(func() {
			if err := scheduler.Run(); err != nil {
				slog.Error("asynq scheduler error", "error", err)
			}
		})
	}

	// Worker health check server
	healthMux := http.NewServeMux()
	healthMux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","service":"worker"}`))
	})
	healthMux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		sqlDB, err := deps.DB.DB()
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"error","detail":"db connection failed"}`))
			return
		}
		if err := sqlDB.Ping(); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"error","detail":"db ping failed"}`))
			return
		}
		if err := deps.Redis.Ping(r.Context()).Err(); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"error","detail":"redis ping failed"}`))
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","service":"worker"}`))
	})

	healthSrv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.WorkerHealthPort),
		Handler: healthMux,
	}
	goroutine.Go(func() {
		slog.Info("worker health server starting", "port", cfg.WorkerHealthPort)
		if err := healthSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("worker health server error", "error", err)
		}
	})

	// Wait for shutdown
	select {
	case <-ctx.Done():
	case err := <-errCh:
		return err
	}

	slog.Info("worker shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.AsynqShutdownTimeout)
	defer cancel()

	srv.Shutdown()

	if err := healthSrv.Shutdown(shutdownCtx); err != nil {
		slog.Error("health server shutdown error", "error", err)
	}

	slog.Info("worker shutdown complete")
	return nil
}

// asynqLogger adapts slog to asynq's Logger interface.
type asynqLogger struct{}

func newAsynqLogger() *asynqLogger { return &asynqLogger{} }

func (l *asynqLogger) Debug(args ...any) {
	slog.Debug(fmt.Sprint(args...))
}

func (l *asynqLogger) Info(args ...any) {
	slog.Info(fmt.Sprint(args...))
}

func (l *asynqLogger) Warn(args ...any) {
	slog.Warn(fmt.Sprint(args...))
}

func (l *asynqLogger) Error(args ...any) {
	slog.Error(fmt.Sprint(args...))
}

func (l *asynqLogger) Fatal(args ...any) {
	slog.Error(fmt.Sprint(args...))
}
