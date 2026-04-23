package streaming

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

const chunkAccTTL = 10 * time.Minute

func (b *EventBus) chunkKey(convID, messageID string) string {
	return "acc:" + convID + ":" + messageID
}

func (b *EventBus) chunkSetKey(convID string) string {
	return "acc_msgs:" + convID
}

func (b *EventBus) AppendChunk(ctx context.Context, convID, messageID, delta string) error {
	key := b.chunkKey(convID, messageID)
	setKey := b.chunkSetKey(convID)
	pipe := b.redis.Pipeline()
	pipe.Append(ctx, key, delta)
	pipe.Expire(ctx, key, chunkAccTTL)
	pipe.SAdd(ctx, setKey, messageID)
	pipe.Expire(ctx, setKey, chunkAccTTL)
	_, err := pipe.Exec(ctx)
	return err
}

func (b *EventBus) DropChunk(ctx context.Context, convID, messageID string) error {
	pipe := b.redis.Pipeline()
	pipe.Del(ctx, b.chunkKey(convID, messageID))
	pipe.SRem(ctx, b.chunkSetKey(convID), messageID)
	_, err := pipe.Exec(ctx)
	return err
}

func (b *EventBus) PeekChunks(ctx context.Context, convID string) (map[string]string, error) {
	messageIDs, err := b.redis.SMembers(ctx, b.chunkSetKey(convID)).Result()
	if err != nil || len(messageIDs) == 0 {
		return nil, err
	}
	result := make(map[string]string, len(messageIDs))
	for _, mid := range messageIDs {
		content, err := b.redis.Get(ctx, b.chunkKey(convID, mid)).Result()
		if err == redis.Nil {
			continue
		} else if err != nil {
			return nil, err
		}
		if content != "" {
			result[mid] = content
		}
	}
	return result, nil
}
