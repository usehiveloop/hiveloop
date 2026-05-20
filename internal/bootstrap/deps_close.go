package bootstrap

import (
	"context"

	"github.com/usehivy/hivy/internal/logging"
	sentryobs "github.com/usehivy/hivy/internal/observability/sentry"
)

// Close releases all resources held by Deps. Sentry is flushed LAST so it
// can capture any errors produced by the other subsystems shutting down.
func (d *Deps) Close(ctx context.Context) {
	d.CacheManager.Memory().Purge()
	if sqlDB, err := d.DB.DB(); err == nil {
		_ = sqlDB.Close()
	}
	_ = d.Redis.Close()
	sentryobs.Close()
	logging.FromContext(ctx).InfoContext(ctx, "deps closed")
}
