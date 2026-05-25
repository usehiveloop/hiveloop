package streaming

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/usehivy/hivy/internal/bridgeevents"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

func (f *Flusher) accumulateChunk(ctx context.Context, convID, dataStr string) {
	f.accumulateDelta(ctx, "response", convID, dataStr)
}

func (f *Flusher) accumulateReasoning(ctx context.Context, convID, dataStr string) {
	f.accumulateDelta(ctx, "reasoning", convID, dataStr)
}

func (f *Flusher) accumulateDelta(ctx context.Context, kind, convID, dataStr string) {
	var envelope struct {
		EventID string `json:"event_id"`
		Data    struct {
			Delta     string `json:"delta"`
			MessageID string `json:"message_id"`
			Text      string `json:"text"`
			Content   struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(dataStr), &envelope); err != nil {
		captureTimelineFlush(ctx, "parse_delta_payload", convID, err, map[string]any{"kind": kind})
		return
	}
	delta := envelope.Data.Delta
	if delta == "" && (envelope.Data.Content.Type == "" || envelope.Data.Content.Type == "text") {
		delta = envelope.Data.Content.Text
	}
	if delta == "" {
		delta = envelope.Data.Text
	}
	if delta == "" {
		return
	}

	messageID, err := f.bus.ResolveAccumulatedMessageID(ctx, kind, convID, envelope.Data.MessageID, envelope.EventID)
	if err != nil {
		logging.FromContext(ctx).WarnContext(ctx, "flusher: failed to resolve accumulated message id",
			"conversation_id", convID,
			"kind", kind,
			"error", err,
		)
		captureTimelineFlush(ctx, "resolve_accumulated_message_id", convID, err, map[string]any{"kind": kind})
		return
	}
	if messageID == "" {
		return
	}

	if err := f.bus.AppendAccumulated(ctx, kind, convID, messageID, delta); err != nil {
		logging.FromContext(ctx).WarnContext(ctx, "flusher: failed to accumulate delta",
			"conversation_id", convID,
			"kind", kind,
			"error", err,
		)
		captureTimelineFlush(ctx, "append_accumulated_delta", convID, err, map[string]any{
			"kind":       kind,
			"message_id": messageID,
		})
	}
}

func buildRecoveredEvent(conv *model.EmployeeConversation, terminal model.EmployeeSessionEvent, messageID, content string) model.EmployeeSessionEvent {
	return buildRecoveredAccumulatedEvent(conv, terminal, messageID, content, bridgeevents.EventResponseCompleted, "full_response")
}

func buildRecoveredReasoningEvent(conv *model.EmployeeConversation, terminal model.EmployeeSessionEvent, messageID, content string) model.EmployeeSessionEvent {
	return buildRecoveredAccumulatedEvent(conv, terminal, messageID, content, bridgeevents.EventReasoningCompleted, "full_reasoning")
}

func buildRecoveredAccumulatedEvent(conv *model.EmployeeConversation, terminal model.EmployeeSessionEvent, messageID, content, eventType, contentField string) model.EmployeeSessionEvent {
	data, _ := json.Marshal(map[string]any{
		"message_id": messageID,
		contentField: content,
		"recovered":  true,
	})
	timestamp := terminal.EventAt
	if timestamp.IsZero() {
		timestamp = time.Now()
	}
	sequenceNumber := terminal.SequenceNumber
	if sequenceNumber > 0 {
		sequenceNumber--
	}
	source := conv.Source
	if source == "" {
		source = "manual"
	}
	return model.EmployeeSessionEvent{
		OrgID:             conv.OrgID,
		EmployeeID:        terminal.EmployeeID,
		SandboxID:         conv.SandboxID,
		EmployeeSessionID: conv.ID,
		SessionID:         conv.RuntimeConversationID,
		EventID:           recoveredEventID(eventType, messageID),
		EventType:         eventType,
		Source:            source,
		SequenceNumber:    sequenceNumber,
		Payload:           model.RawJSON(data),
		EventAt:           timestamp,
	}
}

func recoveredEventID(eventType, messageID string) string {
	if eventType == bridgeevents.EventResponseCompleted {
		return "recovered-" + messageID
	}
	return "recovered-" + eventType + "-" + messageID
}

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

		_ = f.bus.Redis().XGroupCreateMkStream(ctx, streamKey, flusherGroup, "0").Err()

		streams, err := f.bus.Redis().XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    flusherGroup,
			Consumer: f.consumer,
			Streams:  []string{streamKey, "0"},
			Count:    flushBatchSize,
		}).Result()
		if err != nil || len(streams) == 0 || len(streams[0].Messages) == 0 {
			continue
		}

		f.flushStream(ctx, convID)
	}
}
