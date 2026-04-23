package streaming

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

func (b *EventBus) ReadRange(ctx context.Context, convID string, start string, end string) ([]StreamEvent, error) {
	msgs, err := b.redis.XRange(ctx, b.streamKey(convID), start, end).Result()
	if err != nil {
		return nil, fmt.Errorf("XRANGE: %w", err)
	}

	events := make([]StreamEvent, 0, len(msgs))
	for _, msg := range msgs {
		events = append(events, StreamEvent{
			ID:        msg.ID,
			EventType: msgStringField(msg.Values, "event_type"),
			Data:      json.RawMessage(msgStringField(msg.Values, "data")),
		})
	}
	return events, nil
}

func (b *EventBus) StreamLen(ctx context.Context, convID string) (int64, error) {
	return b.redis.XLen(ctx, b.streamKey(convID)).Result()
}

func (b *EventBus) Trim(ctx context.Context, convID string, maxLen int64) error {
	return b.redis.XTrimMaxLenApprox(ctx, b.streamKey(convID), maxLen, 0).Err()
}

func (b *EventBus) Delete(ctx context.Context, convID string) error {
	pipe := b.redis.Pipeline()
	pipe.Del(ctx, b.streamKey(convID))
	pipe.SRem(ctx, b.prefix+"active", convID)
	_, err := pipe.Exec(ctx)
	return err
}

func (b *EventBus) ActiveConversations(ctx context.Context) ([]string, error) {
	return b.redis.SMembers(ctx, b.prefix+"active").Result()
}
