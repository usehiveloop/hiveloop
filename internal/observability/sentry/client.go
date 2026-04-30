package sentry

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync/atomic"
	"time"

	sentrygo "github.com/getsentry/sentry-go"

	"github.com/usehiveloop/hiveloop/internal/config"
)

type ClientOptions struct {
	ServiceName string
	Environment string
	Hostname    string
	Release     string
}

var initialized atomic.Bool

func Init(cfg *config.Config, opts ClientOptions) error {
	if cfg == nil || !cfg.SentryEnabled || cfg.SentryDSN == "" {
		return nil
	}

	hostname := opts.Hostname
	if hostname == "" {
		if h, err := os.Hostname(); err == nil {
			hostname = h
		}
	}

	environment := opts.Environment
	if environment == "" {
		environment = cfg.Environment
	}

	release := opts.Release
	if release == "" {
		release = cfg.SentryRelease
	}

	if err := sentrygo.Init(sentrygo.ClientOptions{
		Dsn:              cfg.SentryDSN,
		Environment:      environment,
		Release:          release,
		EnableTracing:    true,
		TracesSampleRate: cfg.SentryTracesSampleRate,
		AttachStacktrace: true,
		ServerName:       hostname,
	}); err != nil {
		return fmt.Errorf("sentry: initialize client: %w", err)
	}

	sentrygo.ConfigureScope(func(scope *sentrygo.Scope) {
		if opts.ServiceName != "" {
			scope.SetTag("service", opts.ServiceName)
		}
		if hostname != "" {
			scope.SetTag("hostname", hostname)
		}
	})

	initialized.Store(true)
	slog.Info("sentry initialized",
		"environment", environment,
		"release", release,
		"traces_sample_rate", cfg.SentryTracesSampleRate,
		"service", opts.ServiceName,
	)
	return nil
}

func Enabled() bool { return initialized.Load() }

func Flush(timeout time.Duration) bool {
	if !Enabled() {
		return true
	}
	return sentrygo.Flush(timeout)
}

func Close() {
	if !Enabled() {
		return
	}
	if !sentrygo.Flush(5 * time.Second) {
		slog.Warn("sentry: flush timed out before shutdown")
	}
}

func CaptureException(ctx context.Context, err error) {
	if !Enabled() || err == nil {
		return
	}
	hubFromContext(ctx).CaptureException(err)
}

func CaptureMessage(ctx context.Context, message string) {
	if !Enabled() || message == "" {
		return
	}
	hubFromContext(ctx).CaptureMessage(message)
}

func hubFromContext(ctx context.Context) *sentrygo.Hub {
	if ctx != nil {
		if hub := sentrygo.GetHubFromContext(ctx); hub != nil {
			return hub
		}
	}
	return sentrygo.CurrentHub()
}
