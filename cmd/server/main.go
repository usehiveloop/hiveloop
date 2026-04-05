package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/awnumar/memguard"

	"github.com/ziraloop/ziraloop/internal/bootstrap"
	"github.com/ziraloop/ziraloop/internal/enqueue"
	"github.com/ziraloop/ziraloop/internal/goroutine"
	"github.com/ziraloop/ziraloop/internal/logging"
)

// @title ZiraLoop API
// @version 1.0
// @description Proxy bridge for LLM API credentials.
// @host api.dev.ziraloop.com
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
		fmt.Printf("ziraloop %s (%s)\n", version, commit)
		return
	}

	if err := run(cmd); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run(cmd string) error {
	// Bootstrap shared deps (config, DB, Redis, KMS, cache, etc.)
	// Logging must be initialized first.
	cfg, err := loadConfigForLogging()
	if err != nil {
		return err
	}
	logging.Init(cfg.LogLevel, cfg.LogFormat)
	slog.Info("starting ziraloop", "version", version, "commit", commit, "mode", cmd)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	deps, err := bootstrap.New(ctx)
	if err != nil {
		return err
	}
	defer deps.Close()

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
		goroutine.Go(func() {
			if err := runWork(ctx, deps); err != nil {
				errCh <- fmt.Errorf("worker: %w", err)
			}
		})
		goroutine.Go(func() {
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
