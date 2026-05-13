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

func (b *EventBus) activeChunkMessageKey(convID string) string {
	return "acc_active:" + convID
}

func (b *EventBus) ResolveChunkMessageID(ctx context.Context, convID, explicitMessageID, eventID string) (string, error) {
	if explicitMessageID != "" {
		return explicitMessageID, nil
	}
	if eventID == "" {
		return "", nil
	}

	key := b.activeChunkMessageKey(convID)
	created, err := b.redis.SetNX(ctx, key, eventID, chunkAccTTL).Result()
	if err != nil {
		return "", err
	}
	if created {
		return eventID, nil
	}

	messageID, err := b.redis.Get(ctx, key).Result()
	if err == redis.Nil {
		if _, setErr := b.redis.SetNX(ctx, key, eventID, chunkAccTTL).Result(); setErr != nil {
			return "", setErr
		}
		return eventID, nil
	}
	if err != nil {
		return "", err
	}
	_ = b.redis.Expire(ctx, key, chunkAccTTL).Err()
	return messageID, nil
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

func (b *EventBus) ClearActiveChunkMessage(ctx context.Context, convID string) error {
	return b.redis.Del(ctx, b.activeChunkMessageKey(convID)).Err()
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
