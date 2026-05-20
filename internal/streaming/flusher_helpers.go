package streaming

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

func (f *Flusher) accumulateChunk(ctx context.Context, convID, dataStr string) {
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

	messageID, err := f.bus.ResolveChunkMessageID(ctx, convID, envelope.Data.MessageID, envelope.EventID)
	if err != nil {
		logging.FromContext(ctx).WarnContext(ctx, "flusher: failed to resolve chunk message id", "conversation_id", convID, "error", err)
		return
	}
	if messageID == "" {
		return
	}

	if err := f.bus.AppendChunk(ctx, convID, messageID, delta); err != nil {
		logging.FromContext(ctx).WarnContext(ctx, "flusher: failed to accumulate chunk", "conversation_id", convID, "error", err)
	}
}

func buildRecoveredEvent(conv *model.AgentConversation, terminal model.ConversationEvent, messageID, content string) model.ConversationEvent {
	data, _ := json.Marshal(map[string]any{
		"message_id":    messageID,
		"full_response": content,
		"recovered":     true,
	})
	timestamp := terminal.Timestamp
	if timestamp.IsZero() {
		timestamp = time.Now()
	}
	sequenceNumber := terminal.SequenceNumber
	if sequenceNumber > 0 {
		sequenceNumber--
	}
	return model.ConversationEvent{
		OrgID:                 conv.OrgID,
		ConversationID:        conv.ID,
		EventID:               "recovered-" + messageID,
		EventType:             "response_completed",
		AgentID:               terminal.AgentID,
		RuntimeConversationID: conv.RuntimeConversationID,
		Timestamp:             timestamp,
		SequenceNumber:        sequenceNumber,
		Data:                  model.RawJSON(data),
	}
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
