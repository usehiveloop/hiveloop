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
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"

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
	bus         *EventBus
	convID      string
	streamKey   string
	stopCtx     context.Context
	cancel      context.CancelFunc
	done        chan struct{}

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
//   - ""  = replay all events in the stream (legacy default, treated as "0")
//   - "0" = replay all events in the stream
//   - "$" = only new events from this point forward
//   - a specific entry ID = resume from after that entry
//
// Under the hood, subscribers on the same conversation on the same pod share
// a single XREAD BLOCK loop. Each subscriber receives its own backfill from
// the requested cursor and then the same live event stream.
//
// The returned channel is closed when the context is cancelled or when the
// subscriber is evicted for being slow (its 64-buffer filled).
func (b *EventBus) Subscribe(ctx context.Context, convID string, cursor string) <-chan StreamEvent {
	userCh := make(chan StreamEvent, 64)

	backfillFrom := cursor
	if backfillFrom == "" {
		backfillFrom = "0" // legacy default: replay everything
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("event bus subscriber panicked",
					"conversation_id", convID,
					"panic", r,
					"stack", string(debug.Stack()),
				)
			}
			closeChannelSafely(userCh)
		}()

		// 1. Attach to the conversation's tap. attachCursor is a snapshot of
		//    the tap's cursor at the moment we attached — the tap will only
		//    forward events with ID > attachCursor to us.
		tap, err := b.getOrCreateTap(convID)
		if err != nil {
			slog.Error("eventbus.Subscribe: failed to create tap",
				"conversation_id", convID, "error", err)
			return
		}
		sub, attachCursor := tap.attach()
		defer b.detachSubscriber(convID, tap, sub)

		slog.Info("eventbus.Subscribe: attached",
			"stream_key", tap.streamKey,
			"cursor", backfillFrom,
			"attach_cursor", attachCursor,
			"conversation_id", convID,
		)

		// 2. Backfill: read events from backfillFrom up to attachCursor
		//    (inclusive) and deliver them in order. The tap will pick up
		//    from attachCursor + 1 onwards. No duplicates, no gaps.
		if backfillFrom != "$" {
			if err := b.backfill(ctx, convID, backfillFrom, attachCursor, userCh); err != nil {
				if ctx.Err() != nil {
					return
				}
				slog.Warn("eventbus.Subscribe: backfill error",
					"conversation_id", convID, "error", err)
			}
		}

		// 3. Drain live events from the tap until the subscriber is cancelled
		//    or evicted.
		for {
			select {
			case <-ctx.Done():
				return
			case <-sub.evicted:
				slog.Warn("eventbus.Subscribe: evicted (slow consumer)",
					"conversation_id", convID)
				return
			case evt, ok := <-sub.ch:
				if !ok {
					return
				}
				select {
				case userCh <- evt:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return userCh
}

// backfill delivers stream entries strictly greater than `from` and
// less-than-or-equal to `upTo` into out. If upTo is "" or "0-0" (empty
// stream), nothing is delivered. The anchor entry (the one matching `from`
// exactly) is skipped so callers that pass their last-seen ID don't see it
// again.
func (b *EventBus) backfill(ctx context.Context, convID, from, upTo string, out chan<- StreamEvent) error {
	// Empty stream: nothing to backfill.
	if upTo == "" || upTo == "0-0" {
		return nil
	}

	start := from
	if start == "" || start == "0" {
		start = "-" // XRANGE: "-" means beginning of stream
	}

	msgs, err := b.redis.XRange(ctx, b.streamKey(convID), start, upTo).Result()
	if err != nil {
		return fmt.Errorf("backfill XRANGE: %w", err)
	}

	for _, msg := range msgs {
		// XRANGE is inclusive on both ends. Skip the anchor entry if the
		// caller supplied a real entry ID (they already saw it).
		if msg.ID == from && from != "" && from != "0" && from != "-" {
			continue
		}

		evt := StreamEvent{
			ID:        msg.ID,
			EventType: msgStringField(msg.Values, "event_type"),
			Data:      json.RawMessage(msgStringField(msg.Values, "data")),
		}

		select {
		case out <- evt:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

// getOrCreateTap returns the tap for convID, spawning it if necessary.
func (b *EventBus) getOrCreateTap(convID string) (*convTap, error) {
	b.tapsMu.Lock()
	defer b.tapsMu.Unlock()

	if tap, ok := b.taps[convID]; ok {
		return tap, nil
	}

	streamKey := b.streamKey(convID)

	// Seed the tap's starting cursor with the current last entry of the
	// stream so XREAD picks up only genuinely new events. "0-0" is used when
	// the stream is empty; XREAD accepts it and returns every future entry.
	startCursor := "0-0"
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	if res, err := b.redis.XRevRangeN(ctx, streamKey, "+", "-", 1).Result(); err == nil && len(res) > 0 {
		startCursor = res[0].ID
	}
	cancel()

	tapCtx, tapCancel := context.WithCancel(context.Background())
	tap := &convTap{
		bus:         b,
		convID:      convID,
		streamKey:   streamKey,
		stopCtx:     tapCtx,
		cancel:      tapCancel,
		done:        make(chan struct{}),
		cursor:      startCursor,
		subscribers: make(map[*subscriber]struct{}),
	}
	b.taps[convID] = tap
	go tap.run()
	return tap, nil
}

// detachSubscriber removes sub from tap and tears the tap down if no
// subscribers remain.
func (b *EventBus) detachSubscriber(convID string, tap *convTap, sub *subscriber) {
	tap.mu.Lock()
	delete(tap.subscribers, sub)
	empty := len(tap.subscribers) == 0
	tap.mu.Unlock()

	if !empty {
		return
	}

	// Only remove the tap from the registry if it's still the current one
	// for this convID (protects against racing recreation).
	b.tapsMu.Lock()
	if cur, ok := b.taps[convID]; ok && cur == tap {
		delete(b.taps, convID)
	}
	b.tapsMu.Unlock()

	tap.cancel()
	<-tap.done
}

// ActiveTaps returns the number of live conversation taps. Intended for tests
// and observability.
func (b *EventBus) ActiveTaps() int {
	b.tapsMu.Lock()
	defer b.tapsMu.Unlock()
	return len(b.taps)
}

// attach registers a new subscriber with the tap and returns its subscriber
// handle plus the tap's current cursor snapshot.
func (t *convTap) attach() (*subscriber, string) {
	sub := &subscriber{
		ch:      make(chan StreamEvent, 64),
		evicted: make(chan struct{}),
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	t.subscribers[sub] = struct{}{}
	return sub, t.cursor
}

// run is the single XREAD BLOCK loop for this conversation.
func (t *convTap) run() {
	defer close(t.done)
	defer func() {
		if r := recover(); r != nil {
			slog.Error("conv tap panicked",
				"conversation_id", t.convID,
				"panic", r,
				"stack", string(debug.Stack()),
			)
		}
		// Close all attached subscribers when the tap exits.
		t.mu.Lock()
		for s := range t.subscribers {
			closeChannelSafely(s.ch)
		}
		t.subscribers = nil
		t.mu.Unlock()
	}()

	slog.Info("conv tap started",
		"stream_key", t.streamKey,
		"conversation_id", t.convID,
		"start_cursor", t.cursor,
	)

	for {
		if t.stopCtx.Err() != nil {
			return
		}

		t.mu.Lock()
		pos := t.cursor
		t.mu.Unlock()

		streams, err := t.bus.redis.XRead(t.stopCtx, &redis.XReadArgs{
			Streams: []string{t.streamKey, pos},
			Block:   5 * time.Second,
			Count:   50,
		}).Result()
		if err != nil {
			if errors.Is(err, redis.Nil) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				continue
			}
			if t.stopCtx.Err() != nil {
				return
			}
			slog.Error("conv tap XREAD error",
				"conversation_id", t.convID, "error", err)
			select {
			case <-t.stopCtx.Done():
				return
			case <-time.After(500 * time.Millisecond):
			}
			continue
		}

		for _, stream := range streams {
			for _, msg := range stream.Messages {
				evt := StreamEvent{
					ID:        msg.ID,
					EventType: msgStringField(msg.Values, "event_type"),
					Data:      json.RawMessage(msgStringField(msg.Values, "data")),
				}
				t.broadcast(evt)

				t.mu.Lock()
				t.cursor = msg.ID
				t.mu.Unlock()
			}
		}
	}
}

// broadcast forwards evt to every attached subscriber. Subscribers whose
// channel buffer is full are evicted (marked as slow consumer).
func (t *convTap) broadcast(evt StreamEvent) {
	t.mu.Lock()
	defer t.mu.Unlock()

	for s := range t.subscribers {
		select {
		case s.ch <- evt:
		default:
			// Slow consumer: kick them out and close their channel.
			delete(t.subscribers, s)
			closeChannelSafely(s.ch)
			closeChannelSafely(s.evicted)
		}
	}
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
			EventType: msgStringField(msg.Values, "event_type"),
			Data:      json.RawMessage(msgStringField(msg.Values, "data")),
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

// closeChannelSafely closes ch unless it has already been closed.
func closeChannelSafely[T any](ch chan T) {
	defer func() { _ = recover() }()
	close(ch)
}

// msgStringField extracts a string field from a Redis stream message value
// map, returning "" if absent or the wrong type.
func msgStringField(values map[string]any, key string) string {
	v, ok := values[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}
