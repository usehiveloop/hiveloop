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

	"github.com/usehiveloop/hiveloop/internal/model"
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

	batchHasTerminal := false

	for _, msg := range msgs {
		eventType, _ := msg.Values["event_type"].(string)
		dataStr, _ := msg.Values["data"].(string)

		// Streaming token deltas aren't persisted. We accumulate them in Redis
		// keyed by message_id so we can synthesize a row if response_completed
		// never arrives; on success, they're dropped below.
		if eventType == "response_chunk" {
			f.accumulateChunk(ctx, convID, dataStr)
			entryIDs = append(entryIDs, msg.ID)
			continue
		}

		// Parse the full event payload to extract individual fields.
		var full struct {
			EventID              string          `json:"event_id"`
			AgentID              string          `json:"agent_id"`
			ConversationID       string          `json:"conversation_id"`
			Timestamp            time.Time       `json:"timestamp"`
			SequenceNumber       int64           `json:"sequence_number"`
			Data                 json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal([]byte(dataStr), &full); err != nil {
			slog.Warn("flusher: failed to parse event payload", "conversation_id", convID, "error", err)
			entryIDs = append(entryIDs, msg.ID)
			continue
		}

		events = append(events, model.ConversationEvent{
			OrgID:                conv.OrgID,
			ConversationID:       conv.ID,
			EventID:              full.EventID,
			EventType:            eventType,
			AgentID:              full.AgentID,
			BridgeConversationID: full.ConversationID,
			Timestamp:            full.Timestamp,
			SequenceNumber:       full.SequenceNumber,
			Data:                 model.RawJSON(full.Data),
		})
		entryIDs = append(entryIDs, msg.ID)

		// A real completion supersedes any accumulator for this message.
		if eventType == "response_completed" {
			var d struct {
				MessageID string `json:"message_id"`
			}
			if err := json.Unmarshal(full.Data, &d); err == nil && d.MessageID != "" {
				f.bus.DropChunk(ctx, convID, d.MessageID)
			}
		}

		if eventType == "done" || eventType == "ConversationEnded" || eventType == "AgentError" {
			batchHasTerminal = true
		}
	}

	// Fallback: if this batch ends a turn/conversation and any accumulators
	// remain, synthesize response_completed rows from them.
	var recoveredMsgIDs []string
	if batchHasTerminal {
		if recovered, err := f.bus.PeekChunks(ctx, convID); err == nil {
			for messageID, content := range recovered {
				events = append(events, buildRecoveredEvent(&conv, messageID, content))
				recoveredMsgIDs = append(recoveredMsgIDs, messageID)
			}
		}
	}

	// Batch insert to Postgres
	if err := f.db.CreateInBatches(events, 50).Error; err != nil {
		slog.Error("flusher: batch insert failed", "conversation_id", convID, "count", len(events), "error", err)
		// Don't ACK — events will be retried. Recovered accumulators stay in
		// Redis so the retry can synthesize them again.
		return
	}

	// ACK all flushed entries
	if len(entryIDs) > 0 {
		f.bus.Redis().XAck(ctx, streamKey, flusherGroup, entryIDs...)
	}

	// Safe to drop accumulators now that their synthesized rows are persisted.
	for _, mid := range recoveredMsgIDs {
		f.bus.DropChunk(ctx, convID, mid)
	}

	// Trim stream to keep it bounded
	f.bus.Trim(ctx, convID, trimMaxLen)

	slog.Debug("flusher: flushed events", "conversation_id", convID, "count", len(events))
}

// accumulateChunk extracts the delta + message_id from a response_chunk
// envelope and appends to the Redis accumulator for that message.
func (f *Flusher) accumulateChunk(ctx context.Context, convID, dataStr string) {
	var envelope struct {
		Data struct {
			Delta     string `json:"delta"`
			MessageID string `json:"message_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(dataStr), &envelope); err != nil {
		return
	}
	if envelope.Data.MessageID == "" || envelope.Data.Delta == "" {
		return
	}
	if err := f.bus.AppendChunk(ctx, convID, envelope.Data.MessageID, envelope.Data.Delta); err != nil {
		slog.Warn("flusher: failed to accumulate chunk", "conversation_id", convID, "error", err)
	}
}

// buildRecoveredEvent constructs a synthesized response_completed row from
// accumulated chunks. Flagged with recovered:true so downstream consumers can
// tell it apart from a Bridge-sent completion.
func buildRecoveredEvent(conv *model.AgentConversation, messageID, content string) model.ConversationEvent {
	data, _ := json.Marshal(map[string]any{
		"message_id":    messageID,
		"full_response": content,
		"recovered":     true,
	})
	return model.ConversationEvent{
		OrgID:                conv.OrgID,
		ConversationID:       conv.ID,
		EventID:              "recovered-" + messageID,
		EventType:            "response_completed",
		BridgeConversationID: conv.BridgeConversationID,
		Timestamp:            time.Now(),
		Data:                 model.RawJSON(data),
	}
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
