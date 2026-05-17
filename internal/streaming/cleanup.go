package streaming

import (
	"context"
	"time"

	"github.com/usehiveloop/hiveloop/internal/logging"
)

const (
	idleTimeout = 30 * time.Minute
)

// Cleanup periodically removes idle conversation streams from Redis.
// A stream is idle if its last entry is older than idleTimeout.
type Cleanup struct {
	bus *EventBus
}

// NewCleanup creates a new Cleanup.
func NewCleanup(bus *EventBus) *Cleanup {
	return &Cleanup{bus: bus}
}

// CleanIdle removes conversation streams that have been idle for longer than idleTimeout.
func (c *Cleanup) CleanIdle(ctx context.Context) {
	convIDs, err := c.bus.ActiveConversations(ctx)
	if err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "cleanup: failed to get active conversations", "error", err)
		return
	}

	cutoff := time.Now().Add(-idleTimeout)

	for _, convID := range convIDs {
		if ctx.Err() != nil {
			return
		}

		streamKey := c.bus.Prefix() + convID

		msgs, err := c.bus.Redis().XRevRangeN(ctx, streamKey, "+", "-", 1).Result()
		if err != nil || len(msgs) == 0 {

			if delErr := c.bus.Delete(ctx, convID); delErr != nil {
				logging.FromContext(ctx).WarnContext(ctx, "cleanup delete failed", "error", delErr, "conv_id", convID)
			}
			continue
		}

		entryID := msgs[0].ID
		var tsMs int64
		for i := 0; i < len(entryID); i++ {
			if entryID[i] == '-' {
				break
			}
			tsMs = tsMs*10 + int64(entryID[i]-'0')
		}
		entryTime := time.UnixMilli(tsMs)

		if entryTime.Before(cutoff) {
			if err := c.bus.Delete(ctx, convID); err != nil {
				logging.FromContext(ctx).WarnContext(ctx, "cleanup delete failed", "error", err, "conv_id", convID)
			}
		}
	}
}
