package streaming

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func setupTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	client := redis.NewClient(&redis.Options{Addr: "localhost:6379", DB: 15})
	if err := client.Ping(context.Background()).Err(); err != nil {
		t.Skipf("Redis not available: %v", err)
	}
	t.Cleanup(func() {
		client.FlushDB(context.Background())
		client.Close()
	})
	return client
}

func TestPublish_WritesToRedisStream(t *testing.T) {
	rc := setupTestRedis(t)
	bus := NewEventBus(rc)

	data := json.RawMessage(`{"content":"hello"}`)
	entryID, err := bus.Publish(context.Background(), "test-conv-1", "message_received", data)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if entryID == "" {
		t.Fatal("expected non-empty entry ID")
	}

	// Verify it's in the stream
	msgs, err := rc.XRange(context.Background(), "conv:test-conv-1", "-", "+").Result()
	if err != nil {
		t.Fatalf("XRange: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Values["event_type"] != "message_received" {
		t.Errorf("event_type = %v, want message_received", msgs[0].Values["event_type"])
	}
}

func TestPublish_ReturnsEntryID(t *testing.T) {
	rc := setupTestRedis(t)
	bus := NewEventBus(rc)

	id, err := bus.Publish(context.Background(), "test-conv-2", "chunk", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	// Redis entry IDs have format "{ms}-{seq}"
	if len(id) < 3 {
		t.Fatalf("invalid entry ID: %q", id)
	}
	for i, c := range id {
		if c == '-' && i > 0 {
			return // valid format
		}
	}
	t.Fatalf("entry ID missing dash separator: %q", id)
}

func TestPublish_TracksActiveConversation(t *testing.T) {
	rc := setupTestRedis(t)
	bus := NewEventBus(rc)

	bus.Publish(context.Background(), "active-1", "chunk", json.RawMessage(`{}`))
	bus.Publish(context.Background(), "active-2", "chunk", json.RawMessage(`{}`))

	active, err := bus.ActiveConversations(context.Background())
	if err != nil {
		t.Fatalf("ActiveConversations: %v", err)
	}
	if len(active) != 2 {
		t.Fatalf("expected 2 active conversations, got %d", len(active))
	}
}

func TestSubscribe_ReceivesNewEvents(t *testing.T) {
	rc := setupTestRedis(t)
	bus := NewEventBus(rc)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ch := bus.Subscribe(ctx, "sub-test-1", "$") // new events only

	// Publish after subscribing
	go func() {
		time.Sleep(200 * time.Millisecond)
		bus.Publish(ctx, "sub-test-1", "event_a", json.RawMessage(`{"n":1}`))
		bus.Publish(ctx, "sub-test-1", "event_b", json.RawMessage(`{"n":2}`))
		bus.Publish(ctx, "sub-test-1", "event_c", json.RawMessage(`{"n":3}`))
	}()

	var received []StreamEvent
	for i := 0; i < 3; i++ {
		select {
		case ev := <-ch:
			received = append(received, ev)
		case <-time.After(8 * time.Second):
			t.Fatalf("timeout waiting for event %d, received %d so far", i+1, len(received))
		}
	}

	if received[0].EventType != "event_a" || received[1].EventType != "event_b" || received[2].EventType != "event_c" {
		t.Errorf("wrong event order: %v, %v, %v", received[0].EventType, received[1].EventType, received[2].EventType)
	}
}

func TestSubscribe_ResumeFromCursor(t *testing.T) {
	rc := setupTestRedis(t)
	bus := NewEventBus(rc)
	ctx := context.Background()

	// Publish 5 events
	var ids []string
	for i := 0; i < 5; i++ {
		id, _ := bus.Publish(ctx, "resume-test", "chunk", json.RawMessage(`{}`))
		ids = append(ids, id)
	}

	// Subscribe from after event 3 (should get events 4 and 5)
	subCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ch := bus.Subscribe(subCtx, "resume-test", ids[2]) // after event 3

	var received []StreamEvent
	for i := 0; i < 2; i++ {
		select {
		case ev := <-ch:
			received = append(received, ev)
		case <-time.After(3 * time.Second):
			t.Fatalf("timeout, got %d events", len(received))
		}
	}

	if len(received) != 2 {
		t.Fatalf("expected 2 events, got %d", len(received))
	}
	if received[0].ID != ids[3] || received[1].ID != ids[4] {
		t.Errorf("wrong resume: got IDs %s, %s; want %s, %s", received[0].ID, received[1].ID, ids[3], ids[4])
	}
}

func TestSubscribe_FullReplay(t *testing.T) {
	rc := setupTestRedis(t)
	bus := NewEventBus(rc)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		bus.Publish(ctx, "replay-test", "chunk", json.RawMessage(`{}`))
	}

	subCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ch := bus.Subscribe(subCtx, "replay-test", "0") // full replay

	var count int
	for count < 5 {
		select {
		case <-ch:
			count++
		case <-time.After(3 * time.Second):
			t.Fatalf("timeout, got %d of 5 events", count)
		}
	}
}

func TestSubscribe_ContextCancellation(t *testing.T) {
	rc := setupTestRedis(t)
	bus := NewEventBus(rc)
	ctx, cancel := context.WithCancel(context.Background())

	ch := bus.Subscribe(ctx, "cancel-test", "$")
	cancel()

	// Channel should close
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("expected channel to be closed after cancel")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("channel did not close after context cancellation")
	}
}

