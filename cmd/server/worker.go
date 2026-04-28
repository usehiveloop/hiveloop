package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/hibiken/asynq"
	"github.com/hibiken/asynqmon"

	"github.com/usehiveloop/hiveloop/internal/bootstrap"
	"github.com/usehiveloop/hiveloop/internal/email"
	"github.com/usehiveloop/hiveloop/internal/enqueue"
	"github.com/usehiveloop/hiveloop/internal/goroutine"
	posthogobs "github.com/usehiveloop/hiveloop/internal/observability/posthog"
	// Blank import populates interfaces.Registry via init().
	_ "github.com/usehiveloop/hiveloop/internal/rag/connectors"
	ragscheduler "github.com/usehiveloop/hiveloop/internal/rag/scheduler"
	ragtasks "github.com/usehiveloop/hiveloop/internal/rag/tasks"
	"github.com/usehiveloop/hiveloop/internal/skills"
	subagents "github.com/usehiveloop/hiveloop/internal/sub-agents"
	"github.com/usehiveloop/hiveloop/internal/tasks"
)

func runWork(ctx context.Context, deps *bootstrap.Deps) error {
	cfg := deps.Config

	// Seed subagents on startup — idempotent, runs on every worker boot.
	goroutine.Go(ctx, func(context.Context) {
		if err := subagents.Seed(deps.DB); err != nil {
			slog.Error("failed to seed subagents", "error", err)
			return
		}
		slog.Info("subagents seeded")
	})

	// Start long-running stream consumers as goroutines
	// (sub-second ticks, not suitable for Asynq periodic tasks)
	goroutine.Go(ctx, func(ctx context.Context) { deps.Flusher.Run(ctx) })

	if deps.Retainer != nil {
		goroutine.Go(ctx, func(ctx context.Context) { deps.Retainer.Run(ctx) })
		slog.Info("hindsight memory retainer started")
	}

	// Stream cleanup, sandbox health/resource checks, and token cleanup
	// are now Asynq periodic tasks — no more goroutines here.

	// Asynq server
	redisOpt := cfg.AsynqRedisOpt()

	// Email sender: Kibamail when configured, LogSender otherwise. The
	// worker is the only process that actually hits Kibamail — handlers
	// enqueue, the worker drains with built-in retries.
	var workerSender email.Sender = &email.LogSender{}
	if cfg.KibamailAPIKey != "" {
		workerSender = email.NewKibamailSender(cfg.KibamailAPIKey, cfg.KibamailFromAddress, cfg.KibamailFromName)
		slog.Info("kibamail sender configured", "from", cfg.KibamailFromAddress, "from_name", cfg.KibamailFromName)
	} else {
		slog.Warn("KIBAMAIL_API_KEY not set — emails will be logged only")
	}

	enqueuer := enqueue.NewClient(redisOpt)
	ragSched := &ragscheduler.Deps{
		DB:  deps.DB,
		Enq: enqueuer,
		Cfg: ragscheduler.NewConfig(),
	}

	ragDeps := buildRagDeps(ctx, cfg, deps.DB, deps.NangoClient)

	workerDeps := &tasks.WorkerDeps{
		DB:           deps.DB,
		Cleanup:      deps.Cleanup,
		Orchestrator: deps.Orchestrator,
		Pusher:       deps.AgentPusher,
		EncKey:       deps.SandboxEncKey,
		EmailSend: func(ctx context.Context, to, subject, body string) error {
			return workerSender.Send(ctx, email.Message{To: to, Subject: subject, Body: body})
		},
		EmailSendTemplate: func(ctx context.Context, to, slug string, variables map[string]string) error {
			return workerSender.SendTemplate(ctx, email.TemplateMessage{
				To:        to,
				Slug:      email.TemplateSlug(slug),
				Variables: variables,
			})
		},
		SkillFetcher: skills.NewGitFetcher(cfg.GitHubToken),
		NangoClient:  deps.NangoClient,
		CacheManager: deps.CacheManager,
		Credits:       deps.Credits,
		Subscriptions: deps.Subscriptions,
		Enqueuer:      enqueuer,
		Rag:          ragDeps,
		RagScheduler: ragSched,
	}

	mux := tasks.NewServeMux(workerDeps)

	srv := asynq.NewServer(redisOpt, asynq.Config{
		Concurrency: cfg.AsynqConcurrency,
		Queues: map[string]int{
			tasks.QueueCritical:    6,
			tasks.QueueDefault:     3,
			tasks.QueuePeriodic:    2,
			ragtasks.QueueRagWork:  2,
			tasks.QueueBulk:        1,
		},
		Logger:          newAsynqLogger(),
		ShutdownTimeout: cfg.AsynqShutdownTimeout,
		// ErrorHandler fires on EVERY task failure (before retry is scheduled).
		// This is how we capture task-handler panics and errors to PostHog —
		// asynq recovers panics internally and surfaces them as errors here.
		ErrorHandler: posthogobs.AsynqErrorHandler(deps.PostHog),
	})

	// Start Asynq server in background
	errCh := make(chan error, 1)
	goroutine.Go(ctx, func(context.Context) {
		slog.Info("asynq worker starting", "concurrency", cfg.AsynqConcurrency)
		if err := srv.Run(mux); err != nil {
			slog.Error("asynq server error", "error", err)
			errCh <- err
		}
	})

	// Asynq periodic task scheduler
	periodicConfigs := tasks.PeriodicTaskConfigs(cfg, ragSched)
	if len(periodicConfigs) > 0 {
		scheduler := asynq.NewScheduler(redisOpt, nil)
		for _, pc := range periodicConfigs {
			if _, err := scheduler.Register(pc.Cronspec, pc.Task, pc.Opts...); err != nil {
				return fmt.Errorf("registering periodic task %s: %w", pc.Task.Type(), err)
			}
			slog.Info("registered periodic task", "type", pc.Task.Type(), "cron", pc.Cronspec)
		}
		goroutine.Go(ctx, func(context.Context) {
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

	// Asynq dashboard
	dashboard := asynqmon.New(asynqmon.Options{
		RootPath:     "/asynq",
		RedisConnOpt: redisOpt,
		ReadOnly:     true,
	})
	healthMux.Handle("/asynq/", dashboard)
	slog.Info("asynq dashboard enabled at /asynq")

	healthSrv := &http.Server{
		Addr:     fmt.Sprintf(":%d", cfg.WorkerHealthPort),
		Handler:  healthMux,
		ErrorLog: posthogobs.NewStdlogBridge("worker_health_server"),
	}
	goroutine.Go(ctx, func(context.Context) {
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

	// Shutdown intentionally decouples from the (already-cancelled) parent ctx
	// but inherits its values so observability tags propagate. context.WithoutCancel
	// strips cancellation while preserving values; the WithTimeout below bounds
	// how long shutdown can take.
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.AsynqShutdownTimeout)
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


