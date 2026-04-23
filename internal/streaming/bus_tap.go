package streaming

import (
	"errors"
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/redis/go-redis/v9"
)

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

func (t *convTap) broadcast(evt StreamEvent) {
	t.mu.Lock()
	defer t.mu.Unlock()

	for s := range t.subscribers {
		select {
		case s.ch <- evt:
		default:
			delete(t.subscribers, s)
			closeChannelSafely(s.ch)
			closeChannelSafely(s.evicted)
		}
	}
}
