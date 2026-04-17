// Package streaming provides real-time event delivery via Redis Streams.
// Events are published by webhook handlers and consumed by SSE subscribers
// and a background DB flusher.
package streaming

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/redis/go-redis/v9"
)

// StreamEvent is a single event read from a Redis Stream.
type StreamEvent struct {
	ID        string          // Redis entry ID (e.g., "1712019600000-0")
	EventType string          // e.g., "response_chunk", "response_completed"
	Data      json.RawMessage // Raw JSON payload
}

// EventBus publishes and subscribes to conversation events via Redis Streams.
type EventBus struct {
	redis  *redis.Client
	prefix string // stream key prefix, e.g., "conv:"
}

// NewEventBus creates a new EventBus.
func NewEventBus(redisClient *redis.Client) *EventBus {
	return &EventBus{
		redis:  redisClient,
		prefix: "conv:",
	}
}

// streamKey returns the Redis Stream key for a conversation.
func (b *EventBus) streamKey(convID string) string {
	return b.prefix + convID
}

// Publish writes an event to a conversation's Redis Stream.
// Returns the Redis entry ID (used as the SSE event id).
func (b *EventBus) Publish(ctx context.Context, convID string, eventType string, data json.RawMessage) (string, error) {
	result, err := b.redis.XAdd(ctx, &redis.XAddArgs{
		Stream: b.streamKey(convID),
		Values: map[string]any{
			"event_type": eventType,
			"data":       string(data),
		},
	}).Result()
	if err != nil {
		return "", fmt.Errorf("XADD: %w", err)
	}

	slog.Info("eventbus.Publish: event added",
		"stream_key", b.streamKey(convID),
		"event_type", eventType,
		"entry_id", result,
		"conversation_id", convID,
	)

	// Track this conversation as active for the flusher
	b.redis.SAdd(ctx, b.prefix+"active", convID)

	return result, nil
}

// Subscribe returns a channel that yields events from a conversation stream.
// cursor controls the starting point:
//   - "0" = replay all events in the stream
//   - "$" = only new events from this point forward
//   - a specific entry ID = resume from after that entry
//
// The channel is closed when the context is cancelled.
func (b *EventBus) Subscribe(ctx context.Context, convID string, cursor string) <-chan StreamEvent {
	ch := make(chan StreamEvent, 64)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("event bus subscriber panicked",
					"conversation_id", convID,
					"panic", r,
					"stack", string(debug.Stack()),
				)
			}
			close(ch)
		}()

		pos := cursor
		if pos == "" {
			pos = "0" // replay everything by default
		}

		streamKey := b.streamKey(convID)
		slog.Info("eventbus.Subscribe: started",
			"stream_key", streamKey,
			"cursor", pos,
			"conversation_id", convID,
		)
		pollCount := 0

		for {
			select {
			case <-ctx.Done():
				slog.Info("eventbus.Subscribe: context cancelled",
					"stream_key", streamKey,
					"polls", pollCount,
				)
				return
			default:
			}

			pollCount++
			streams, err := b.redis.XRead(ctx, &redis.XReadArgs{
				Streams: []string{streamKey, pos},
				Block:   5 * time.Second,
				Count:   50,
			}).Result()
			if err != nil {
				if err == redis.Nil || err == context.Canceled || err == context.DeadlineExceeded {
					if pollCount%12 == 0 { // log every ~60s
						slog.Info("eventbus.Subscribe: still waiting",
							"stream_key", streamKey,
							"polls", pollCount,
							"cursor", pos,
						)
					}
					continue
				}
				if ctx.Err() != nil {
					return
				}
				slog.Error("XREAD error", "conversation_id", convID, "error", err)
				time.Sleep(500 * time.Millisecond)
				continue
			}

			for _, stream := range streams {
				for _, msg := range stream.Messages {
					event := StreamEvent{
						ID:        msg.ID,
						EventType: msg.Values["event_type"].(string),
						Data:      json.RawMessage(msg.Values["data"].(string)),
					}

					select {
					case ch <- event:
						pos = msg.ID
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}()

	return ch
}

// ReadRange returns events between two entry IDs (inclusive).
// Use "-" and "+" for the beginning/end of the stream.
func (b *EventBus) ReadRange(ctx context.Context, convID string, start string, end string) ([]StreamEvent, error) {
	msgs, err := b.redis.XRange(ctx, b.streamKey(convID), start, end).Result()
	if err != nil {
		return nil, fmt.Errorf("XRANGE: %w", err)
	}

	events := make([]StreamEvent, 0, len(msgs))
	for _, msg := range msgs {
		events = append(events, StreamEvent{
			ID:        msg.ID,
			EventType: msg.Values["event_type"].(string),
			Data:      json.RawMessage(msg.Values["data"].(string)),
		})
	}
	return events, nil
}

// StreamLen returns the number of events in a conversation's stream.
func (b *EventBus) StreamLen(ctx context.Context, convID string) (int64, error) {
	return b.redis.XLen(ctx, b.streamKey(convID)).Result()
}

// Trim trims a stream to approximately maxLen entries.
func (b *EventBus) Trim(ctx context.Context, convID string, maxLen int64) error {
	return b.redis.XTrimMaxLenApprox(ctx, b.streamKey(convID), maxLen, 0).Err()
}

// Delete removes a conversation's stream entirely.
func (b *EventBus) Delete(ctx context.Context, convID string) error {
	pipe := b.redis.Pipeline()
	pipe.Del(ctx, b.streamKey(convID))
	pipe.SRem(ctx, b.prefix+"active", convID)
	_, err := pipe.Exec(ctx)
	return err
}

// ActiveConversations returns the set of conversation IDs with active streams.
func (b *EventBus) ActiveConversations(ctx context.Context) ([]string, error) {
	return b.redis.SMembers(ctx, b.prefix+"active").Result()
}

// --- chunk accumulators (fallback for missing response_completed) ---

const chunkAccTTL = 10 * time.Minute

func (b *EventBus) chunkKey(convID, messageID string) string {
	return "acc:" + convID + ":" + messageID
}

func (b *EventBus) chunkSetKey(convID string) string {
	return "acc_msgs:" + convID
}

// AppendChunk accumulates a streaming delta keyed by message_id so we can
// synthesize a response_completed if Bridge never sends one.
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

// DropChunk clears one message's accumulator — called when response_completed arrives.
func (b *EventBus) DropChunk(ctx context.Context, convID, messageID string) error {
	pipe := b.redis.Pipeline()
	pipe.Del(ctx, b.chunkKey(convID, messageID))
	pipe.SRem(ctx, b.chunkSetKey(convID), messageID)
	_, err := pipe.Exec(ctx)
	return err
}

// PeekChunks returns all unfinished accumulators for a conversation without
// deleting them. Caller must DropChunk each one after persisting.
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

// Prefix returns the stream key prefix.
func (b *EventBus) Prefix() string {
	return b.prefix
}

// Redis returns the underlying Redis client (for flusher consumer groups).
func (b *EventBus) Redis() *redis.Client {
	return b.redis
}
