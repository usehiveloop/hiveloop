package streaming

import (
	"context"
	"time"

	"github.com/usehiveloop/hiveloop/internal/logging"
)

const (
	cleanupInterval = 5 * time.Minute
	idleTimeout     = 30 * time.Minute
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

// Run starts the cleanup loop. It blocks until ctx is cancelled.
func (c *Cleanup) Run(ctx context.Context) {
	logging.FromContext(ctx).InfoContext(ctx, "stream cleanup started")
	defer logging.FromContext(ctx).InfoContext(ctx, "stream cleanup stopped")

	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.CleanIdle(ctx)
		}
	}
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

		// Check the last entry in the stream
		msgs, err := c.bus.Redis().XRevRangeN(ctx, streamKey, "+", "-", 1).Result()
		if err != nil || len(msgs) == 0 {
			// Stream is empty or gone — remove from active set
			if delErr := c.bus.Delete(ctx, convID); delErr != nil {
				logging.FromContext(ctx).WarnContext(ctx, "cleanup delete failed", "error", delErr, "conv_id", convID)
			}
			continue
		}

		// Parse timestamp from the Redis entry ID (format: "1712019600000-0")
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
