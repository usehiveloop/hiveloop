package logging

import (
	"log/slog"
	"os"
	"strings"
)

// Init configures the global slog logger.
//
// level: "debug", "info", "warn", "error"
// format: "json" (production) or "text" (development)
func Init(level, format string) {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn", "warning":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: lvl,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			// Redact known sensitive keys if they ever appear in log attributes.
			switch a.Key {
			case "authorization", "api_key", "token", "password", "secret":
				return slog.String(a.Key, "[REDACTED]")
			}
			return a
		},
	}

	var handler slog.Handler
	if strings.ToLower(format) == "text" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	slog.SetDefault(slog.New(handler))
}
