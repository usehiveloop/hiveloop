package streaming

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/llmvault/llmvault/internal/model"
)

const (
	flusherGroup    = "db-flusher"
	flushBatchSize  = 100
	flushBlockTime  = 2 * time.Second
	trimMaxLen      = 500
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

	// Process pending (unacknowledged) entries from a previous crash first
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

// flushAll reads from all active conversation streams and flushes to Postgres.
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

// flushStream reads new events from a single conversation stream and writes to Postgres.
func (f *Flusher) flushStream(ctx context.Context, convID string) {
	streamKey := f.bus.Prefix() + convID

	// Ensure consumer group exists
	f.bus.Redis().XGroupCreateMkStream(ctx, streamKey, flusherGroup, "0").Err()

	// Read new messages
	streams, err := f.bus.Redis().XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    flusherGroup,
		Consumer: f.consumer,
		Streams:  []string{streamKey, ">"},
		Count:    flushBatchSize,
		Block:    100 * time.Millisecond,
	}).Result()
	if err != nil && err != redis.Nil {
		if ctx.Err() == nil {
			slog.Error("flusher: XREADGROUP error", "conversation_id", convID, "error", err)
		}
		return
	}

	if len(streams) == 0 || len(streams[0].Messages) == 0 {
		return
	}

	msgs := streams[0].Messages
	events := make([]model.ConversationEvent, 0, len(msgs))
	entryIDs := make([]string, 0, len(msgs))

	// Find the conversation record to get the org_id
	convUUID, err := uuid.Parse(convID)
	if err != nil {
		slog.Error("flusher: invalid conversation ID", "conversation_id", convID, "error", err)
		return
	}

	var conv model.AgentConversation
	if err := f.db.Where("id = ?", convUUID).First(&conv).Error; err != nil {
		slog.Debug("flusher: conversation not found, skipping", "conversation_id", convID)
		// ACK anyway to avoid reprocessing
		for _, msg := range msgs {
			f.bus.Redis().XAck(ctx, streamKey, flusherGroup, msg.ID)
		}
		return
	}

	for _, msg := range msgs {
		var payload model.JSON
		if dataStr, ok := msg.Values["data"].(string); ok {
			if err := json.Unmarshal([]byte(dataStr), &payload); err != nil {
				payload = model.JSON{"raw": dataStr}
			}
		}

		eventType, _ := msg.Values["event_type"].(string)

		events = append(events, model.ConversationEvent{
			OrgID:          conv.OrgID,
			ConversationID: conv.ID,
			EventType:      eventType,
			Payload:        payload,
		})
		entryIDs = append(entryIDs, msg.ID)
	}

	// Batch insert to Postgres
	if err := f.db.CreateInBatches(events, 50).Error; err != nil {
		slog.Error("flusher: batch insert failed", "conversation_id", convID, "count", len(events), "error", err)
		// Don't ACK — events will be retried
		return
	}

	// ACK all flushed entries
	if len(entryIDs) > 0 {
		f.bus.Redis().XAck(ctx, streamKey, flusherGroup, entryIDs...)
	}

	// Trim stream to keep it bounded
	f.bus.Trim(ctx, convID, trimMaxLen)

	slog.Debug("flusher: flushed events", "conversation_id", convID, "count", len(events))
}

// processPending re-processes entries that were read but not acknowledged (crash recovery).
func (f *Flusher) processPending(ctx context.Context) {
	convIDs, err := f.bus.ActiveConversations(ctx)
	if err != nil {
		return
	}

	for _, convID := range convIDs {
		if ctx.Err() != nil {
			return
		}
		streamKey := f.bus.Prefix() + convID

		// Ensure group exists
		f.bus.Redis().XGroupCreateMkStream(ctx, streamKey, flusherGroup, "0").Err()

		// Read pending (unacknowledged) entries: use "0" instead of ">"
		streams, err := f.bus.Redis().XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    flusherGroup,
			Consumer: f.consumer,
			Streams:  []string{streamKey, "0"},
			Count:    flushBatchSize,
		}).Result()
		if err != nil || len(streams) == 0 || len(streams[0].Messages) == 0 {
			continue
		}

		// These are pending entries — flush them
		f.flushStream(ctx, convID)
	}
}
