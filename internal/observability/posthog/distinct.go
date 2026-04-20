package posthog

import (
	"context"
	"sync/atomic"
)

// DistinctIDExtractor extracts a stable identifier from a request context for
// attributing captured errors to users/orgs/requests. Implementations should
// return an empty string when nothing is available, so the caller can fall
// back to "system" or skip capture.
type DistinctIDExtractor func(ctx context.Context) string

var distinctIDExtractor atomic.Pointer[DistinctIDExtractor]

// SetDistinctIDExtractor installs a package-level extractor used by DistinctID.
// This allows the observability/posthog package to avoid importing the
// middleware package directly (which would create an import cycle via
// internal/goroutine.Go). Call this once during main() startup.
func SetDistinctIDExtractor(fn DistinctIDExtractor) {
	if fn == nil {
		return
	}
	distinctIDExtractor.Store(&fn)
}

// DistinctID returns the distinct ID for the supplied context by delegating
// to the extractor installed via SetDistinctIDExtractor. Returns "" when no
// extractor is installed or the extractor finds nothing.
func DistinctID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	extractor := distinctIDExtractor.Load()
	if extractor == nil {
		return ""
	}
	return (*extractor)(ctx)
}