func TestReadRange_ReturnsCorrectWindow(t *testing.T) {
	rc := setupTestRedis(t)
	bus := NewEventBus(rc)
	ctx := context.Background()

	var ids []string
	for i := 0; i < 10; i++ {
		id, _ := bus.Publish(ctx, "range-test", "chunk", json.RawMessage(`{}`))
		ids = append(ids, id)
	}

	events, err := bus.ReadRange(ctx, "range-test", ids[2], ids[6])
	if err != nil {
		t.Fatalf("ReadRange: %v", err)
	}
	if len(events) != 5 { // inclusive range: 2,3,4,5,6
		t.Fatalf("expected 5 events, got %d", len(events))
	}
	if events[0].ID != ids[2] || events[4].ID != ids[6] {
		t.Errorf("wrong range boundaries")
	}
}

func TestTrim_ReducesStreamLength(t *testing.T) {
	rc := setupTestRedis(t)
	bus := NewEventBus(rc)
	ctx := context.Background()

	for i := 0; i < 1000; i++ {
		bus.Publish(ctx, "trim-test", "chunk", json.RawMessage(`{}`))
	}

	before, _ := bus.StreamLen(ctx, "trim-test")
	if before != 1000 {
		t.Fatalf("expected 1000 entries, got %d", before)
	}

	bus.Trim(ctx, "trim-test", 100)

	after, _ := bus.StreamLen(ctx, "trim-test")
	if after > 200 { // MAXLEN ~ is approximate
		t.Fatalf("expected ~100 entries after trim, got %d", after)
	}
}

func TestDelete_RemovesStream(t *testing.T) {
	rc := setupTestRedis(t)
	bus := NewEventBus(rc)
	ctx := context.Background()

	bus.Publish(ctx, "delete-test", "chunk", json.RawMessage(`{}`))

	bus.Delete(ctx, "delete-test")

	length, _ := bus.StreamLen(ctx, "delete-test")
	if length != 0 {
		t.Fatalf("expected 0 after delete, got %d", length)
	}

	active, _ := bus.ActiveConversations(ctx)
	for _, id := range active {
		if id == "delete-test" {
			t.Fatal("conversation still in active set after delete")
		}
	}
}

func TestPublish_RedisDown_ReturnsError(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: "localhost:19999"}) // non-existent
	bus := NewEventBus(client)

	_, err := bus.Publish(context.Background(), "fail-test", "chunk", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error when Redis is down")
	}
}
