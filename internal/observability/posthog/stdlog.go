package posthog

import (
	stdlog "log"
	"log/slog"
)

// NewStdlogBridge returns a *log.Logger that forwards its writes to slog at
// Error level. Useful for injecting into http.Server.ErrorLog so stdlib HTTP
// server errors (connection accept failures, TLS handshake failures, etc.)
// are captured by the wrapped slog handler and mirrored to PostHog.
//
// The returned logger strips the stdlib prefix/flags so the slog record gets
// a clean message.
func NewStdlogBridge(component string) *stdlog.Logger {
	writer := slogWriter{
		logger:    slog.Default(),
		component: component,
	}
	return stdlog.New(writer, "", 0)
}

type slogWriter struct {
	logger    *slog.Logger
	component string
}

func (sw slogWriter) Write(payload []byte) (int, error) {
	// stdlib log.Logger appends a trailing newline — strip it.
	message := string(payload)
	if length := len(message); length > 0 && message[length-1] == '\n' {
		message = message[:length-1]
	}
	sw.logger.Error(sw.component+" stdlog", "message", message)
	return len(payload), nil
}
