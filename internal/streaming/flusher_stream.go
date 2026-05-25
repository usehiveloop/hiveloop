package streaming

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/usehivy/hivy/internal/bridgeevents"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
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
			captureTimelineFlush(ctx, "read_stream", convID, err, nil)
		}
		return
	}

	if len(streams) == 0 || len(streams[0].Messages) == 0 {
		return
	}

	msgs := streams[0].Messages
	events := make([]model.EmployeeSessionEvent, 0, len(msgs))
	entryIDs := make([]string, 0, len(msgs))
	recoveredMsgIDs := make([]string, 0)
	recoveredReasoningIDs := make([]string, 0)
	recoveredSeen := make(map[string]struct{})
	recoveredReasoningSeen := make(map[string]struct{})

	convUUID, err := uuid.Parse(convID)
	if err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "flusher: invalid conversation ID", "conversation_id", convID, "error", err)
		captureTimelineFlush(ctx, "invalid_conversation_id", convID, err, map[string]any{"stream_key": streamKey})
		return
	}

	var conv model.EmployeeConversation
	if err := f.db.Where("id = ?", convUUID).First(&conv).Error; err != nil {
		captureTimelineFlush(ctx, "conversation_not_found", convID, err, map[string]any{
			"message_count": len(msgs),
			"stream_key":    streamKey,
		})
		for _, msg := range msgs {
			f.bus.Redis().XAck(ctx, streamKey, flusherGroup, msg.ID)
		}
		return
	}

	for _, msg := range msgs {
		eventType, _ := msg.Values["event_type"].(string)
		dataStr, _ := msg.Values["data"].(string)

		if eventType == bridgeevents.EventResponseChunk {
			f.accumulateChunk(ctx, convID, dataStr)
			entryIDs = append(entryIDs, msg.ID)
			continue
		}
		if eventType == bridgeevents.EventReasoningDelta {
			f.accumulateReasoning(ctx, convID, dataStr)
			entryIDs = append(entryIDs, msg.ID)
			continue
		}

		var full struct {
			EventID        string          `json:"event_id"`
			EmployeeID     string          `json:"employee_id"`
			ConversationID string          `json:"conversation_id"`
			Timestamp      time.Time       `json:"timestamp"`
			SequenceNumber int64           `json:"sequence_number"`
			Data           json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal([]byte(dataStr), &full); err != nil {
			logging.FromContext(ctx).WarnContext(ctx, "flusher: failed to parse event payload", "conversation_id", convID, "error", err)
			captureTimelineFlush(ctx, "parse_event_payload", convID, err, map[string]any{
				"event_type":      eventType,
				"redis_stream_id": msg.ID,
			})
			entryIDs = append(entryIDs, msg.ID)
			continue
		}

		runtimeSessionID := full.ConversationID
		if runtimeSessionID == "" {
			runtimeSessionID = conv.RuntimeConversationID
		}
		source := conv.Source
		if source == "" {
			source = "manual"
		}
		eventAt := full.Timestamp
		if eventAt.IsZero() {
			eventAt = time.Now().UTC()
		}
		dbEvent := model.EmployeeSessionEvent{
			OrgID:             conv.OrgID,
			EmployeeID:        conv.EmployeeID,
			SandboxID:         conv.SandboxID,
			EmployeeSessionID: conv.ID,
			SessionID:         runtimeSessionID,
			EventID:           full.EventID,
			EventType:         eventType,
			Source:            source,
			SequenceNumber:    full.SequenceNumber,
			Payload:           model.RawJSON(full.Data),
			EventAt:           eventAt,
		}
		entryIDs = append(entryIDs, msg.ID)

		if eventType == bridgeevents.EventResponseCompleted {
			var d struct {
				MessageID string `json:"message_id"`
			}
			if err := json.Unmarshal(full.Data, &d); err == nil && d.MessageID != "" {
				if err := f.bus.DropChunk(ctx, convID, d.MessageID); err != nil {
					logging.FromContext(ctx).WarnContext(ctx, "flusher: drop chunk failed", "conversation_id", convID, "message_id", d.MessageID, "error", err)
					captureTimelineFlush(ctx, "drop_response_accumulator", convID, err, map[string]any{
						"event_type": eventType,
						"message_id": d.MessageID,
					})
				}
			}
		}
		if eventType == bridgeevents.EventReasoningCompleted {
			var d struct {
				MessageID string `json:"message_id"`
			}
			if err := json.Unmarshal(full.Data, &d); err == nil && d.MessageID != "" {
				if err := f.bus.DropAccumulated(ctx, "reasoning", convID, d.MessageID); err != nil {
					logging.FromContext(ctx).WarnContext(ctx, "flusher: drop reasoning accumulator failed", "conversation_id", convID, "message_id", d.MessageID, "error", err)
					captureTimelineFlush(ctx, "drop_reasoning_accumulator", convID, err, map[string]any{
						"event_type": eventType,
						"message_id": d.MessageID,
					})
				}
			}
		}

		if bridgeevents.IsTerminalEventType(eventType) {
			f.appendRecoveredEvents(ctx, convID, &conv, dbEvent, &events, &recoveredMsgIDs, recoveredSeen)
			f.appendRecoveredReasoningEvents(ctx, convID, &conv, dbEvent, &events, &recoveredReasoningIDs, recoveredReasoningSeen)
		}
		events = append(events, dbEvent)
	}

	if err := f.db.CreateInBatches(events, 50).Error; err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "flusher: batch insert failed", "conversation_id", convID, "count", len(events), "error", err)
		captureTimelineFlush(ctx, "store_employee_session_events", convID, err, map[string]any{
			"event_count": len(events),
			"entry_count": len(entryIDs),
		})
		return
	}

	if len(entryIDs) > 0 {
		f.bus.Redis().XAck(ctx, streamKey, flusherGroup, entryIDs...)
	}

	for _, mid := range recoveredMsgIDs {
		if err := f.bus.DropChunk(ctx, convID, mid); err != nil {
			logging.FromContext(ctx).WarnContext(ctx, "flusher: drop recovered chunk failed", "conversation_id", convID, "message_id", mid, "error", err)
			captureTimelineFlush(ctx, "drop_recovered_response", convID, err, map[string]any{"message_id": mid})
		}
	}
	for _, mid := range recoveredReasoningIDs {
		if err := f.bus.DropAccumulated(ctx, "reasoning", convID, mid); err != nil {
			logging.FromContext(ctx).WarnContext(ctx, "flusher: drop recovered reasoning failed", "conversation_id", convID, "message_id", mid, "error", err)
			captureTimelineFlush(ctx, "drop_recovered_reasoning", convID, err, map[string]any{"message_id": mid})
		}
	}

	if err := f.bus.Trim(ctx, convID, trimMaxLen); err != nil {
		logging.FromContext(ctx).WarnContext(ctx, "flusher: trim failed", "conversation_id", convID, "error", err)
		captureTimelineFlush(ctx, "trim_stream", convID, err, nil)
	}
}

