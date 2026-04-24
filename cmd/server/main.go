package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/awnumar/memguard"

	"github.com/usehiveloop/hiveloop/internal/bootstrap"
	"github.com/usehiveloop/hiveloop/internal/enqueue"
	"github.com/usehiveloop/hiveloop/internal/goroutine"
	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	posthogobs "github.com/usehiveloop/hiveloop/internal/observability/posthog"
)

func init() {
	// Wire the middleware-based distinct ID extractor into the posthog
	// package. Done at init so background goroutines that start before
	// run() (e.g. memguard) still have a valid extractor installed.
	posthogobs.SetDistinctIDExtractor(middleware.DistinctID)
}

// @title HiveLoop API
// @version 1.0
// @description Proxy bridge for LLM API credentials.
// @host api.dev.hiveloop.com
// @BasePath /
// @schemes https
//
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Bearer token (JWT or API key).

// Set via -ldflags at build time.
var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	memguard.CatchInterrupt()
	disableCoreDumps()

	// Determine subcommand (default: "serve" for backward compatibility)
	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	if cmd == "version" {
		fmt.Printf("hiveloop %s (%s)\n", version, commit)
		return
	}

	if err := run(cmd); err != nil {
		// run() already logs the fatal via slog.Error BEFORE deps.Close
		// runs, so any wrapped PostHog client will have captured it. Here
		// we only need to flag the exit code — no log needed.
		os.Exit(1)
	}
}

func run(cmd string) error {
	// Bootstrap shared deps (config, DB, Redis, KMS, cache, etc.)
	// Logging must be initialized first.
	cfg, err := loadConfigForLogging()
	if err != nil {
		slog.Error("fatal", "error", err)
		return err
	}
	logging.Init(cfg.LogLevel, cfg.LogFormat)
	slog.Info("starting hiveloop", "version", version, "commit", commit, "mode", cmd)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	deps, err := bootstrap.New(ctx)
	if err != nil {
		// No PostHog client yet — log to stdout only.
		slog.Error("bootstrap failed", "error", err)
		return err
	}
	defer deps.Close()

	// Wrap the global slog handler so every Error-level log mirrors to
	// PostHog error tracking. This is a no-op when PostHog is disabled.
	// Must happen AFTER bootstrap.New so the PostHog client exists, but
	// BEFORE runServe/runWork so all subsequent errors are captured.
	if deps.PostHog != nil {
		wrapped := posthogobs.WrapSlogHandler(slog.Default().Handler(), deps.PostHog)
		slog.SetDefault(slog.New(wrapped))
	}

	posthogobs.CaptureEvent(deps.PostHog, ctx, "service_started", map[string]any{
		"mode":    cmd,
		"version": version,
		"commit":  commit,
	})

	runErr := dispatch(ctx, cmd, deps)
	if runErr != nil {
		// Log the fatal through the wrapped slog handler BEFORE deps.Close
		// flushes the PostHog client, so the exception is captured.
		slog.Error("service exited with error", "mode", cmd, "error", runErr)
	}

	posthogobs.CaptureEvent(deps.PostHog, ctx, "service_stopped", map[string]any{
		"mode":   cmd,
		"errored": runErr != nil,
	})

	return runErr
}

func dispatch(ctx context.Context, cmd string, deps *bootstrap.Deps) error {
	switch cmd {
	case "serve":
		enqueuer := enqueue.NewClient(deps.Config.AsynqRedisOpt())
		defer enqueuer.Close()
		return runServe(ctx, deps, enqueuer)

	case "work":
		return runWork(ctx, deps)

	case "both":
		// Run both server and worker in one process (for local dev).
		enqueuer := enqueue.NewClient(deps.Config.AsynqRedisOpt())
		defer enqueuer.Close()

		errCh := make(chan error, 2)
		goroutine.Go(ctx, func(ctx context.Context) {
			if err := runWork(ctx, deps); err != nil {
				errCh <- fmt.Errorf("worker: %w", err)
			}
		})
		goroutine.Go(ctx, func(ctx context.Context) {
			if err := runServe(ctx, deps, enqueuer); err != nil {
				errCh <- fmt.Errorf("serve: %w", err)
			}
		})

		select {
		case <-ctx.Done():
			return nil
		case err := <-errCh:
			return err
		}

	default:
		return fmt.Errorf("unknown command %q (use: serve, work, both, version)", cmd)
	}
}

type logConfig struct {
	LogLevel  string
	LogFormat string
}

// loadConfigForLogging reads log level/format from env vars so we can
// initialize structured logging before the full bootstrap runs.
func loadConfigForLogging() (*logConfig, error) {
	level := os.Getenv("LOG_LEVEL")
	if level == "" {
		level = "info"
	}
	format := os.Getenv("LOG_FORMAT")
	if format == "" {
		format = "text"
	}
	return &logConfig{LogLevel: level, LogFormat: format}, nil
}

func disableCoreDumps() {
	var rLimit syscall.Rlimit
	rLimit.Cur = 0
	rLimit.Max = 0
	_ = syscall.Setrlimit(syscall.RLIMIT_CORE, &rLimit)
}
