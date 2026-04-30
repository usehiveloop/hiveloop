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
	sentryobs "github.com/usehiveloop/hiveloop/internal/observability/sentry"
)

func init() {
	sentryobs.SetUserExtractor(middleware.UserID)
	sentryobs.SetOrgExtractor(middleware.OrgID)
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
		slog.Error("bootstrap failed", "error", err)
		return err
	}
	defer deps.Close()

	// Wrap slog AFTER bootstrap (Sentry is initialized) and BEFORE dispatch
	// so every subsequent Error log is mirrored to Sentry.
	slog.SetDefault(slog.New(sentryobs.WrapSlogHandler(slog.Default().Handler())))

	sentryobs.CaptureMessage(ctx, fmt.Sprintf("service_started mode=%s version=%s", cmd, version))

	runErr := dispatch(ctx, cmd, deps)
	if runErr != nil {
		slog.Error("service exited with error", "mode", cmd, "error", runErr)
	}

	sentryobs.CaptureMessage(ctx, fmt.Sprintf("service_stopped mode=%s errored=%t", cmd, runErr != nil))

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
