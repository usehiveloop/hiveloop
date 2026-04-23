// Package streaming provides real-time event delivery via Redis Streams.
// Events are published by webhook handlers and consumed by SSE subscribers
// and a background DB flusher.
//
// Subscriber fan-out:
//
// Each pod runs at most one XREAD BLOCK loop per conversation, regardless of
// how many SSE subscribers are watching that conversation. Subscribers attach
// to an in-process "tap" that forwards every new event to every attached
// subscriber. This keeps Redis connections bounded by the number of active
// conversations viewed on the pod, not by the number of viewers.
//
// Each subscriber still gets correct backfill from its requested cursor via a
// synchronous XRANGE up to the tap's cursor at attach time; the tap only
// forwards events that arrive after the subscriber attached. There are no
// duplicates and no gaps.
package streaming

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"github.com/redis/go-redis/v9"
)

// StreamEvent is a single event read from a Redis Stream.
type StreamEvent struct {
	ID        string          // Redis entry ID (e.g., "1712019600000-0")
	EventType string          // e.g., "response_chunk", "response_completed"
	Data      json.RawMessage // Raw JSON payload
}

// subscriber is one attached consumer of a convTap.
type subscriber struct {
	ch      chan StreamEvent // tap -> subscriber drain goroutine
	evicted chan struct{}    // closed by tap when subscriber is dropped (slow consumer)
}

// convTap is the single XREAD BLOCK loop for one conversation on this pod.
// Multiple subscribers can attach; each receives every new event.
type convTap struct {
	bus       *EventBus
	convID    string
	streamKey string
	stopCtx   context.Context
	cancel    context.CancelFunc
	done      chan struct{}

	mu          sync.Mutex
	cursor      string                      // last entry ID the tap has read (advances monotonically)
	subscribers map[*subscriber]struct{}
}

// EventBus publishes and subscribes to conversation events via Redis Streams.
type EventBus struct {
	redis  *redis.Client
	prefix string // stream key prefix, e.g., "conv:"

	tapsMu sync.Mutex
	taps   map[string]*convTap
}

// NewEventBus creates a new EventBus.
func NewEventBus(redisClient *redis.Client) *EventBus {
	return &EventBus{
		redis:  redisClient,
		prefix: "conv:",
		taps:   make(map[string]*convTap),
	}
}

func (b *EventBus) streamKey(convID string) string {
	return b.prefix + convID
}

// Publish writes an event to a conversation's Redis Stream.
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

	b.redis.SAdd(ctx, b.prefix+"active", convID)

	return result, nil
}

// ActiveTaps returns the number of live conversation taps. Intended for tests
// and observability.
func (b *EventBus) ActiveTaps() int {
	b.tapsMu.Lock()
	defer b.tapsMu.Unlock()
	return len(b.taps)
}

// Prefix returns the stream key prefix.
func (b *EventBus) Prefix() string {
	return b.prefix
}

// Redis returns the underlying Redis client (for flusher consumer groups).
func (b *EventBus) Redis() *redis.Client {
	return b.redis
}

func closeChannelSafely[T any](ch chan T) {
	defer func() { _ = recover() }()
	close(ch)
}

func msgStringField(values map[string]any, key string) string {
	v, ok := values[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}
