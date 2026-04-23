package streaming

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"

	"encoding/json"
)

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
		backfillFrom = "0"
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

		if backfillFrom != "$" {
			if err := b.backfill(ctx, convID, backfillFrom, attachCursor, userCh); err != nil {
				if ctx.Err() != nil {
					return
				}
				slog.Warn("eventbus.Subscribe: backfill error",
					"conversation_id", convID, "error", err)
			}
		}

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

func (b *EventBus) backfill(ctx context.Context, convID, from, upTo string, out chan<- StreamEvent) error {
	if upTo == "" || upTo == "0-0" {
		return nil
	}

	start := from
	if start == "" || start == "0" {
		start = "-"
	}

	msgs, err := b.redis.XRange(ctx, b.streamKey(convID), start, upTo).Result()
	if err != nil {
		return fmt.Errorf("backfill XRANGE: %w", err)
	}

	for _, msg := range msgs {
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

func (b *EventBus) getOrCreateTap(convID string) (*convTap, error) {
	b.tapsMu.Lock()
	defer b.tapsMu.Unlock()

	if tap, ok := b.taps[convID]; ok {
		return tap, nil
	}

	streamKey := b.streamKey(convID)

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

func (b *EventBus) detachSubscriber(convID string, tap *convTap, sub *subscriber) {
	tap.mu.Lock()
	delete(tap.subscribers, sub)
	empty := len(tap.subscribers) == 0
	tap.mu.Unlock()

	if !empty {
		return
	}

	b.tapsMu.Lock()
	if cur, ok := b.taps[convID]; ok && cur == tap {
		delete(b.taps, convID)
	}
	b.tapsMu.Unlock()

	tap.cancel()
	<-tap.done
}
