package streaming

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

)

const (
	flusherGroup         = "db-flusher"
	flushBatchSize       = 100
	flushBlockTime       = 2 * time.Second
	trimMaxLen           = 500
	pendingCheckInterval = 30 * time.Second
)

// Flusher reads events from Redis Streams and batch-writes them to Postgres.
// Uses Redis consumer groups to ensure each event is flushed exactly once,
// even with multiple API instances running.
type Flusher struct {
	bus      *EventBus
	db       *gorm.DB
	consumer string // unique per instance
}

// NewFlusher creates a new Flusher. consumer should be unique per API instance
// (e.g., hostname or pod name).
func NewFlusher(bus *EventBus, db *gorm.DB) *Flusher {
	consumer, _ := os.Hostname()
	if consumer == "" {
		consumer = uuid.New().String()[:8]
	}
	return &Flusher{
		bus:      bus,
		db:       db,
		consumer: consumer,
	}
}

// Run starts the flusher loop. It blocks until ctx is cancelled.
func (f *Flusher) Run(ctx context.Context) {
	slog.Info("stream flusher started", "consumer", f.consumer)
	defer slog.Info("stream flusher stopped", "consumer", f.consumer)

	f.processPending(ctx)

	ticker := time.NewTicker(flushBlockTime)
	defer ticker.Stop()

	pendingTicker := time.NewTicker(pendingCheckInterval)
	defer pendingTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			f.flushAll(ctx)
		case <-pendingTicker.C:
			f.processPending(ctx)
		}
	}
}

func (f *Flusher) flushAll(ctx context.Context) {
	convIDs, err := f.bus.ActiveConversations(ctx)
	if err != nil {
		slog.Error("flusher: failed to get active conversations", "error", err)
		return
	}

	for _, convID := range convIDs {
		if ctx.Err() != nil {
			return
		}
		f.flushStream(ctx, convID)
	}
}
