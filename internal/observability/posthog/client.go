// Package posthog wires the posthog-go SDK into the hiveloop platform for
// error tracking across the HTTP API, the LLM proxy, Asynq workers, and
// long-running background goroutines. See internal/config.Config for the
// environment knobs (POSTHOG_API_KEY, POSTHOG_ENDPOINT, POSTHOG_ENABLED).
package posthog

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync/atomic"
	"time"

	ph "github.com/posthog/posthog-go"

	"github.com/usehiveloop/hiveloop/internal/config"
)

// ClientOptions controls the behavior of the PostHog client created by
// NewClient. The zero value is valid and produces a client with sensible
// defaults matching the posthog-go documentation for Go error tracking.
type ClientOptions struct {
	// ServiceName is attached as a default property ($service) on every event.
	// Typical values: "api", "worker", "proxy", "mcp".
	ServiceName string

	// Environment is attached as a default property ($environment) on every
	// event. Typical values: "development", "production".
	Environment string

	// Hostname is attached as a default property ($hostname) on every event.
	// If empty, os.Hostname is used.
	Hostname string
}

// NewClient returns a PostHog client based on the supplied config. The client
// is a no-op (nil ph.Client) when PostHog is disabled or the API key is empty,
// so callers can always defer Close and Enqueue without nil checks via the
// helpers exposed in this package.
func NewClient(cfg *config.Config, opts ClientOptions) (ph.Client, error) {
	if cfg == nil || !cfg.PostHogEnabled || cfg.PostHogAPIKey == "" {
		return nil, nil
	}

	hostname := opts.Hostname
	if hostname == "" {
		if h, err := os.Hostname(); err == nil {
			hostname = h
		}
	}

	defaults := ph.NewProperties()
	if opts.ServiceName != "" {
		defaults.Set("$service", opts.ServiceName)
	}
	if opts.Environment != "" {
		defaults.Set("$environment", opts.Environment)
	}
	if hostname != "" {
		defaults.Set("$hostname", hostname)
	}

	client, err := ph.NewWithConfig(cfg.PostHogAPIKey, ph.Config{
		Endpoint:               cfg.PostHogEndpoint,
		DefaultEventProperties: defaults,
		// Posthog's default of 5s is fine for server events; shutdown timeout
		// guarantees Close() doesn't hang the process if the network is down.
		ShutdownTimeout: 10 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("posthog: initialize client: %w", err)
	}

	setDefault(client)
	return client, nil
}

// defaultClient is the package-level client used by helpers that cannot take a
// client parameter (most notably goroutine.Go recover, which is called from
// packages that cannot depend on the observability wiring). It is set once by
// NewClient. Never replace the value once set non-nil.
var defaultClient atomic.Pointer[ph.Client]

func setDefault(client ph.Client) {
	if client == nil {
		return
	}
	defaultClient.Store(&client)
}

// Default returns the package-level client set by NewClient. May be nil if
// PostHog is disabled or NewClient has not yet been called. Callers MUST
// nil-check before using the returned value.
func Default() ph.Client {
	if p := defaultClient.Load(); p != nil {
		return *p
	}
	return nil
}

// Close flushes any pending events and shuts down the supplied client. Safe
// to call with a nil client.
func Close(client ph.Client) {
	if client == nil {
		return
	}
	if err := client.Close(); err != nil {
		slog.Warn("posthog: close failed", "error", err)
	}
}

// CaptureException enqueues an exception event with the supplied title and
// description. Safe to call with a nil client or empty distinct ID (both
// become no-ops). Properties map is attached verbatim to the exception item.
func CaptureException(client ph.Client, ctx context.Context, title, description string) {
	if client == nil {
		return
	}
	distinctID := DistinctID(ctx)
	if distinctID == "" {
		distinctID = "system"
	}
	exception := ph.NewDefaultException(time.Now(), distinctID, title, description)
	if err := client.Enqueue(exception); err != nil {
		slog.Warn("posthog: enqueue exception failed", "error", err)
	}
}

// CaptureError is a convenience wrapper that calls CaptureException with the
// error's message as the description. Safe to call with a nil client or nil
// error (both become no-ops).
func CaptureError(client ph.Client, ctx context.Context, title string, err error) {
	if err == nil {
		return
	}
	CaptureException(client, ctx, title, err.Error())
}

// CaptureEvent enqueues a non-exception event. Useful for lifecycle events
// like "startup" and "shutdown". Safe to call with a nil client.
func CaptureEvent(client ph.Client, ctx context.Context, event string, properties map[string]any) {
	if client == nil {
		return
	}
	distinctID := DistinctID(ctx)
	if distinctID == "" {
		distinctID = "system"
	}
	props := ph.NewProperties()
	for key, value := range properties {
		props.Set(key, value)
	}
	capture := ph.Capture{
		DistinctId: distinctID,
		Event:      event,
		Properties: props,
	}
	if err := client.Enqueue(capture); err != nil {
		slog.Warn("posthog: enqueue event failed", "event", event, "error", err)
	}
}
