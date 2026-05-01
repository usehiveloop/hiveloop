package logging

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

// sensitiveKeys are attribute keys whose values are always redacted from log output.
// Extend this set whenever a new credential-bearing key is introduced.
var sensitiveKeys = map[string]struct{}{
	"authorization":  {},
	"api_key":        {},
	"apikey":         {},
	"token":          {},
	"access_token":   {},
	"refresh_token":  {},
	"id_token":       {},
	"bearer":         {},
	"password":       {},
	"passphrase":     {},
	"secret":         {},
	"client_secret":  {},
	"webhook_secret": {},
	"signing_secret": {},
	"private_key":    {},
	"dsn":            {},
	"cookie":         {},
	"set-cookie":     {},
	"ssn":            {},
	"card":           {},
	"credit_card":    {},
	"cvv":            {},
	"pan":            {},
}

// Init configures the global slog logger.
//
// level: "debug", "info", "warn", "error" (default: "info")
// format: "json" (production) or "text" (development)
//
// Per-package overrides are read from environment variables of the form
// LOG_LEVEL_<PKG>=debug where <PKG> is the upper-cased final segment of the
// package import path (e.g. LOG_LEVEL_NANGO=debug for internal/nango).
// See packageLevelFromEnv for resolution details.
func Init(level, format string) {
	base := parseLevel(level)

	opts := &slog.HandlerOptions{
		Level:       base, // base level; per-package overrides are layered below
		AddSource:   false,
		ReplaceAttr: redactSensitive,
	}

	var inner slog.Handler
	if strings.ToLower(format) == "text" {
		inner = slog.NewTextHandler(os.Stdout, opts)
	} else {
		inner = slog.NewJSONHandler(os.Stdout, opts)
	}

	slog.SetDefault(slog.New(&perPackageHandler{
		inner:    inner,
		baseLvl:  base,
		pkgLevel: loadPackageOverrides(),
	}))
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func redactSensitive(_ []string, a slog.Attr) slog.Attr {
	if _, ok := sensitiveKeys[strings.ToLower(a.Key)]; ok {
		return slog.String(a.Key, "[REDACTED]")
	}
	return a
}

// loadPackageOverrides scans the process environment for LOG_LEVEL_<PKG>
// variables and returns a map of upper-cased package name -> slog.Level.
func loadPackageOverrides() map[string]slog.Level {
	out := map[string]slog.Level{}
	for _, e := range os.Environ() {
		eq := strings.IndexByte(e, '=')
		if eq < 0 {
			continue
		}
		k, v := e[:eq], e[eq+1:]
		const prefix = "LOG_LEVEL_"
		if !strings.HasPrefix(k, prefix) || k == prefix {
			continue
		}
		out[strings.TrimPrefix(k, prefix)] = parseLevel(v)
	}
	return out
}

// perPackageHandler wraps a base slog.Handler and applies per-package level
// overrides via the "pkg" attribute (set by FromContext). Records without a
// pkg attribute fall through to the base level.
type perPackageHandler struct {
	inner    slog.Handler
	baseLvl  slog.Level
	pkgLevel map[string]slog.Level
}

func (h *perPackageHandler) Enabled(ctx context.Context, level slog.Level) bool {
	// Cheap path: if the base level allows it, no need to check overrides.
	if level >= h.baseLvl {
		return h.inner.Enabled(ctx, level)
	}
	// At this point the record is below the base level. The actual decision
	// happens inside Handle once we can read the pkg attribute. Return true
	// so the record is forwarded to Handle.
	return len(h.pkgLevel) > 0
}

func (h *perPackageHandler) Handle(ctx context.Context, r slog.Record) error {
	if r.Level >= h.baseLvl {
		return h.inner.Handle(ctx, r)
	}
	// Below base level: only emit if a per-package override allows it.
	if len(h.pkgLevel) == 0 {
		return nil
	}
	var pkg string
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == "pkg" {
			pkg = strings.ToUpper(a.Value.String())
			return false
		}
		return true
	})
	if pkg == "" {
		return nil
	}
	if lvl, ok := h.pkgLevel[pkg]; ok && r.Level >= lvl {
		return h.inner.Handle(ctx, r)
	}
	return nil
}

func (h *perPackageHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &perPackageHandler{
		inner:    h.inner.WithAttrs(attrs),
		baseLvl:  h.baseLvl,
		pkgLevel: h.pkgLevel,
	}
}

func (h *perPackageHandler) WithGroup(name string) slog.Handler {
	return &perPackageHandler{
		inner:    h.inner.WithGroup(name),
		baseLvl:  h.baseLvl,
		pkgLevel: h.pkgLevel,
	}
}
