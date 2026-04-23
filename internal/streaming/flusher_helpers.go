package streaming

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/usehiveloop/hiveloop/internal/model"
)

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

		f.bus.Redis().XGroupCreateMkStream(ctx, streamKey, flusherGroup, "0").Err()

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
