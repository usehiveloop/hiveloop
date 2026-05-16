package streaming

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/usehiveloop/hiveloop/internal/bridgeevents"
	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/model"
)

func (f *Flusher) flushStream(ctx context.Context, convID string) {
	streamKey := f.bus.Prefix() + convID

	_ = f.bus.Redis().XGroupCreateMkStream(ctx, streamKey, flusherGroup, "0").Err()

	streams, err := f.bus.Redis().XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    flusherGroup,
		Consumer: f.consumer,
		Streams:  []string{streamKey, ">"},
		Count:    flushBatchSize,
		Block:    100 * time.Millisecond,
	}).Result()
	if err != nil && err != redis.Nil {
		if ctx.Err() == nil {
			logging.FromContext(ctx).ErrorContext(ctx, "flusher: XREADGROUP error", "conversation_id", convID, "error", err)
		}
		return
	}

	if len(streams) == 0 || len(streams[0].Messages) == 0 {
		return
	}

	msgs := streams[0].Messages
	events := make([]model.ConversationEvent, 0, len(msgs))
	entryIDs := make([]string, 0, len(msgs))
	recoveredMsgIDs := make([]string, 0)
	recoveredSeen := make(map[string]struct{})

	convUUID, err := uuid.Parse(convID)
	if err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "flusher: invalid conversation ID", "conversation_id", convID, "error", err)
		return
	}

	var conv model.AgentConversation
	if err := f.db.Where("id = ?", convUUID).First(&conv).Error; err != nil {
		for _, msg := range msgs {
			f.bus.Redis().XAck(ctx, streamKey, flusherGroup, msg.ID)
		}
		return
	}

	for _, msg := range msgs {
		eventType, _ := msg.Values["event_type"].(string)
		eventType = bridgeevents.NormalizeEventType(eventType)
		dataStr, _ := msg.Values["data"].(string)

		if eventType == bridgeevents.EventResponseChunk {
			f.accumulateChunk(ctx, convID, dataStr)
			entryIDs = append(entryIDs, msg.ID)
			continue
		}
		if eventType == bridgeevents.EventReasoningDelta {
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
			logging.FromContext(ctx).WarnContext(ctx, "flusher: failed to parse event payload", "conversation_id", convID, "error", err)
			entryIDs = append(entryIDs, msg.ID)
			continue
		}

		dbEvent := model.ConversationEvent{
			OrgID:                conv.OrgID,
			ConversationID:       conv.ID,
			EventID:              full.EventID,
			EventType:            eventType,
			AgentID:              full.AgentID,
			RuntimeConversationID: full.ConversationID,
			Timestamp:            full.Timestamp,
			SequenceNumber:       full.SequenceNumber,
			Data:                 model.RawJSON(full.Data),
		}
		entryIDs = append(entryIDs, msg.ID)

		if eventType == bridgeevents.EventResponseCompleted {
			var d struct {
				MessageID string `json:"message_id"`
			}
			if err := json.Unmarshal(full.Data, &d); err == nil && d.MessageID != "" {
				if err := f.bus.DropChunk(ctx, convID, d.MessageID); err != nil {
					logging.FromContext(ctx).WarnContext(ctx, "flusher: drop chunk failed", "conversation_id", convID, "message_id", d.MessageID, "error", err)
				}
			}
		}

		if bridgeevents.IsTerminalEventType(eventType) {
			f.appendRecoveredEvents(ctx, convID, &conv, dbEvent, &events, &recoveredMsgIDs, recoveredSeen)
		}
		events = append(events, dbEvent)
	}

	if err := f.db.CreateInBatches(events, 50).Error; err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "flusher: batch insert failed", "conversation_id", convID, "count", len(events), "error", err)
		return
	}

	if len(entryIDs) > 0 {
		f.bus.Redis().XAck(ctx, streamKey, flusherGroup, entryIDs...)
	}

	for _, mid := range recoveredMsgIDs {
		if err := f.bus.DropChunk(ctx, convID, mid); err != nil {
			logging.FromContext(ctx).WarnContext(ctx, "flusher: drop recovered chunk failed", "conversation_id", convID, "message_id", mid, "error", err)
		}
	}

	if err := f.bus.Trim(ctx, convID, trimMaxLen); err != nil {
		logging.FromContext(ctx).WarnContext(ctx, "flusher: trim failed", "conversation_id", convID, "error", err)
	}
}

func (f *Flusher) appendRecoveredEvents(
	ctx context.Context,
	convID string,
	conv *model.AgentConversation,
	terminal model.ConversationEvent,
	events *[]model.ConversationEvent,
	recoveredMsgIDs *[]string,
	recoveredSeen map[string]struct{},
) {
	recovered, err := f.bus.PeekChunks(ctx, convID)
	if err == nil && len(recovered) > 0 {
		messageIDs := make([]string, 0, len(recovered))
		for messageID := range recovered {
			messageIDs = append(messageIDs, messageID)
		}
		sort.Strings(messageIDs)
		for _, messageID := range messageIDs {
			if _, ok := recoveredSeen[messageID]; ok {
				continue
			}
			recoveredSeen[messageID] = struct{}{}
			*events = append(*events, buildRecoveredEvent(conv, terminal, messageID, recovered[messageID]))
			*recoveredMsgIDs = append(*recoveredMsgIDs, messageID)
		}
	}
	if err := f.bus.ClearActiveChunkMessage(ctx, convID); err != nil {
		logging.FromContext(ctx).WarnContext(ctx, "flusher: clear active chunk message failed", "conversation_id", convID, "error", err)
	}
}