func (f *Flusher) appendRecoveredReasoningEvents(
	ctx context.Context,
	convID string,
	conv *model.EmployeeConversation,
	terminal model.EmployeeSessionEvent,
	events *[]model.EmployeeSessionEvent,
	recoveredMsgIDs *[]string,
	recoveredSeen map[string]struct{},
) {
	recovered, err := f.bus.PeekAccumulated(ctx, "reasoning", convID)
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
			*events = append(*events, buildRecoveredReasoningEvent(conv, terminal, messageID, recovered[messageID]))
			*recoveredMsgIDs = append(*recoveredMsgIDs, messageID)
		}
	} else if err != nil {
		captureTimelineFlush(ctx, "peek_recovered_reasoning", convID, err, nil)
	}
	if err := f.bus.ClearActiveAccumulatedMessage(ctx, "reasoning", convID); err != nil {
		logging.FromContext(ctx).WarnContext(ctx, "flusher: clear active reasoning message failed", "conversation_id", convID, "error", err)
		captureTimelineFlush(ctx, "clear_active_reasoning", convID, err, nil)
	}
}

func (f *Flusher) appendRecoveredEvents(
	ctx context.Context,
	convID string,
	conv *model.EmployeeConversation,
	terminal model.EmployeeSessionEvent,
	events *[]model.EmployeeSessionEvent,
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
	} else if err != nil {
		captureTimelineFlush(ctx, "peek_recovered_response", convID, err, nil)
	}
	if err := f.bus.ClearActiveChunkMessage(ctx, convID); err != nil {
		logging.FromContext(ctx).WarnContext(ctx, "flusher: clear active chunk message failed", "conversation_id", convID, "error", err)
		captureTimelineFlush(ctx, "clear_active_response", convID, err, nil)
	}
}
