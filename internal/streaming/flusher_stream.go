package streaming

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/usehiveloop/hiveloop/internal/model"
)

func (f *Flusher) flushStream(ctx context.Context, convID string) {
	streamKey := f.bus.Prefix() + convID

	f.bus.Redis().XGroupCreateMkStream(ctx, streamKey, flusherGroup, "0").Err()

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

	convUUID, err := uuid.Parse(convID)
	if err != nil {
		slog.Error("flusher: invalid conversation ID", "conversation_id", convID, "error", err)
		return
	}

	var conv model.AgentConversation
	if err := f.db.Where("id = ?", convUUID).First(&conv).Error; err != nil {
		slog.Debug("flusher: conversation not found, skipping", "conversation_id", convID)
		for _, msg := range msgs {
			f.bus.Redis().XAck(ctx, streamKey, flusherGroup, msg.ID)
		}
		return
	}

	batchHasTerminal := false

	for _, msg := range msgs {
		eventType, _ := msg.Values["event_type"].(string)
		dataStr, _ := msg.Values["data"].(string)

		if eventType == "response_chunk" {
			f.accumulateChunk(ctx, convID, dataStr)
			entryIDs = append(entryIDs, msg.ID)
			continue
		}

		var full struct {
			EventID        string          `json:"event_id"`
			AgentID        string          `json:"agent_id"`
			ConversationID string          `json:"conversation_id"`
			Timestamp      time.Time       `json:"timestamp"`
			SequenceNumber int64           `json:"sequence_number"`
			Data           json.RawMessage `json:"data"`
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

	var recoveredMsgIDs []string
	if batchHasTerminal {
		if recovered, err := f.bus.PeekChunks(ctx, convID); err == nil {
			for messageID, content := range recovered {
				events = append(events, buildRecoveredEvent(&conv, messageID, content))
				recoveredMsgIDs = append(recoveredMsgIDs, messageID)
			}
		}
	}

	if err := f.db.CreateInBatches(events, 50).Error; err != nil {
		slog.Error("flusher: batch insert failed", "conversation_id", convID, "count", len(events), "error", err)
		return
	}

	if len(entryIDs) > 0 {
		f.bus.Redis().XAck(ctx, streamKey, flusherGroup, entryIDs...)
	}

	for _, mid := range recoveredMsgIDs {
		f.bus.DropChunk(ctx, convID, mid)
	}

	f.bus.Trim(ctx, convID, trimMaxLen)

	slog.Debug("flusher: flushed events", "conversation_id", convID, "count", len(events))
}
